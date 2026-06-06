package handler

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"easyimage/config"
	"easyimage/internal/service"
	"easyimage/internal/storage"

	"github.com/gin-gonic/gin"
)

type s3MultipartStore interface {
	CreateMultipart(ctx context.Context, key, contentType string) (string, error)
	UploadPart(ctx context.Context, key, uploadID string, partNumber int32, body io.Reader, size int64) (string, error)
	CompleteMultipart(ctx context.Context, key, uploadID string, parts map[int32]string) error
	AbortMultipart(ctx context.Context, key, uploadID string) error
}

type s3MultipartLock struct {
	mu   sync.Mutex
	refs int
}

var (
	s3MultipartLocksMu  sync.Mutex
	s3MultipartLocks    = map[string]*s3MultipartLock{}
	newS3MultipartStore = func(ctx context.Context, source config.StorageSourceConfig) (s3MultipartStore, error) {
		return storage.NewS3Store(ctx, source)
	}
)

func lockS3MultipartState(uploadID string) func() {
	s3MultipartLocksMu.Lock()
	lock := s3MultipartLocks[uploadID]
	if lock == nil {
		lock = &s3MultipartLock{}
		s3MultipartLocks[uploadID] = lock
	}
	lock.refs++
	s3MultipartLocksMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		s3MultipartLocksMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(s3MultipartLocks, uploadID)
		}
		s3MultipartLocksMu.Unlock()
	}
}

func handleS3ChunkUpload(c *gin.Context, cfg *config.Config, source config.StorageSourceConfig, uploadID string, totalChunks, chunkIndex int, isMerge bool, filename string) {
	ctx := c.Request.Context()
	store, err := newS3MultipartStore(ctx, source)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "S3配置无效"})
		return
	}

	if isMerge {
		mergeStarted := time.Now()
		unlock := lockS3MultipartState(uploadID)
		defer unlock()

		state, err := storage.LoadMultipartState(uploadID)
		if err != nil || state.SourceID != source.ID || state.TotalChunks != totalChunks {
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "无效的S3分片状态"})
			return
		}
		if len(state.Parts) != state.TotalChunks {
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "分片尚未上传完成"})
			return
		}
		if state.TotalSize > cfg.MaxSize {
			_ = store.AbortMultipart(ctx, state.ObjectKey, state.S3UploadID)
			_ = storage.DeleteMultipartState(uploadID)
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": fmt.Sprintf("文件大小超过限制: %s", service.FormatSize(cfg.MaxSize))})
			return
		}
		if err := store.CompleteMultipart(ctx, state.ObjectKey, state.S3UploadID, state.Parts); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "S3分片合并失败"})
			return
		}
		_ = storage.DeleteMultipartState(uploadID)

		imageURL := storage.PublicURL(source, state.ObjectKey)
		service.SaveImageMetadataWithStorage(state.RelativePath, state.OriginalName, state.TotalSize, "web", time.Now(), source.ID, "s3", state.ObjectKey, imageURL, imageURL)
		if elapsed := time.Since(mergeStarted); elapsed > 2*time.Second {
			log.Printf("[s3 chunk upload] slow merge uploadId=%s chunks=%d size=%d elapsed=%s", uploadID, state.TotalChunks, state.TotalSize, elapsed)
		}
		c.JSON(http.StatusOK, gin.H{
			"result": "success", "code": 200,
			"url": imageURL, "srcName": trimExt(state.OriginalName), "original_name": state.OriginalName, "thumb": imageURL, "storage_source": source.ID,
		})
		return
	}

	chunk, err := c.FormFile("chunk")
	if err != nil || chunk == nil {
		c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "缺少分片数据"})
		return
	}
	if chunk.Size == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": fmt.Sprintf("分片 %d 为空文件", chunkIndex)})
		return
	}

	state, stateErr := loadOrCreateS3MultipartState(ctx, store, cfg, source, uploadID, totalChunks, filename)
	if stateErr != nil {
		status := http.StatusInternalServerError
		if stateErr.clientError {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"result": "failed", "code": status, "message": stateErr.message})
		return
	}
	uploadState := state

	file, err := chunk.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "读取分片失败"})
		return
	}
	defer file.Close()

	partNumber := int32(chunkIndex + 1)
	uploadStarted := time.Now()
	etag, err := store.UploadPart(ctx, state.ObjectKey, state.S3UploadID, partNumber, file, chunk.Size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "上传S3分片失败"})
		return
	}
	if elapsed := time.Since(uploadStarted); elapsed > 2*time.Second {
		log.Printf("[s3 chunk upload] slow part upload uploadId=%s chunkIndex=%d size=%d elapsed=%s", uploadID, chunkIndex, chunk.Size, elapsed)
	}

	unlock := lockS3MultipartState(uploadID)
	state, err = storage.LoadMultipartState(uploadID)
	if err != nil {
		unlock()
		c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "S3分片状态不存在"})
		return
	}
	if state.SourceID != source.ID || state.TotalChunks != totalChunks || state.S3UploadID != uploadState.S3UploadID || state.ObjectKey != uploadState.ObjectKey {
		unlock()
		c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "S3分片状态不匹配"})
		return
	}
	if previousSize := state.PartSizes[partNumber]; previousSize > 0 {
		state.TotalSize -= previousSize
	}
	state.Parts[partNumber] = etag
	state.PartSizes[partNumber] = chunk.Size
	state.TotalSize += chunk.Size
	if err := storage.SaveMultipartState(state); err != nil {
		unlock()
		c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "保存S3分片状态失败"})
		return
	}
	unlock()
	c.JSON(http.StatusOK, gin.H{"result": "success", "code": 200, "message": "S3分片上传成功", "chunkIndex": chunkIndex})
}

