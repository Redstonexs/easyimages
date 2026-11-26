package service

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"easyimage/config"
)

const imageMetadataDir = "admin/logs/metadata"

type ImageMetadata struct {
	Path          string `json:"path"`
	StoredName    string `json:"stored_name"`
	OriginalName  string `json:"original_name"`
	OriginalBase  string `json:"original_base"`
	UploadedAt    string `json:"uploaded_at"`
	Size          int64  `json:"size"`
	From          string `json:"from"`
	StorageSource string `json:"storage_source,omitempty"`
	StorageType   string `json:"storage_type,omitempty"`
	ObjectKey     string `json:"object_key,omitempty"`
	URL           string `json:"url,omitempty"`
	ThumbURL      string `json:"thumb_url,omitempty"`
}

type metadataMonthCacheEntry struct {
	exists  bool
	modTime time.Time
	size    int64
	items   map[string]ImageMetadata
}

var (
	imageMetadataMu    sync.RWMutex
	imageMetadataCache = make(map[string]metadataMonthCacheEntry)
)

func SaveImageMetadata(relativePath, originalName string, size int64, from string, uploadedAt time.Time) {
	SaveImageMetadataWithStorage(relativePath, originalName, size, from, uploadedAt, "local", "local", strings.TrimPrefix(relativePath, metadataStoragePrefix()), "", "")
}

func SaveImageMetadataWithStorage(relativePath, originalName string, size int64, from string, uploadedAt time.Time, storageSource, storageType, objectKey, url, thumbURL string) {
	if relativePath == "" || originalName == "" {
		return
	}
	if _, err := ValidateURLPath(relativePath, metadataStoragePrefix()); err != nil {
		log.Printf("[Metadata] invalid image path %q: %v", relativePath, err)
		return
	}
	originalName = OriginalUploadName(originalName)

	metadata := ImageMetadata{
		Path:          relativePath,
		StoredName:    filepath.Base(relativePath),
		OriginalName:  originalName,
		OriginalBase:  strings.TrimSuffix(originalName, filepath.Ext(originalName)),
		UploadedAt:    uploadedAt.Format("2006-01-02 15:04:05"),
		Size:          size,
		From:          from,
		StorageSource: storageSource,
		StorageType:   storageType,
		ObjectKey:     objectKey,
		URL:           url,
		ThumbURL:      thumbURL,
	}
	if metadata.StorageSource == "" {
		metadata.StorageSource = "local"
	}
	if metadata.StorageType == "" {
		metadata.StorageType = "local"
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
	month := metadataMonthFromPath(relativePath)
	metadataPath, err := safeImageMetadataMonthPath(month)
	if err != nil {
		return ImageMetadata{}, false
	}

	items, err := getImageMetadataMonth(metadataPath)
	if err != nil || len(items) == 0 {
		return ImageMetadata{}, false
	}

	imageMetadataMu.RLock()
	metadata, ok := items[relativePath]
	imageMetadataMu.RUnlock()
	if ok {
		metadata = normalizeImageMetadata(metadata)
	}
	return metadata, ok
}

func DeleteImageMetadata(relativePath string) {
	if relativePath == "" {
		return
	}
	month := metadataMonthFromPath(relativePath)
	metadataPath, err := safeImageMetadataMonthPath(month)
	if err != nil {
		return
	}

	imageMetadataMu.Lock()
	defer imageMetadataMu.Unlock()

	items, err := loadImageMetadataMonthLocked(metadataPath)
	if err != nil || len(items) == 0 {
		return
	}
	if _, ok := items[relativePath]; !ok {
		return
	}
	delete(items, relativePath)
	if err := writeImageMetadataMonthLocked(metadataPath, items); err != nil {
		log.Printf("[Metadata] delete failed for %s: %v", relativePath, err)
	}
}

func LoadImageMetadataForDate(datePath string) map[string]ImageMetadata {
	month := metadataMonthFromDatePath(datePath)
	metadataPath, err := safeImageMetadataMonthPath(month)
	if err != nil {
		return map[string]ImageMetadata{}
	}

	items, err := getImageMetadataMonth(metadataPath)
	if err != nil || len(items) == 0 {
		return map[string]ImageMetadata{}
	}

	result := make(map[string]ImageMetadata)
	needle := "/" + strings.Trim(datePath, "/") + "/"
	imageMetadataMu.RLock()
	for p, metadata := range items {
		if strings.Contains(p, needle) {
			result[p] = normalizeImageMetadata(metadata)
		}
	}
	imageMetadataMu.RUnlock()
	return result
}

func ListImageMetadataForDate(datePath string, sortDesc bool) []ImageMetadata {
	items := LoadImageMetadataForDate(datePath)
	list := make([]ImageMetadata, 0, len(items))
	for _, metadata := range items {
		list = append(list, metadata)
	}
	sort.Slice(list, func(i, j int) bool {
		if sortDesc {
			return list[i].UploadedAt > list[j].UploadedAt
		}
		return list[i].UploadedAt < list[j].UploadedAt
	})
	return list
}

func normalizeImageMetadata(metadata ImageMetadata) ImageMetadata {
	if metadata.StorageSource == "" {
		metadata.StorageSource = "local"
	}
	if metadata.StorageType == "" {
		metadata.StorageType = "local"
	}
	if metadata.ObjectKey == "" {
		metadata.ObjectKey = strings.TrimPrefix(metadata.Path, metadataStoragePrefix())
	}
	return metadata
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
	month := metadataMonthFromPath(metadata.Path)
	metadataPath, err := safeImageMetadataMonthPath(month)
	if err != nil {
		return err
	}

	imageMetadataMu.Lock()
	defer imageMetadataMu.Unlock()

	items, err := loadImageMetadataMonthLocked(metadataPath)
	if err != nil {
		return err
	}
	items[metadata.Path] = metadata
	return writeImageMetadataMonthLocked(metadataPath, items)
}

// getImageMetadataMonth returns a cached monthly metadata map using RLock on
// hits, validating against file stat so out-of-band updates or test chdir are
// always observed without re-parsing the JSON on every lookup.
func getImageMetadataMonth(metadataPath string) (map[string]ImageMetadata, error) {
	info, statErr := os.Stat(metadataPath)
	exists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, statErr
	}

	imageMetadataMu.RLock()
	cached, ok := imageMetadataCache[metadataPath]
	if ok {
		if !exists && !cached.exists {
			imageMetadataMu.RUnlock()
			return nil, nil
		}
		if exists && cached.exists && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
			imageMetadataMu.RUnlock()
			return cached.items, nil
		}
	}
	imageMetadataMu.RUnlock()

	imageMetadataMu.Lock()
	defer imageMetadataMu.Unlock()
	return loadImageMetadataMonthLocked(metadataPath)
}

