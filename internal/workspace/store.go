package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
)

const (
	AdminID            = "admin"
	defaultRuntimePath = "admin/logs/workspace"
)

// RuntimePath keeps the catalog and blobs outside a custom legacy static tree.
// Both locations are inside directories already mounted in Docker Compose.
func RuntimePath(legacyPath string) string {
	legacy := resolvedPath("." + legacyPath)
	data := resolvedPath(defaultRuntimePath)
	if containsPath(legacy, data) || containsPath(data, legacy) {
		return "config/workspace"
	}
	return defaultRuntimePath
}

func resolvedPath(name string) string {
	if path, err := filepath.EvalSymlinks(name); err == nil {
		return path
	}
	parent, base := filepath.Dir(name), filepath.Base(name)
	if path, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(path, base)
	}
	return filepath.Clean(name)
}

func containsPath(root, target string) bool {
	root, rootErr := filepath.Abs(root)
	target, targetErr := filepath.Abs(target)
	if rootErr != nil || targetErr != nil {
		return false
	}
	rel, err := filepath.Rel(root, target)
	return err == nil && (rel == "." || filepath.IsLocal(rel) && rel != "..")
}

var (
	ErrUnauthorized     = errors.New("authentication required")
	ErrForbidden        = errors.New("access denied")
	ErrNotFound         = errors.New("resource not found")
	ErrConflict         = errors.New("resource already exists")
	ErrFolderNotFound   = errors.New("folder does not exist")
	ErrTooLarge         = errors.New("file exceeds the upload size limit")
	ErrUnsupportedImage = errors.New("unsupported image format or filename extension")
	ErrRenderBusy       = errors.New("image transformation capacity is busy")
	ErrWebPUnavailable  = errors.New("WebP encoding requires cwebp on the server")
	ErrCannotTransform  = errors.New("image cannot be decoded or exceeds transformation limits")
)

type ValidationError string

func (err ValidationError) Error() string { return string(err) }

type Settings struct {
	PublicOrigin string `json:"public_origin"`
	PublicPrefix string `json:"public_prefix"`
}

type catalog struct {
	Settings       Settings           `json:"settings"`
	PreviousRoutes []string           `json:"previous_routes,omitempty"`
	AdminTokenHash string             `json:"admin_token_hash,omitempty"`
	Accounts       map[string]account `json:"accounts"`
	Folders        map[string]Folder  `json:"folders"`
	Assets         map[string]Asset   `json:"assets"`
}

type Store struct {
	root          string
	legacyRoot    string
	mu            sync.RWMutex
	catalog       catalog
	usage         map[string]int64
	tokenOwner    map[string]string
	deletedCount  int
	catalogExists bool
	renderers     chan struct{}
}

// Open loads the catalog for a single server process. Temporary files, the
// catalog and image bytes share a volume so writes can be published by rename.
func Open(root, legacyPath string) (*Store, error) {
	s := &Store{
		root:       root,
		legacyRoot: strings.Split(strings.Trim(legacyPath, "/"), "/")[0],
		renderers:  make(chan struct{}, max(2, runtime.NumCPU())),
	}
	s.catalog.Settings.PublicPrefix = "/media"
	if !s.validPrefix(s.catalog.Settings.PublicPrefix) {
		s.catalog.Settings.PublicPrefix = "/image-library"
	}
	data, err := os.ReadFile(filepath.Join(root, "catalog.json"))
	if err == nil {
		s.catalogExists = true
		if err := json.Unmarshal(data, &s.catalog); err != nil {
			return nil, fmt.Errorf("read workspace catalog: %w", err)
		}
		if !s.validPrefix(s.catalog.Settings.PublicPrefix) {
			return nil, fmt.Errorf("workspace public prefix conflicts with an application route: %q", s.catalog.Settings.PublicPrefix)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("open workspace catalog: %w", err)
	}
	if s.catalog.Accounts == nil {
		s.catalog.Accounts = make(map[string]account)
	}
	if s.catalog.Folders == nil {
		s.catalog.Folders = make(map[string]Folder)
	}
	if s.catalog.Assets == nil {
		s.catalog.Assets = make(map[string]Asset)
	}
	if err := s.checkCatalog(); err != nil {
		return nil, err
	}
	s.rebuildIndexes()
	if err == nil {
		s.cleanDeletedLocked()
		s.cleanOrphans()
	}
	return s, nil
}

func (s *Store) checkCatalog() error {
	for id, user := range s.catalog.Accounts {
		if !isID(id) || user.ID != id || !usernamePattern.MatchString(user.Username) || user.AuthVersion == 0 {
			return fmt.Errorf("invalid user in workspace catalog")
		}
	}
	knownOwner := func(id string) bool {
		_, exists := s.catalog.Accounts[id]
		return exists || id == AdminID
	}
	for key, folder := range s.catalog.Folders {
		if !knownOwner(folder.OwnerID) || folder.Path == "" || !ValidFolderPath(folder.Path) || key != folderKey(folder.OwnerID, folder.Path) {
			return fmt.Errorf("invalid folder in workspace catalog")
		}
	}
	for id, asset := range s.catalog.Assets {
		if !isID(id) || asset.ID != id || !knownOwner(asset.OwnerID) || !ValidFolderPath(asset.Folder) ||
			!strings.HasPrefix(asset.ContentType, "image/") || asset.SizeBytes <= 0 {
			return fmt.Errorf("invalid image in workspace catalog")
		}
		if !asset.Deleted && asset.ImageID != "" {
			parent, exists := s.catalog.Assets[asset.ImageID]
			if !exists || parent.Deleted || parent.ImageID != "" || parent.OwnerID != asset.OwnerID {
				return fmt.Errorf("invalid variant in workspace catalog")
			}
		}
	}
	return nil
}

func (s *Store) rebuildTokenIndexLocked() {
	s.tokenOwner = make(map[string]string)
	if hash := s.catalog.AdminTokenHash; hash != "" {
		s.tokenOwner[hash] = AdminID
	}
	for id, user := range s.catalog.Accounts {
		if !user.Disabled && user.TokenHash != "" {
			s.tokenOwner[user.TokenHash] = id
		}
	}
}

func (s *Store) rebuildIndexes() {
	s.usage = make(map[string]int64)
	s.deletedCount = 0
	for _, asset := range s.catalog.Assets {
		s.usage[asset.OwnerID] += asset.SizeBytes
		if asset.Deleted {
			s.deletedCount++
		}
	}
	s.rebuildTokenIndexLocked()
}

func (s *Store) persistCatalogLocked(next catalog) error {
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.root, ".catalog-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	err = json.NewEncoder(file).Encode(next)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(file.Name(), filepath.Join(s.root, "catalog.json")); err != nil {
		return err
	}
	s.catalog = next
	s.catalogExists = true
	return nil
}

