package storage

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"easyimage/config"
)

func TestObjectKeyUsesPrefixAndStripsStorageRoot(t *testing.T) {
	source := config.StorageSourceConfig{ID: "s3-main", Type: "s3", S3Prefix: "uploads"}
	key, err := ObjectKey(source, "/i/2026/06/05/photo.jpg")
	if err != nil {
		t.Fatalf("ObjectKey() error = %v", err)
	}
	if key != "uploads/2026/06/05/photo.jpg" {
		t.Fatalf("ObjectKey() = %q", key)
	}
}

func TestPublicURLUsesConfiguredBase(t *testing.T) {
	source := config.StorageSourceConfig{PublicBaseURL: "https://cdn.example.com/images/"}
	url := PublicURL(source, "2026/06/05/photo.jpg")
	if url != "https://cdn.example.com/images/2026/06/05/photo.jpg" {
		t.Fatalf("PublicURL() = %q", url)
	}
}

func TestCustomS3EndpointKeepsUploadCompatibility(t *testing.T) {
	const content = "test payload"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/photo.jpg" {
			t.Errorf("unexpected object path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Amz-Checksum-Crc32") != "" || r.Header.Get("X-Amz-Trailer") != "" {
			t.Errorf("custom endpoint received an optional request checksum")
		}
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.Method == http.MethodPut:
			data, err := io.ReadAll(r.Body)
			if err != nil || string(data) != content {
				t.Errorf("unexpected object body: %q (%v)", data, err)
			}
			w.Header().Set("ETag", `"fixture-etag"`)
		case r.Method == http.MethodPost && r.URL.Query().Has("uploads"):
			fmt.Fprint(w, "<InitiateMultipartUploadResult><UploadId>fixture-upload</UploadId></InitiateMultipartUploadResult>")
		case r.Method == http.MethodPost && r.URL.Query().Get("uploadId") == "fixture-upload":
			fmt.Fprint(w, "<CompleteMultipartUploadResult><ETag>fixture-complete</ETag></CompleteMultipartUploadResult>")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	store, err := NewS3Store(t.Context(), config.StorageSourceConfig{
		S3Endpoint: server.URL, S3Bucket: "images", S3AccessKeyID: "fixture", S3AccessKeySecret: "fixture",
		S3Region: "auto", S3ForcePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	const key = "photo.jpg"
	if err := store.Put(t.Context(), key, strings.NewReader(content), int64(len(content)), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateMultipart(t.Context(), key, "image/jpeg")
	if err != nil || id != "fixture-upload" {
		t.Fatalf("initiate multipart = %q, %v", id, err)
	}
	etag, err := store.UploadPart(t.Context(), key, id, 1, strings.NewReader(content), int64(len(content)))
	if err != nil || etag != `"fixture-etag"` {
		t.Fatalf("upload part = %q, %v", etag, err)
	}
	if err := store.CompleteMultipart(t.Context(), key, id, map[int32]string{1: etag}); err != nil {
		t.Fatal(err)
	}
}
