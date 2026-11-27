package service

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
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

// metadataMonthCacheEntry holds an immutable, pre-normalized monthly metadata map.
// Writers replace the map via copy-on-write so readers can safely inspect cached
// maps without holding imageMetadataMu across iterations or cloning every entry.
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

	metadata := normalizeImageMetadata(ImageMetadata{
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
	})
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

	metadata, ok := items[relativePath]
	return metadata, ok
}

// FindImageMetadata looks up metadata for relativePath, trying the path's
// implied month first and falling back across all months for custom folders.
func FindImageMetadata(relativePath string) (ImageMetadata, bool) {
	if metadata, ok := GetImageMetadata(relativePath); ok {
		return metadata, true
	}
	if _, explicit := explicitMetadataMonthFromPath(relativePath); explicit {
		return ImageMetadata{}, false
	}
	return OpenMetadataSnapshot().Get(relativePath)
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
	next := maps.Clone(items)
	delete(next, relativePath)
	if err := writeImageMetadataMonthLocked(metadataPath, next); err != nil {
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
	for p, metadata := range items {
		if strings.Contains(p, needle) {
			result[p] = metadata
		}
	}
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

// MetadataSnapshot provides lock-free, zero-copy read access across every
// cached monthly metadata file without merging all entries into a new map.
type MetadataSnapshot struct {
	byMonth map[string]map[string]ImageMetadata
	months  []map[string]ImageMetadata
}

// OpenMetadataSnapshot collects references to the current immutable monthly
// metadata maps.
func OpenMetadataSnapshot() MetadataSnapshot {
	files, err := os.ReadDir(imageMetadataDir)
	if err != nil || len(files) == 0 {
		return MetadataSnapshot{}
	}
	absDir, err := filepath.Abs(imageMetadataDir)
	if err != nil {
		return MetadataSnapshot{}
	}

	snapshot := MetadataSnapshot{
		byMonth: make(map[string]map[string]ImageMetadata, len(files)),
		months:  make([]map[string]ImageMetadata, 0, len(files)),
	}
	for _, file := range files {
		month, ok := strings.CutSuffix(file.Name(), ".json")
		if !ok || !file.Type().IsRegular() || !isValidMetadataMonth(month) {
			continue
		}
		items, err := getImageMetadataMonth(filepath.Join(absDir, file.Name()))
		if err != nil {
			log.Printf("[Metadata] could not load %s: %v", file.Name(), err)
			continue
		}
		if len(items) == 0 {
			continue
		}
		snapshot.byMonth[month] = items
		snapshot.months = append(snapshot.months, items)
	}
	return snapshot
}

// Get returns the metadata entry for relativePath, checking its implied month first.
func (s MetadataSnapshot) Get(relativePath string) (ImageMetadata, bool) {
	if len(s.months) == 0 {
		return ImageMetadata{}, false
	}
	if month, ok := explicitMetadataMonthFromPath(relativePath); ok {
		if items := s.byMonth[month]; items != nil {
			if metadata, found := items[relativePath]; found {
				return metadata, true
			}
		}
	}
	for _, items := range s.months {
		if metadata, found := items[relativePath]; found {
			return metadata, true
		}
	}
	return ImageMetadata{}, false
}

// ForEach iterates over all metadata entries until fn returns false.
func (s MetadataSnapshot) ForEach(fn func(path string, metadata ImageMetadata) bool) {
	for _, items := range s.months {
		for p, metadata := range items {
			if !fn(p, metadata) {
				return
			}
		}
	}
}

// LoadImageMetadata covers every month, including uploads made to custom folders.
func LoadImageMetadata() map[string]ImageMetadata {
	snapshot := OpenMetadataSnapshot()
	total := 0
	for _, items := range snapshot.months {
		total += len(items)
	}
	result := make(map[string]ImageMetadata, total)
	snapshot.ForEach(func(p string, metadata ImageMetadata) bool {
		result[p] = metadata
		return true
	})
	return result
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
	next := make(map[string]ImageMetadata, len(items)+1)
	maps.Copy(next, items)
	next[metadata.Path] = metadata
	return writeImageMetadataMonthLocked(metadataPath, next)
}

// getImageMetadataMonth returns a cached immutable monthly metadata map using
// RLock on hits, validating against file stat so out-of-band updates or test
// chdir are always observed without re-parsing the JSON on every lookup.
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
			imageMetadataCache[metadataPath] = metadataMonthCacheEntry{exists: false}
			return nil, nil
		}
		return nil, statErr
	}

	if cached, ok := imageMetadataCache[metadataPath]; ok && cached.exists && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		return cached.items, nil
	}

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			imageMetadataCache[metadataPath] = metadataMonthCacheEntry{exists: false}
			return nil, nil
		}
		return nil, err
	}
	items := make(map[string]ImageMetadata)
	if len(data) > 0 {
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, err
		}
		for p, metadata := range items {
			items[p] = normalizeImageMetadata(metadata)
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
		imageMetadataCache[metadataPath] = metadataMonthCacheEntry{exists: false}
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
	if !isValidMetadataMonth(month) {
		return "", fmt.Errorf("invalid metadata month")
	}
	return filepath.Abs(filepath.Join(imageMetadataDir, month+".json"))
}

func explicitMetadataMonthFromPath(relativePath string) (string, bool) {
	parts := strings.Split(strings.Trim(relativePath, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if len(parts[i]) == 4 && len(parts[i+1]) == 2 && isDigits(parts[i]) && isDigits(parts[i+1]) {
			return parts[i] + "-" + parts[i+1], true
		}
	}
	return "", false
}

func metadataMonthFromPath(relativePath string) string {
	if month, ok := explicitMetadataMonthFromPath(relativePath); ok {
		return month
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

func isValidMetadataMonth(month string) bool {
	if len(month) != len("2006-01") || month[4] != '-' {
		return false
	}
	if !isDigits(month[:4]) || !isDigits(month[5:]) {
		return false
	}
	monthNumber, err := strconv.Atoi(month[5:])
	return err == nil && monthNumber >= 1 && monthNumber <= 12
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
