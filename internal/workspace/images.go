package workspace

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"easyimage/internal/service"
)

type Folder struct {
	OwnerID   string    `json:"owner_id"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

type Asset struct {
	ID          string            `json:"id"`
	OwnerID     string            `json:"owner_id"`
	Folder      string            `json:"folder"`
	Filename    string            `json:"filename"`
	ContentType string            `json:"content_type"`
	SizeBytes   int64             `json:"size_bytes"`
	Width       int               `json:"width,omitempty"`
	Height      int               `json:"height,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	ImageID     string            `json:"image_id,omitempty"`
	Transform   *TransformOptions `json:"transform,omitempty"`
	Deleted     bool              `json:"deleted,omitempty"`
}

func ValidFolderPath(path string) bool {
	if path == "" {
		return true
	}
	if len(path) > 200 || !utf8.ValidString(path) {
		return false
	}
	segments := strings.Split(path, "/")
	if len(segments) > 12 {
		return false
	}
	for _, name := range segments {
		if name == "" || len(name) > 80 || !filepath.IsLocal(name) || strings.TrimSpace(name) != name ||
			strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") ||
			strings.ContainsAny(name, `\:*?"<>|%`) || strings.ContainsFunc(name, unicode.IsControl) {
			return false
		}
		base, _, _ := strings.Cut(strings.ToUpper(name), ".")
		switch base {
		case "CON", "PRN", "AUX", "NUL", "COM0", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
			"LPT0", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
			return false
		}
	}
	return true
}

