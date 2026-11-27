package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"easyimage/config"
	"easyimage/internal/service"
	"easyimage/internal/storage"
	"easyimage/internal/workspace"

	"github.com/gin-gonic/gin"
)

type workspaceFolder struct {
	workspace.Folder
	BrowseOnly bool `json:"browse_only,omitempty"`
}

func legacyOwner(actor workspace.Identity, owner string) bool {
	return actor.IsAdmin() && (owner == "" || owner == workspace.AdminID)
}

func includeLegacy(c *gin.Context, actor workspace.Identity, owner string) (bool, error) {
	enabled, err := workspaceQueryBool(c, "include_legacy")
	return enabled && legacyOwner(actor, owner), err
}

func legacyStoragePrefix(cfg *config.Config) string {
	return strings.TrimRight(cfg.Path, "/") + "/"
}

func legacyAssetID(source string) string {
	hash := sha256.Sum256([]byte("easyimage-legacy:" + source))
	return hex.EncodeToString(hash[:16])
}

func isHex32(id string) bool {
	if len(id) != 32 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func isValidLegacyFolderQuery(folder string) bool {
	return folder == "" || (folder != "." && fs.ValidPath(folder) && !service.IsInternalStoragePath(folder))
}

func addLegacyFolder(folders map[string]workspaceFolder, name string) {
	for name != "" {
		if _, exists := folders[name]; exists {
			return
		}
		folders[name] = workspaceFolder{
			Folder:     workspace.Folder{OwnerID: workspace.AdminID, Path: name},
			BrowseOnly: !workspace.ValidFolderPath(name),
		}
		name = legacyFolderOf(name)
	}
}

func legacyFolderOf(name string) string {
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		return name[:i]
	}
	return ""
}

// readLegacyFolders discovers legacy folders without stat'ing image files or
// cloning the full metadata map.
func readLegacyFolders(cfg *config.Config) map[string]workspaceFolder {
	folders := make(map[string]workspaceFolder)
	prefix := legacyStoragePrefix(cfg)

	_ = fs.WalkDir(os.DirFS("."+prefix), ".", func(rel string, entry fs.DirEntry, err error) error {
		if err != nil || rel == "." {
			return nil
		}
		logical, isWebP := strings.CutPrefix(rel, "webp/")
		if rel == "webp" && entry.IsDir() {
			return nil
		}
		if service.IsInternalStoragePath(logical) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if !isWebP {
				addLegacyFolder(folders, logical)
			}
			return nil
		}
		if !isWebP {
			return nil
		}
		parent := legacyFolderOf(logical)
		if parent == "" {
			return nil
		}
		if _, exists := folders[parent]; exists {
			return nil
		}
		if entry.Type().IsRegular() && strings.EqualFold(path.Ext(logical), ".webp") {
			addLegacyFolder(folders, parent)
		}
		return nil
	})

	service.OpenMetadataSnapshot().ForEach(func(source string, record service.ImageMetadata) bool {
		if record.StorageType != "s3" || record.URL == "" {
			return true
		}
		rel, ok := strings.CutPrefix(source, prefix)
		if !ok || rel == "." || !fs.ValidPath(rel) || service.IsInternalStoragePath(rel) {
			return true
		}
		addLegacyFolder(folders, legacyFolderOf(rel))
		return true
	})

	return folders
}

