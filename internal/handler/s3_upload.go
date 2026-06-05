package handler

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"easyimage/config"
	"easyimage/internal/service"
	"easyimage/internal/storage"

	"github.com/gin-gonic/gin"
)

var s3MultipartMu sync.Mutex

func handleS3ChunkUpload(c *gin.Context, cfg *config.Config, source config.StorageSourceConfig, uploadID string, totalChunks, chunkIndex int, isMerge bool, filename string) {
	s3MultipartMu.Lock()
	defer s3MultipartMu.Unlock()

	ctx := c.Request.Context()
	store, err := storage.NewS3Store(ctx, source)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "S3配置无效"})
		return
	}

	if isMerge {
		state, err := storage.LoadMultipartState(uploadID)
		if err != nil || state.SourceID != source.ID {
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

	state, err := storage.LoadMultipartState(uploadID)
	if err != nil {
		target, targetErr := service.BuildUploadTarget(filename, cfg, source)
		if targetErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": targetErr.Error()})
			return
		}
		if target.Ext == "svg" {
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "S3分片上传暂不支持SVG文件"})
			return
		}
		s3UploadID, createErr := store.CreateMultipart(ctx, target.ObjectKey, storage.ContentType(target.FileName))
		if createErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "创建S3分片上传失败"})
			return
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
	}
	if state.SourceID != source.ID || state.TotalChunks != totalChunks {
		c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "S3分片状态不匹配"})
		return
	}

	file, err := chunk.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "读取分片失败"})
		return
	}
	defer file.Close()

	etag, err := store.UploadPart(ctx, state.ObjectKey, state.S3UploadID, int32(chunkIndex+1), file, chunk.Size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "上传S3分片失败"})
		return
	}
	partNumber := int32(chunkIndex + 1)
	if previousSize := state.PartSizes[partNumber]; previousSize > 0 {
		state.TotalSize -= previousSize
	}
	state.Parts[partNumber] = etag
	state.PartSizes[partNumber] = chunk.Size
	state.TotalSize += chunk.Size
	if err := storage.SaveMultipartState(state); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "保存S3分片状态失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": "success", "code": 200, "message": "S3分片上传成功", "chunkIndex": chunkIndex})
}

func trimExt(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}