func (s *Store) Folders(actor Identity, ownerID string) ([]Folder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ownerID, err := s.ownerLocked(actor, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]Folder, 0)
	for _, folder := range s.catalog.Folders {
		if folder.OwnerID == ownerID {
			out = append(out, folder)
		}
	}
	slices.SortFunc(out, func(a, b Folder) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

func (s *Store) CreateFolder(actor Identity, ownerID, path string) (Folder, error) {
	if path == "" || !ValidFolderPath(path) {
		return Folder{}, ValidationError("path must be a relative folder path, such as projects/summer")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ownerID, err := s.ownerLocked(actor, ownerID)
	if err != nil {
		return Folder{}, err
	}
	key := folderKey(ownerID, path)
	if _, ok := s.catalog.Folders[key]; ok {
		return Folder{}, ErrConflict
	}
	added := make([]string, 0, 4)
	now := time.Now().UTC()
	var folder Folder
	var ancestor string
	for _, name := range strings.Split(path, "/") {
		if ancestor != "" {
			ancestor += "/"
		}
		ancestor += name
		ancestorKey := folderKey(ownerID, ancestor)
		if existing, ok := s.catalog.Folders[ancestorKey]; ok {
			folder = existing
			continue
		}
		folder = Folder{OwnerID: ownerID, Path: ancestor, CreatedAt: now}
		s.catalog.Folders[ancestorKey] = folder
		added = append(added, ancestorKey)
	}
	if err := os.MkdirAll(s.folderPath(ownerID, path), 0700); err != nil {
		for _, k := range added {
			delete(s.catalog.Folders, k)
		}
		return Folder{}, err
	}
	if err := s.persistCatalogLocked(s.catalog); err != nil {
		for _, k := range added {
			delete(s.catalog.Folders, k)
		}
		return Folder{}, err
	}
	return folder, nil
}

func folderKey(ownerID, folder string) string { return ownerID + "/" + folder }

func (s *Store) folderPath(ownerID, folder string) string {
	return filepath.Join(s.root, "files", ownerID, filepath.FromSlash(folder))
}

func (s *Store) assetPath(asset Asset) string {
	return filepath.Join(s.folderPath(asset.OwnerID, asset.Folder), ".files", asset.ID)
}

func (s *Store) validateTargetLocked(ownerID, folder string, size int64) error {
	if !ValidFolderPath(folder) {
		return ValidationError("folder must be a relative path")
	}
	if _, exists := s.catalog.Folders[folderKey(ownerID, folder)]; folder != "" && !exists {
		return ErrFolderNotFound
	}
	quota, used := s.catalog.Accounts[ownerID].QuotaBytes, s.usage[ownerID]
	remaining := quota - used
	if quota != 0 && (remaining <= 0 || size > remaining) {
		return &QuotaError{QuotaBytes: quota, UsedBytes: used, RequestedBytes: size}
	}
	return nil
}

func (s *Store) Upload(actor Identity, ownerID, folder, filename string, body io.Reader, size, maxSize int64) (Asset, error) {
	if size <= 0 {
		return Asset{}, ValidationError("image must not be empty")
	}
	if size > maxSize {
		return Asset{}, ErrTooLarge
	}
	filename = service.OriginalUploadName(filename)
	if len(filename) > 255 || !utf8.ValidString(filename) || strings.ContainsFunc(filename, unicode.IsControl) {
		return Asset{}, ValidationError("invalid image filename")
	}

	s.mu.Lock()
	s.cleanDeletedLocked()
	ownerID, err := s.ownerLocked(actor, ownerID)
	if err == nil {
		err = s.validateTargetLocked(ownerID, folder, size)
	}
	s.mu.Unlock()
	if err != nil {
		return Asset{}, err
	}

	staged, err := s.stage()
	if err != nil {
		return Asset{}, err
	}
	defer os.Remove(staged.Name())
	defer staged.Close()
	written, err := io.Copy(staged, io.LimitReader(body, size+1))
	if err != nil {
		return Asset{}, err
	}
	if written != size {
		return Asset{}, ValidationError("image size does not match the upload")
	}
	contentType, width, height, err := inspectOriginal(staged, filename, size)
	if err != nil {
		return Asset{}, err
	}
	if err := staged.Close(); err != nil {
		return Asset{}, err
	}
	asset := Asset{OwnerID: ownerID, Folder: folder, Filename: filename, ContentType: contentType, SizeBytes: size, Width: width, Height: height}
	return s.commitAsset(actor, asset, staged.Name())
}

func (s *Store) CreateVariant(ctx context.Context, actor Identity, imageID string, folder *string, opts TransformOptions) (Asset, error) {
	s.mu.RLock()
	original, err := s.findImageLocked(actor, imageID)
	if err == nil && folder == nil {
		folder = &original.Folder
	}
	if err == nil {
		// Reject a full quota before encoding. The variant's size is unknown here.
		err = s.validateTargetLocked(original.OwnerID, *folder, 0)
	}
	s.mu.RUnlock()
	if err != nil {
		return Asset{}, err
	}
	file, err := os.Open(s.assetPath(original))
	if errors.Is(err, fs.ErrNotExist) {
		return Asset{}, ErrNotFound
	}
	if err != nil {
		return Asset{}, err
	}
	defer file.Close()
	staged, err := s.Render(ctx, file, original.ContentType, opts)
	if err != nil {
		return Asset{}, err
	}
	defer staged.Remove()
	if err := staged.File.Close(); err != nil {
		return Asset{}, err
	}

	base := strings.TrimSuffix(original.Filename, filepath.Ext(original.Filename))
	filename := fmt.Sprintf("%s-%dx%d.%s", base, staged.Width, staged.Height, staged.Options.Format)
	if len(filename) > 255 {
		filename = fmt.Sprintf("variant-%dx%d.%s", staged.Width, staged.Height, staged.Options.Format)
	}
	asset := Asset{
		OwnerID: original.OwnerID, Folder: *folder, Filename: filename, ContentType: staged.ContentType,
		SizeBytes: staged.SizeBytes, Width: staged.Width, Height: staged.Height, ImageID: imageID, Transform: &staged.Options,
	}
	return s.commitAsset(actor, asset, staged.File.Name())
}

func (s *Store) commitAsset(actor Identity, asset Asset, stagePath string) (Asset, error) {
	id, err := randomHex(16)
	if err != nil {
		return Asset{}, err
	}
	asset.ID = id
	asset.CreatedAt = time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.ownerLocked(actor, asset.OwnerID); err != nil {
		return Asset{}, err
	}
	if asset.ImageID != "" {
		if _, err := s.findImageLocked(actor, asset.ImageID); err != nil {
			return Asset{}, err
		}
	}
	s.cleanDeletedLocked()
	if err := s.validateTargetLocked(asset.OwnerID, asset.Folder, asset.SizeBytes); err != nil {
		return Asset{}, err
	}
	if _, exists := s.catalog.Assets[id]; exists {
		return Asset{}, ErrConflict
	}
	name := s.assetPath(asset)
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return Asset{}, err
	}
	if err := os.Rename(stagePath, name); err != nil {
		return Asset{}, err
	}
	s.catalog.Assets[id] = asset
	if err := s.persistCatalogLocked(s.catalog); err != nil {
		delete(s.catalog.Assets, id)
		_ = os.Remove(name)
		return Asset{}, err
	}
	s.usage[asset.OwnerID] += asset.SizeBytes
	return asset, nil
}

// LookupPublic looks up an active asset by opaque ID without touching the filesystem.
func (s *Store) LookupPublic(id string) (Asset, error) {
	s.mu.RLock()
	asset, exists := s.catalog.Assets[id]
	s.mu.RUnlock()
	if !exists || asset.Deleted {
		return Asset{}, ErrNotFound
	}
	return asset, nil
}

// OpenAsset opens the underlying file for a previously looked-up asset.
func (s *Store) OpenAsset(asset Asset) (*os.File, error) {
	file, err := os.Open(s.assetPath(asset))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return file, err
}

// OpenPublic uses opaque asset IDs only. Filesystem paths and account names are
// never supplied by delivery requests.
func (s *Store) OpenPublic(id string) (Asset, *os.File, error) {
	asset, err := s.LookupPublic(id)
	if err != nil {
		return Asset{}, nil, err
	}
	file, err := s.OpenAsset(asset)
	return asset, file, err
}

type ImageFilter struct {
	OwnerID         string
	Folder          *string
	Cursor          string
	Limit           int
	IncludeVariants bool
	// Legacy is a read-only inventory for the administrator's own library.
	// A nil slice keeps the workspace-only view.
	Legacy []Asset
}

func (s *Store) Images(actor Identity, filter ImageFilter) ([]Asset, string, error) {
	if filter.Limit < 1 || filter.Limit > 100 {
		return nil, "", ValidationError("limit must be between 1 and 100")
	}
	after, err := parseCursor(filter.Cursor)
	if err != nil {
		return nil, "", err
	}

	s.mu.RLock()
	ownerID, err := s.ownerLocked(actor, filter.OwnerID)
	includeLegacy := filter.Legacy != nil && actor.IsAdmin() && ownerID == AdminID
	if err == nil && filter.Folder != nil && !ValidFolderPath(*filter.Folder) {
		if !includeLegacy || *filter.Folder == "." || !fs.ValidPath(*filter.Folder) {
			err = ValidationError("folder must be a relative path")
		}
	}
	if err != nil {
		s.mu.RUnlock()
		return nil, "", err
	}
	items := make([]Asset, 0)
	add := func(asset Asset) {
		if asset.Deleted || (!filter.IncludeVariants && asset.ImageID != "") || asset.OwnerID != ownerID ||
			(filter.Folder != nil && asset.Folder != *filter.Folder) {
			return
		}
		if after.ID == "" || assetOrder(asset, after) > 0 {
			items = append(items, asset)
		}
	}
	for _, asset := range s.catalog.Assets {
		add(asset)
	}
	s.mu.RUnlock()
	if includeLegacy {
		for _, asset := range filter.Legacy {
			add(asset)
		}
	}

	slices.SortFunc(items, assetOrder)
	if len(items) <= filter.Limit {
		return items, "", nil
	}
	last := items[filter.Limit-1]
	next := base64.RawURLEncoding.EncodeToString([]byte(last.CreatedAt.Format(time.RFC3339Nano) + "/" + last.ID))
	return items[:filter.Limit], next, nil
}

func parseCursor(cursor string) (Asset, error) {
	if cursor == "" {
		return Asset{}, nil
	}
	if len(cursor) <= 96 {
		if data, err := base64.RawURLEncoding.DecodeString(cursor); err == nil {
			rawTime, id, _ := strings.Cut(string(data), "/")
			if createdAt, err := time.Parse(time.RFC3339Nano, rawTime); err == nil && isID(id) {
				return Asset{ID: id, CreatedAt: createdAt}, nil
			}
		}
	}
	return Asset{}, ValidationError("invalid image list cursor")
}

func assetOrder(a, b Asset) int {
	if order := b.CreatedAt.Compare(a.CreatedAt); order != 0 {
		return order
	}
	return strings.Compare(b.ID, a.ID)
}

func (s *Store) Image(actor Identity, id string) (Asset, []Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	original, err := s.findImageLocked(actor, id)
	if err != nil {
		return Asset{}, nil, err
	}
	variants := make([]Asset, 0)
	for _, asset := range s.catalog.Assets {
		if !asset.Deleted && asset.ImageID == id {
			variants = append(variants, asset)
		}
	}
	slices.SortFunc(variants, assetOrder)
	return original, variants, nil
}

func (s *Store) findImageLocked(actor Identity, id string) (Asset, error) {
	if !s.authorizedLocked(actor) {
		return Asset{}, ErrUnauthorized
	}
	asset, ok := s.catalog.Assets[id]
	if !ok || asset.Deleted || asset.ImageID != "" || (!actor.IsAdmin() && asset.OwnerID != actor.ID) {
		return Asset{}, ErrNotFound
	}
	return asset, nil
}

func (s *Store) Delete(actor Identity, imageID, variantID string) error {
	s.mu.Lock()
	image, err := s.findImageLocked(actor, imageID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	var victims []Asset
	if variantID != "" {
		variant, ok := s.catalog.Assets[variantID]
		if !ok || variant.Deleted || variant.ImageID != imageID {
			s.mu.Unlock()
			return ErrNotFound
		}
		victims = append(victims, variant)
	} else {
		victims = append(victims, image)
		for _, variant := range s.catalog.Assets {
			if !variant.Deleted && variant.ImageID == imageID {
				victims = append(victims, variant)
			}
		}
	}

	paths := make([]string, 0, len(victims))
	for _, v := range victims {
		paths = append(paths, s.assetPath(v))
		delete(s.catalog.Assets, v.ID)
	}
	if err := s.persistCatalogLocked(s.catalog); err != nil {
		for _, v := range victims {
			s.catalog.Assets[v.ID] = v
		}
		s.mu.Unlock()
		return err
	}
	for _, v := range victims {
		s.usage[v.OwnerID] -= v.SizeBytes
	}
	s.mu.Unlock()

	// Unlink files outside the catalog lock; if removal fails transiently,
	// startup orphan cleanup still reclaims the unpublished blob.
	for _, p := range paths {
		_ = os.Remove(p)
	}
	return nil
}

func inspectOriginal(file *os.File, filename string, size int64) (string, int, int, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".svg" {
		if size > 1<<20 {
			return "", 0, 0, ErrTooLarge
		}
		data, err := io.ReadAll(io.NewSectionReader(file, 0, size))
		if err != nil {
			return "", 0, 0, err
		}
		var root struct{ XMLName xml.Name }
		if !service.IsSafeSVGContent(data) || xml.Unmarshal(data, &root) != nil || root.XMLName.Local != "svg" {
			return "", 0, 0, ErrUnsupportedImage
		}
		return "image/svg+xml", 0, 0, nil
	}

	expected := map[string]string{
		".jpeg": "image/jpeg", ".jpg": "image/jpeg", ".jfif": "image/jpeg", ".png": "image/png",
		".gif": "image/gif", ".webp": "image/webp", ".bmp": "image/bmp", ".ico": "image/x-icon",
		".tif": "image/tiff", ".tiff": "image/tiff",
	}[ext]
	if expected == "" {
		return "", 0, 0, ErrUnsupportedImage
	}
	var header [512]byte
	n, err := file.ReadAt(header[:], 0)
	if err != nil && err != io.EOF {
		return "", 0, 0, err
	}
	detected := http.DetectContentType(header[:n])
	if bytes.HasPrefix(header[:n], []byte("II*\x00")) || bytes.HasPrefix(header[:n], []byte("MM\x00*")) {
		detected = "image/tiff"
	}
	if detected != expected {
		return "", 0, 0, ErrUnsupportedImage
	}
	if expected == "image/x-icon" || expected == "image/tiff" {
		return expected, 0, 0, nil
	}
	img, _, err := image.DecodeConfig(io.NewSectionReader(file, 0, size))
	if err != nil || img.Width <= 0 || img.Height <= 0 {
		return "", 0, 0, ErrUnsupportedImage
	}
	return expected, img.Width, img.Height, nil
}