// readLegacyImages reconciles existing local files and old S3 metadata without
// modifying either storage system. When folder is non-nil it reads only that
// single directory directly instead of walking the entire storage tree.
func readLegacyImages(cfg *config.Config, folder *string) map[string]workspaceAsset {
	images := make(map[string]workspaceAsset)
	if folder != nil && !isValidLegacyFolderQuery(*folder) {
		return images
	}

	prefix := legacyStoragePrefix(cfg)
	root := "." + prefix
	metaIdx := service.OpenMetadataSnapshot()
	renditions := make(map[string]workspaceAsset)
	webpPaths := make(map[string]string)
	localPaths := make(map[string]bool)

	recordFile := func(rel string, isWebP bool, entry fs.DirEntry) {
		if !entry.Type().IsRegular() {
			return
		}
		logical := rel
		if isWebP {
			logical = strings.TrimPrefix(rel, "webp/")
			if !strings.EqualFold(path.Ext(logical), ".webp") {
				return
			}
		}
		contentType := "image/webp"
		if !isWebP {
			contentType = storage.ContentType(logical)
			if !service.IsImageFile(logical) && !strings.HasPrefix(contentType, "image/") {
				return
			}
		}
		info, err := entry.Info()
		if err != nil || info.Size() == 0 {
			return
		}
		source := prefix + rel
		record, _ := metaIdx.Get(source)
		asset := legacyAsset(cfg, source, legacyFolderOf(logical), contentType, info.Size(), info.ModTime(), record)
		if isWebP {
			renditions[source] = asset
			return
		}
		localPaths[source] = true
		images[asset.ID] = asset
		if !strings.EqualFold(path.Ext(rel), ".webp") {
			webpPaths[asset.ID] = prefix + "webp/" + strings.TrimSuffix(rel, path.Ext(rel)) + ".webp"
		}
	}

	if folder != nil {
		target := *folder
		dirPath := root
		webpDirPath := filepath.Join(root, "webp")
		relPrefix := ""
		if target != "" {
			sub := filepath.FromSlash(target)
			dirPath = filepath.Join(root, sub)
			webpDirPath = filepath.Join(root, "webp", sub)
			relPrefix = target + "/"
		}
		if entries, err := os.ReadDir(dirPath); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				rel := relPrefix + entry.Name()
				if service.IsInternalStoragePath(rel) {
					continue
				}
				recordFile(rel, false, entry)
			}
		}
		if entries, err := os.ReadDir(webpDirPath); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				logical := relPrefix + entry.Name()
				if service.IsInternalStoragePath(logical) {
					continue
				}
				recordFile("webp/"+logical, true, entry)
			}
		}
	} else {
		_ = fs.WalkDir(os.DirFS(root), ".", func(rel string, entry fs.DirEntry, err error) error {
			if err != nil || rel == "." {
				return nil
			}
			logical, isWebP := strings.CutPrefix(rel, "webp/")
			if rel == "webp" && entry.IsDir() {
				return nil
			}
			if service.IsInternalStoragePath(logical) {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			recordFile(rel, isWebP, entry)
			return nil
		})
	}

	used := make(map[string]bool, len(webpPaths))
	for id, rendition := range webpPaths {
		if webp, ok := renditions[rendition]; ok {
			image := images[id]
			image.WebPURL = webp.URL
			images[id] = image
			used[rendition] = true
		}
	}
	for source, asset := range renditions {
		if !used[source] {
			images[asset.ID] = asset
		}
	}

	metaIdx.ForEach(func(source string, record service.ImageMetadata) bool {
		if record.StorageType != "s3" || record.URL == "" || localPaths[source] {
			return true
		}
		rel, ok := strings.CutPrefix(source, prefix)
		if !ok || rel == "." || !fs.ValidPath(rel) || service.IsInternalStoragePath(rel) {
			return true
		}
		parent := legacyFolderOf(rel)
		if folder != nil && parent != *folder {
			return true
		}
		asset := legacyAsset(cfg, source, parent, storage.ContentType(rel), record.Size, time.Time{}, record)
		images[asset.ID] = asset
		return true
	})

	return images
}

// findLegacyImageByID locates a single legacy image by its deterministic ID
// without stat'ing unrelated files or cloning all metadata.
func findLegacyImageByID(cfg *config.Config, id string) (workspaceAsset, bool) {
	if !isHex32(id) {
		return workspaceAsset{}, false
	}
	prefix := legacyStoragePrefix(cfg)
	root := "." + prefix

	var matched workspaceAsset
	var found bool

	_ = fs.WalkDir(os.DirFS(root), ".", func(rel string, entry fs.DirEntry, err error) error {
		if err != nil || rel == "." {
			return nil
		}
		logical, isWebP := strings.CutPrefix(rel, "webp/")
		if rel == "webp" && entry.IsDir() {
			return nil
		}
		if service.IsInternalStoragePath(logical) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		if isWebP && !strings.EqualFold(path.Ext(logical), ".webp") {
			return nil
		}
		source := prefix + rel
		if legacyAssetID(source) != id {
			return nil
		}

		contentType := "image/webp"
		if !isWebP {
			contentType = storage.ContentType(logical)
			if !service.IsImageFile(logical) && !strings.HasPrefix(contentType, "image/") {
				return fs.SkipAll
			}
		}
		info, infoErr := entry.Info()
		if infoErr != nil || info.Size() == 0 {
			return fs.SkipAll
		}
		if isWebP && hasNonWebPSibling(root, logical) {
			return fs.SkipAll
		}

		record, _ := service.FindImageMetadata(source)
		asset := legacyAsset(cfg, source, legacyFolderOf(logical), contentType, info.Size(), info.ModTime(), record)
		if !isWebP && !strings.EqualFold(path.Ext(rel), ".webp") {
			webpRel := "webp/" + strings.TrimSuffix(rel, path.Ext(rel)) + ".webp"
			if webpInfo, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(webpRel))); statErr == nil && webpInfo.Mode().IsRegular() && webpInfo.Size() > 0 {
				webpSource := prefix + webpRel
				webpMeta, _ := service.FindImageMetadata(webpSource)
				asset.WebPURL = imageURLForMetadata(cfg, (&url.URL{Path: webpSource}).EscapedPath(), webpMeta)
			}
		}
		matched = asset
		found = true
		return fs.SkipAll
	})
	if found {
		return matched, true
	}

	service.OpenMetadataSnapshot().ForEach(func(source string, record service.ImageMetadata) bool {
		if record.StorageType != "s3" || record.URL == "" {
			return true
		}
		if legacyAssetID(source) != id {
			return true
		}
		rel, ok := strings.CutPrefix(source, prefix)
		if !ok || rel == "." || !fs.ValidPath(rel) || service.IsInternalStoragePath(rel) {
			return false
		}
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			return false
		}
		matched = legacyAsset(cfg, source, legacyFolderOf(rel), storage.ContentType(rel), record.Size, time.Time{}, record)
		found = true
		return false
	})
	return matched, found
}