func (s *Store) saveLocked(next catalog) error {
	if err := s.persistCatalogLocked(next); err != nil {
		return err
	}
	s.rebuildIndexes()
	return nil
}

func (s *Store) stage() (*os.File, error) {
	dir := filepath.Join(s.root, "staging")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, "image-*.tmp")
}

// A process can stop after writing a blob and before publishing its catalog
// entry. Clean up only the workspace's own temporary files and unpublished blobs.
func (s *Store) cleanOrphans() {
	for _, pattern := range []string{".catalog-*.tmp", "staging/image-*.tmp"} {
		matches, _ := filepath.Glob(filepath.Join(s.root, pattern))
		for _, name := range matches {
			_ = os.Remove(name)
		}
	}
	root := filepath.Join(s.root, "files")
	_ = filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() || filepath.Base(filepath.Dir(name)) != ".files" || !isID(entry.Name()) {
			return nil
		}
		if _, exists := s.catalog.Assets[entry.Name()]; !exists {
			_ = os.Remove(name)
		}
		return nil
	})
}

func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.catalog.Settings
}

func (s *Store) UpdateSettings(actor Identity, settings Settings) (Settings, error) {
	if !actor.IsAdmin() {
		return Settings{}, ErrForbidden
	}
	settings.PublicPrefix = strings.TrimSpace(settings.PublicPrefix)
	settings.PublicOrigin = strings.TrimRight(strings.TrimSpace(settings.PublicOrigin), "/")
	if !s.validPrefix(settings.PublicPrefix) {
		return Settings{}, ValidationError("public_prefix must be a lowercase URL path outside application and legacy image routes")
	}
	if settings.PublicOrigin != "" {
		u, err := url.Parse(settings.PublicOrigin)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" ||
			u.User != nil || u.Opaque != "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(settings.PublicOrigin, "#") {
			return Settings{}, ValidationError("public_origin must be an HTTP(S) origin without a path, query, or credentials")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.catalog
	old := next.Settings.PublicPrefix
	if old != settings.PublicPrefix && !slices.Contains(next.PreviousRoutes, old) {
		next.PreviousRoutes = append(slices.Clone(next.PreviousRoutes), old)
	}
	next.Settings = settings
	if err := s.persistCatalogLocked(next); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

var deliveryPrefix = regexp.MustCompile(`^(/[a-z0-9][a-z0-9_-]*)+$`)

func (s *Store) validPrefix(prefix string) bool {
	if len(prefix) > 80 || !deliveryPrefix.MatchString(prefix) {
		return false
	}
	switch strings.Split(prefix[1:], "/")[0] {
	case s.legacyRoot, "api", "app", "admin", "public", "install", "workspace", "logout":
		return false
	default:
		return true
	}
}

func (s *Store) DeliveryID(path string) string {
	index := strings.LastIndexByte(path, '/')
	if index <= 0 || !isID(path[index+1:]) {
		return ""
	}
	prefix, id := path[:index], path[index+1:]
	s.mu.RLock()
	defer s.mu.RUnlock()
	if prefix == s.catalog.Settings.PublicPrefix || slices.Contains(s.catalog.PreviousRoutes, prefix) {
		return id
	}
	return ""
}

func (s *Store) CollectDeleted() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanDeletedLocked()
}

func (s *Store) cleanDeletedLocked() {
	if s.deletedCount == 0 {
		return
	}
	var remaining map[string]Asset
	for id, asset := range s.catalog.Assets {
		if !asset.Deleted {
			continue
		}
		err := os.Remove(s.assetPath(asset))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("Could not remove workspace image %s: %v", id, err)
			continue
		}
		if remaining == nil {
			remaining = maps.Clone(s.catalog.Assets)
		}
		delete(remaining, id)
	}
	if remaining != nil {
		next := s.catalog
		next.Assets = remaining
		if err := s.saveLocked(next); err != nil {
			log.Printf("Could not release workspace storage after deletion: %v", err)
		}
	}
}

func randomHex(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func isID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, ch := range id {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}
