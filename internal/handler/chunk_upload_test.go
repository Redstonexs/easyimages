package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"easyimage/config"
	"easyimage/internal/storage"

	"github.com/gin-gonic/gin"
)

func TestChunkUploadLocalMergeWritesFinalFileAndCleansChunks(t *testing.T) {
	tmp := chdirTemp(t)
	cfg := chunkTestConfig()
	r := chunkTestRouter(cfg)

	for index, content := range []string{"hello", " world"} {
		w := performChunkRequest(t, r, map[string]string{
			"uploadId":       "local-upload",
			"chunkIndex":     fmt.Sprintf("%d", index),
			"totalChunks":    "2",
			"filename":       "sample.txt",
			"storage_source": "local",
		}, []byte(content))
		if w.Code != http.StatusOK {
			t.Fatalf("chunk %d status = %d, body = %s", index, w.Code, w.Body.String())
		}
	}

	w := performChunkRequest(t, r, map[string]string{
		"uploadId":       "local-upload",
		"totalChunks":    "2",
		"filename":       "sample.txt",
		"merge":          "true",
		"storage_source": "local",
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("merge status = %d, body = %s", w.Code, w.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	url, ok := payload["url"].(string)
	if !ok || url == "" {
		t.Fatalf("url missing in response: %s", w.Body.String())
	}
	relativePath := strings.TrimPrefix(url, cfg.Domain)
	content, err := os.ReadFile(filepath.Join(tmp, filepath.FromSlash(strings.TrimPrefix(relativePath, "/"))))
	if err != nil {
		t.Fatalf("read merged file: %v", err)
	}
	if string(content) != "hello world" {
		t.Fatalf("merged content = %q, want %q", content, "hello world")
	}
	if _, err := os.Stat(filepath.Join(tmp, "i", "chunks", "local-upload")); !os.IsNotExist(err) {
		t.Fatalf("chunk directory still exists, stat error = %v", err)
	}
}

func TestChunkUploadLocalMergeRejectsMissingPart(t *testing.T) {
	chdirTemp(t)
	cfg := chunkTestConfig()
	r := chunkTestRouter(cfg)

	w := performChunkRequest(t, r, map[string]string{
		"uploadId":       "missing-part",
		"chunkIndex":     "0",
		"totalChunks":    "2",
		"filename":       "sample.txt",
		"storage_source": "local",
	}, []byte("hello"))
	if w.Code != http.StatusOK {
		t.Fatalf("chunk status = %d, body = %s", w.Code, w.Body.String())
	}

	w = performChunkRequest(t, r, map[string]string{
		"uploadId":       "missing-part",
		"totalChunks":    "2",
		"filename":       "sample.txt",
		"merge":          "true",
		"storage_source": "local",
	}, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("merge status = %d, want %d, body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "分片 1") {
		t.Fatalf("response = %s, want missing part message", w.Body.String())
	}
}

func TestS3ChunkUploadUploadsPartsConcurrently(t *testing.T) {
	chdirTemp(t)
	cfg := chunkTestConfig()
	cfg.DefaultStorageSource = "s3-main"
	cfg.StorageSources = []config.StorageSourceConfig{
		{ID: "local", Name: "本地存储", Type: "local", Enabled: true},
		{ID: "s3-main", Name: "S3", Type: "s3", Enabled: true, S3Bucket: "bucket", PublicBaseURL: "https://cdn.example.com"},
	}

	state := storage.MultipartState{
		UploadID:     "s3-upload",
		S3UploadID:   "remote-upload",
		SourceID:     "s3-main",
		ObjectKey:    "2026/06/06/sample.jpg",
		RelativePath: "/i/2026/06/06/sample.jpg",
		OriginalName: "sample.jpg",
		TotalChunks:  2,
		CreatedAt:    time.Now(),
		Parts:        map[int32]string{},
		PartSizes:    map[int32]int64{},
	}
	if err := storage.SaveMultipartState(state); err != nil {
		t.Fatalf("save multipart state: %v", err)
	}

	fake := &blockingS3MultipartStore{started: make(chan int32, 2), release: make(chan struct{})}
	oldFactory := newS3MultipartStore
	newS3MultipartStore = func(context.Context, config.StorageSourceConfig) (s3MultipartStore, error) {
		return fake, nil
	}
	t.Cleanup(func() { newS3MultipartStore = oldFactory })

	r := chunkTestRouter(cfg)
	responses := make(chan *httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses <- performChunkRequest(t, r, map[string]string{
				"uploadId":       "s3-upload",
				"chunkIndex":     fmt.Sprintf("%d", index),
				"totalChunks":    "2",
				"filename":       "sample.jpg",
				"storage_source": "s3-main",
			}, []byte{byte('a' + index)})
		}()
	}

	seen := map[int32]bool{}
	for len(seen) < 2 {
		select {
		case partNumber := <-fake.started:
			seen[partNumber] = true
		case <-time.After(time.Second):
			close(fake.release)
			t.Fatal("timed out waiting for both S3 parts to start concurrently")
		}
	}
	close(fake.release)
	waitForWaitGroup(t, &wg)
	close(responses)
	for w := range responses {
		if w.Code != http.StatusOK {
			t.Fatalf("chunk status = %d, body = %s", w.Code, w.Body.String())
		}
	}

	updated, err := storage.LoadMultipartState("s3-upload")
	if err != nil {
		t.Fatalf("load updated state: %v", err)
	}
	if len(updated.Parts) != 2 || updated.TotalSize != 2 {
		t.Fatalf("updated state parts=%v totalSize=%d, want 2 parts and size 2", updated.Parts, updated.TotalSize)
	}
}

type blockingS3MultipartStore struct {
	started chan int32
	release chan struct{}
}

func (s *blockingS3MultipartStore) CreateMultipart(context.Context, string, string) (string, error) {
	return "remote-upload", nil
}

func (s *blockingS3MultipartStore) UploadPart(_ context.Context, _ string, _ string, partNumber int32, body io.Reader, _ int64) (string, error) {
	_, _ = io.Copy(io.Discard, body)
	s.started <- partNumber
	<-s.release
	return fmt.Sprintf("etag-%d", partNumber), nil
}

func (s *blockingS3MultipartStore) CompleteMultipart(context.Context, string, string, map[int32]string) error {
	return nil
}

func (s *blockingS3MultipartStore) AbortMultipart(context.Context, string, string) error {
	return nil
}

func chunkTestConfig() *config.Config {
	return &config.Config{
		Path:                 "/i/",
		StoragePath:          "Y/m/d/",
		Domain:               "https://img.example.com",
		ImgName:              "source",
		MaxSize:              1024 * 1024,
		Extensions:           "txt,jpg,jpeg,png,svg",
		DefaultStorageSource: "local",
		StorageSources:       []config.StorageSourceConfig{{ID: "local", Name: "本地存储", Type: "local", Enabled: true}},
	}
}

func chunkTestRouter(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/app/upload/chunk", ChunkUpload(cfg))
	return r
}

func performChunkRequest(t *testing.T, r *gin.Engine, fields map[string]string, chunk []byte) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	if chunk != nil {
		part, err := writer.CreateFormFile("chunk", fields["filename"])
		if err != nil {
			t.Fatalf("create chunk form file: %v", err)
		}
		if _, err := part.Write(chunk); err != nil {
			t.Fatalf("write chunk: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/app/upload/chunk", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	r.ServeHTTP(w, req)
	return w
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
	return tmp
}

func waitForWaitGroup(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for requests to finish")
	}
}
