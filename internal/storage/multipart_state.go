package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const multipartStateDir = "admin/logs/multipart"

type MultipartState struct {
	UploadID     string           `json:"upload_id"`
	S3UploadID   string           `json:"s3_upload_id"`
	SourceID     string           `json:"source_id"`
	ObjectKey    string           `json:"object_key"`
	RelativePath string           `json:"relative_path"`
	OriginalName string           `json:"original_name"`
	TotalChunks  int              `json:"total_chunks"`
	TotalSize    int64            `json:"total_size"`
	ContentType  string           `json:"content_type"`
	CreatedAt    time.Time        `json:"created_at"`
	Parts        map[int32]string `json:"parts"`
	PartSizes    map[int32]int64  `json:"part_sizes"`
}

func LoadMultipartState(uploadID string) (MultipartState, error) {
	path, err := multipartStatePath(uploadID)
	if err != nil {
		return MultipartState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return MultipartState{}, err
	}
	var state MultipartState
	if err := json.Unmarshal(data, &state); err != nil {
		return MultipartState{}, err
	}
	if state.Parts == nil {
		state.Parts = map[int32]string{}
	}
	if state.PartSizes == nil {
		state.PartSizes = map[int32]int64{}
	}
	return state, nil
}

func SaveMultipartState(state MultipartState) error {
	path, err := multipartStatePath(state.UploadID)
	if err != nil {
		return err
	}
	if state.Parts == nil {
		state.Parts = map[int32]string{}
	}
	if state.PartSizes == nil {
		state.PartSizes = map[int32]int64{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func DeleteMultipartState(uploadID string) error {
	path, err := multipartStatePath(uploadID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func multipartStatePath(uploadID string) (string, error) {
	if uploadID == "" {
		return "", fmt.Errorf("upload id is required")
	}
	for _, ch := range uploadID {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-') {
			return "", fmt.Errorf("invalid upload id")
		}
	}
	base, err := filepath.Abs(multipartStateDir)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(uploadID) + ".json"
	target := filepath.Join(base, name)
	rel, err := filepath.Rel(base, target)
	if err != nil || rel != name || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid upload id")
	}
	return filepath.Join(base, rel), nil
}
