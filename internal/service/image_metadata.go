package service

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"easyimage/config"
)

const imageMetadataDir = "admin/logs/metadata"

type ImageMetadata struct {
	Path         string `json:"path"`
	StoredName   string `json:"stored_name"`
	OriginalName string `json:"original_name"`
	OriginalBase string `json:"original_base"`
	UploadedAt   string `json:"uploaded_at"`
	Size         int64  `json:"size"`
	From         string `json:"from"`
}

var imageMetadataMu sync.Mutex

func SaveImageMetadata(relativePath, originalName string, size int64, from string, uploadedAt time.Time) {
	if relativePath == "" || originalName == "" {
		return
	}
	if _, err := ValidateURLPath(relativePath, metadataStoragePrefix()); err != nil {
		log.Printf("[Metadata] invalid image path %q: %v", relativePath, err)
		return
	}
	originalName = OriginalUploadName(originalName)

	metadata := ImageMetadata{
		Path:         relativePath,
		StoredName:   filepath.Base(relativePath),
		OriginalName: originalName,
		OriginalBase: strings.TrimSuffix(originalName, filepath.Ext(originalName)),
		UploadedAt:   uploadedAt.Format("2006-01-02 15:04:05"),
		Size:         size,
		From:         from,
	}
	if err := saveImageMetadata(metadata); err != nil {
		log.Printf("[Metadata] save failed for %s: %v", relativePath, err)
	}
}

func OriginalUploadName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	if name == "." || name == "/" || name == "" {
		return "upload"
	}
	return name
}

func GetImageMetadata(relativePath string) (ImageMetadata, bool) {
	imageMetadataMu.Lock()
	defer imageMetadataMu.Unlock()

	items, err := loadImageMetadataMonthLocked(metadataMonthFromPath(relativePath))
	if err != nil {
		return ImageMetadata{}, false
	}
	metadata, ok := items[relativePath]
	return metadata, ok
}

func DeleteImageMetadata(relativePath string) {
	if relativePath == "" {
		return
	}
	imageMetadataMu.Lock()
	defer imageMetadataMu.Unlock()

	month := metadataMonthFromPath(relativePath)
	items, err := loadImageMetadataMonthLocked(month)
	if err != nil || len(items) == 0 {
		return
	}
	if _, ok := items[relativePath]; !ok {
		return
	}
	delete(items, relativePath)
	if err := writeImageMetadataMonthLocked(month, items); err != nil {
		log.Printf("[Metadata] delete failed for %s: %v", relativePath, err)
	}
}

func LoadImageMetadataForDate(datePath string) map[string]ImageMetadata {
	imageMetadataMu.Lock()
	defer imageMetadataMu.Unlock()

	items, err := loadImageMetadataMonthLocked(metadataMonthFromDatePath(datePath))
	if err != nil || len(items) == 0 {
		return map[string]ImageMetadata{}
	}
	result := make(map[string]ImageMetadata)
	needle := "/" + strings.Trim(datePath, "/") + "/"
	for path, metadata := range items {
		if strings.Contains(path, needle) {
			result[path] = metadata
		}
	}
	return result
}

func MetadataMatchesQuery(metadata ImageMetadata, storedName, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	return strings.Contains(strings.ToLower(storedName), query) ||
		strings.Contains(strings.ToLower(metadata.StoredName), query) ||
		strings.Contains(strings.ToLower(metadata.OriginalName), query) ||
		strings.Contains(strings.ToLower(metadata.OriginalBase), query)
}

func saveImageMetadata(metadata ImageMetadata) error {
	imageMetadataMu.Lock()
	defer imageMetadataMu.Unlock()

	month := metadataMonthFromPath(metadata.Path)
	items, err := loadImageMetadataMonthLocked(month)
	if err != nil {
		return err
	}
	items[metadata.Path] = metadata
	return writeImageMetadataMonthLocked(month, items)
}

func loadImageMetadataMonthLocked(month string) (map[string]ImageMetadata, error) {
	if !isSafeMetadataMonth(month) {
		return nil, fmt.Errorf("invalid metadata month")
	}
	path := imageMetadataMonthPath(month)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]ImageMetadata{}, nil
		}
		return nil, err
	}
	items := make(map[string]ImageMetadata)
	if len(data) == 0 {
		return items, nil
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func writeImageMetadataMonthLocked(month string, items map[string]ImageMetadata) error {
	if !isSafeMetadataMonth(month) {
		return fmt.Errorf("invalid metadata month")
	}
	if err := os.MkdirAll(imageMetadataDir, 0755); err != nil {
		return err
	}
	path := imageMetadataMonthPath(month)
	if len(items) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func imageMetadataMonthPath(month string) string {
	return filepath.Join(imageMetadataDir, month+".json")
}

func metadataMonthFromPath(relativePath string) string {
	parts := strings.Split(strings.Trim(relativePath, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if len(parts[i]) == 4 && len(parts[i+1]) == 2 && isDigits(parts[i]) && isDigits(parts[i+1]) {
			return parts[i] + "-" + parts[i+1]
		}
	}
	return time.Now().Format("2006-01")
}

func metadataMonthFromDatePath(datePath string) string {
	parts := strings.Split(strings.Trim(datePath, "/"), "/")
	if len(parts) >= 2 && len(parts[0]) == 4 && len(parts[1]) == 2 {
		return parts[0] + "-" + parts[1]
	}
	return time.Now().Format("2006-01")
}

func isSafeMetadataMonth(month string) bool {
	if len(month) != len("2006-01") {
		return false
	}
	for i, ch := range month {
		switch i {
		case 4:
			if ch != '-' {
				return false
			}
		default:
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	return true
}

func metadataStoragePrefix() string {
	cfg := config.Get()
	if cfg != nil && cfg.Path != "" {
		return cfg.Path
	}
	return "/i/"
}

func isDigits(value string) bool {
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return value != ""
}