type s3MultipartStateError struct {
	message     string
	clientError bool
}

func loadOrCreateS3MultipartState(ctx context.Context, store s3MultipartStore, cfg *config.Config, source config.StorageSourceConfig, uploadID string, totalChunks int, filename string) (storage.MultipartState, *s3MultipartStateError) {
	unlock := lockS3MultipartState(uploadID)
	defer unlock()

	state, err := storage.LoadMultipartState(uploadID)
	if err == nil {
		if state.SourceID != source.ID || state.TotalChunks != totalChunks {
			return storage.MultipartState{}, &s3MultipartStateError{message: "S3分片状态不匹配", clientError: true}
		}
		return state, nil
	}

	target, targetErr := service.BuildUploadTarget(filename, cfg, source)
	if targetErr != nil {
		return storage.MultipartState{}, &s3MultipartStateError{message: targetErr.Error(), clientError: true}
	}
	if target.Ext == "svg" {
		return storage.MultipartState{}, &s3MultipartStateError{message: "S3分片上传暂不支持SVG文件", clientError: true}
	}
	s3UploadID, createErr := store.CreateMultipart(ctx, target.ObjectKey, storage.ContentType(target.FileName))
	if createErr != nil {
		return storage.MultipartState{}, &s3MultipartStateError{message: "创建S3分片上传失败"}
	}
	state = storage.MultipartState{
		UploadID:     uploadID,
		S3UploadID:   s3UploadID,
		SourceID:     source.ID,
		ObjectKey:    target.ObjectKey,
		RelativePath: target.RelativePath,
		OriginalName: target.OriginalName,
		TotalChunks:  totalChunks,
		ContentType:  storage.ContentType(target.FileName),
		CreatedAt:    time.Now(),
		Parts:        map[int32]string{},
		PartSizes:    map[int32]int64{},
	}
	if err := storage.SaveMultipartState(state); err != nil {
		return storage.MultipartState{}, &s3MultipartStateError{message: "保存S3分片状态失败"}
	}
	return state, nil
}

func trimExt(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}