// hasNonWebPSibling reports whether a regular non-empty non-WebP image shares
// the same directory and filename stem as a WebP rendition (meaning the WebP
// file is a paired rendition rather than an orphan asset).
func hasNonWebPSibling(root, logicalWebP string) bool {
	parent := legacyFolderOf(logicalWebP)
	stem := strings.TrimSuffix(path.Base(logicalWebP), path.Ext(logicalWebP))
	dirPath := root
	if parent != "" {
		dirPath = filepath.Join(root, filepath.FromSlash(parent))
	}
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		ext := path.Ext(name)
		if strings.EqualFold(ext, ".webp") || strings.TrimSuffix(name, ext) != stem {
			continue
		}
		if !service.IsImageFile(name) && !strings.HasPrefix(storage.ContentType(name), "image/") {
			continue
		}
		if info, infoErr := entry.Info(); infoErr == nil && info.Size() > 0 {
			return true
		}
	}
	return false
}

// hasLegacyFolder checks whether a single legacy folder exists without walking
// unrelated directories or loading all images.
func hasLegacyFolder(cfg *config.Config, folder string) bool {
	if folder == "" || service.IsInternalStoragePath(folder) {
		return false
	}
	prefix := legacyStoragePrefix(cfg)
	root := "." + prefix
	sub := filepath.FromSlash(folder)

	if info, err := os.Stat(filepath.Join(root, sub)); err == nil && info.IsDir() {
		return true
	}

	webpDir := filepath.Join(root, "webp", sub)
	if info, err := os.Stat(webpDir); err == nil && info.IsDir() {
		found := false
		_ = fs.WalkDir(os.DirFS(webpDir), ".", func(rel string, entry fs.DirEntry, err error) error {
			if err != nil || rel == "." || entry.IsDir() {
				return nil
			}
			if entry.Type().IsRegular() && strings.EqualFold(path.Ext(rel), ".webp") {
				found = true
				return fs.SkipAll
			}
			return nil
		})
		if found {
			return true
		}
	}

	folderPrefix := folder + "/"
	found := false
	service.OpenMetadataSnapshot().ForEach(func(source string, record service.ImageMetadata) bool {
		if record.StorageType != "s3" || record.URL == "" {
			return true
		}
		rel, ok := strings.CutPrefix(source, prefix)
		if !ok || rel == "." || !fs.ValidPath(rel) || service.IsInternalStoragePath(rel) {
			return true
		}
		if strings.HasPrefix(rel, folderPrefix) {
			found = true
			return false
		}
		return true
	})
	return found
}

func legacyAsset(cfg *config.Config, source, folder, contentType string, size int64, modifiedAt time.Time, metadata service.ImageMetadata) workspaceAsset {
	name := path.Base(source)
	if metadata.OriginalName != "" {
		name = metadata.OriginalName
	}
	createdAt := modifiedAt.UTC()
	if uploadedAt, err := time.ParseInLocation("2006-01-02 15:04:05", metadata.UploadedAt, time.Local); err == nil {
		createdAt = uploadedAt.UTC()
	}
	item := workspaceAsset{
		Asset: workspace.Asset{
			ID: legacyAssetID(source), OwnerID: workspace.AdminID, Folder: folder,
			Filename: name, ContentType: contentType, SizeBytes: size, CreatedAt: createdAt,
		},
		URL:    imageURLForMetadata(cfg, (&url.URL{Path: source}).EscapedPath(), metadata),
		Legacy: true,
	}
	if strings.EqualFold(path.Ext(source), ".webp") {
		item.WebPURL = item.URL
	}
	return item
}

func (h workspaceHTTP) useLegacyFolder(actor workspace.Identity, owner, folder string) (bool, error) {
	if !legacyOwner(actor, owner) || !workspace.ValidFolderPath(folder) {
		return false, nil
	}
	if !hasLegacyFolder(h.cfg, folder) {
		return false, nil
	}
	_, err := h.store.CreateFolder(actor, owner, folder)
	if errors.Is(err, workspace.ErrConflict) {
		err = nil
	}
	return err == nil, err
}