func loadImageMetadataMonthLocked(metadataPath string) (map[string]ImageMetadata, error) {
	info, statErr := os.Stat(metadataPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			empty := make(map[string]ImageMetadata)
			imageMetadataCache[metadataPath] = metadataMonthCacheEntry{exists: false, items: empty}
			return empty, nil
		}
		return nil, statErr
	}

	if cached, ok := imageMetadataCache[metadataPath]; ok && cached.exists && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		return cached.items, nil
	}

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			empty := make(map[string]ImageMetadata)
			imageMetadataCache[metadataPath] = metadataMonthCacheEntry{exists: false, items: empty}
			return empty, nil
		}
		return nil, err
	}
	items := make(map[string]ImageMetadata)
	if len(data) > 0 {
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, err
		}
	}
	imageMetadataCache[metadataPath] = metadataMonthCacheEntry{
		exists:  true,
		modTime: info.ModTime(),
		size:    info.Size(),
		items:   items,
	}
	return items, nil
}

func writeImageMetadataMonthLocked(metadataPath string, items map[string]ImageMetadata) error {
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0755); err != nil {
		return err
	}
	if len(items) == 0 {
		if err := os.Remove(metadataPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		imageMetadataCache[metadataPath] = metadataMonthCacheEntry{exists: false, items: items}
		return nil
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return err
	}
	if info, err := os.Stat(metadataPath); err == nil {
		imageMetadataCache[metadataPath] = metadataMonthCacheEntry{
			exists:  true,
			modTime: info.ModTime(),
			size:    info.Size(),
			items:   items,
		}
	} else {
		delete(imageMetadataCache, metadataPath)
	}
	return nil
}

func safeImageMetadataMonthPath(month string) (string, error) {
	if !isSafeMetadataMonth(month) {
		return "", fmt.Errorf("invalid metadata month")
	}
	year, err := strconv.Atoi(month[:4])
	if err != nil {
		return "", fmt.Errorf("invalid metadata month")
	}
	monthNumber, err := strconv.Atoi(month[5:])
	if err != nil || monthNumber < 1 || monthNumber > 12 {
		return "", fmt.Errorf("invalid metadata month")
	}
	fileName := fmt.Sprintf("%04d-%02d.json", year, monthNumber)
	return filepath.Abs(filepath.Join(imageMetadataDir, fileName))
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
	if len(parts) >= 2 && len(parts[0]) == 4 && len(parts[1]) == 2 && isDigits(parts[0]) && isDigits(parts[1]) {
		return parts[0] + "-" + parts[1]
	}
	return ""
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
