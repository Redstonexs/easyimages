package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"easyimage/config"
	"easyimage/internal/storage"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/hkdf"
)

// ==================== Session 管理 ====================

// session 存储结构
type sessionData struct {
	User      string
	IsAdmin   bool
	CreatedAt time.Time
}

var (
	sessionStore sync.Map // map[token]sessionData
)

const sessionMaxAge = 14 * 24 * time.Hour // 14天过期

var thumbnailLocks sync.Map // map[thumbPath]*sync.Mutex

func init() {
	// 定时清理过期 session（每小时一次），避免在登录路径上遍历整个 sync.Map
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cleanExpiredSessions()
		}
	}()
}

// generateSessionToken 生成安全的随机 session token
func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// cleanExpiredSessions 清理过期 session（在设置新 session 时触发）
func cleanExpiredSessions() {
	now := time.Now()
	sessionStore.Range(func(key, value interface{}) bool {
		if s, ok := value.(sessionData); ok {
			if now.Sub(s.CreatedAt) > sessionMaxAge {
				sessionStore.Delete(key)
			}
		}
		return true
	})
}

// ==================== 登录速率限制 ====================

type loginAttempt struct {
	Count     int
	FirstTime time.Time
}

var (
	loginAttempts   = make(map[string]*loginAttempt)
	loginAttemptsMu sync.Mutex
)

const (
	maxLoginAttempts   = 5               // 最大尝试次数
	loginLockoutWindow = 5 * time.Minute // 锁定窗口
)

// CheckLoginRateLimit 检查登录速率限制，返回 true 表示允许登录
func CheckLoginRateLimit(ip string) bool {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()

	attempt, exists := loginAttempts[ip]
	if !exists {
		return true
	}

	// 超过窗口时间，重置计数
	if time.Since(attempt.FirstTime) > loginLockoutWindow {
		delete(loginAttempts, ip)
		return true
	}

	return attempt.Count < maxLoginAttempts
}

// RecordFailedLogin 记录登录失败
func RecordFailedLogin(ip string) {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()

	attempt, exists := loginAttempts[ip]
	if !exists || time.Since(attempt.FirstTime) > loginLockoutWindow {
		loginAttempts[ip] = &loginAttempt{
			Count:     1,
			FirstTime: time.Now(),
		}
		return
	}
	attempt.Count++
}

// ResetLoginAttempts 登录成功后重置计数
func ResetLoginAttempts(ip string) {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	delete(loginAttempts, ip)
}

// IsLoggedIn 检查是否已登录
func IsLoggedIn(c *gin.Context) bool {
	token, err := c.Cookie("session")
	if err != nil || token == "" {
		return false
	}

	value, ok := sessionStore.Load(token)
	if !ok {
		return false
	}

	session, ok := value.(sessionData)
	if !ok {
		return false
	}

	// 检查 session 是否过期
	if time.Since(session.CreatedAt) > sessionMaxAge {
		sessionStore.Delete(token)
		return false
	}

	return true
}

// IsAdmin 检查是否管理员
func IsAdmin(c *gin.Context) bool {
	token, err := c.Cookie("session")
	if err != nil || token == "" {
		return false
	}

	value, ok := sessionStore.Load(token)
	if !ok {
		return false
	}

	session, ok := value.(sessionData)
	if !ok {
		return false
	}

	// 检查 session 是否过期
	if time.Since(session.CreatedAt) > sessionMaxAge {
		sessionStore.Delete(token)
		return false
	}

	return session.IsAdmin
}

// SetAdminSession 设置管理员会话
func SetAdminSession(c *gin.Context, user string) {
	cfg := config.Get()

	// 生成安全的随机 token
	token, err := generateSessionToken()
	if err != nil {
		return
	}

	// 存储 session 数据
	sessionStore.Store(token, sessionData{
		User:      user,
		IsAdmin:   user == cfg.User,
		CreatedAt: time.Now(),
	})

	// 设置HttpOnly标志，防止客户端脚本访问cookie
	// secure参数根据域名是否为https来判断
	secure := strings.HasPrefix(cfg.Domain, "https")
	c.SetCookie("session", token, 3600*24*14, "/", "", secure, true)
}

// HashPassword 使用bcrypt对密码进行哈希
func HashPassword(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		// 如果bcrypt失败，返回空字符串
		return ""
	}
	return string(hash)
}

// CheckPassword 检查密码是否匹配
// 支持bcrypt和SHA256（向后兼容PHP版本）
func CheckPassword(password, hashedPassword string) bool {
	// 检查是否是bcrypt格式（以$2a$或$2b$开头）
	if len(hashedPassword) >= 4 && hashedPassword[0] == '$' && hashedPassword[2] == '$' {
		// 尝试bcrypt验证
		if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)); err == nil {
			return true
		}
	}

	// 旧格式验证：用于兼容从PHP版本迁移的密码（SHA256格式）
	legacyHasher := NewLegacyPasswordHasher()
	return legacyHasher.VerifyHash(password, hashedPassword)
}

// ValidateLogin 验证登录并返回详细的错误信息
func ValidateLogin(user, password string, cfg *config.Config) (bool, string) {
	// 统一错误消息，防止用户名枚举
	const failMsg = "用户名或密码错误"

	// 检查用户名（使用常量时间比较防止时序攻击）
	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(cfg.User)) == 1

	// 总是检查密码，防止时序差异泄露用户名是否存在
	passMatch := CheckPassword(password, cfg.Password)

	if !userMatch || !passMatch {
		return false, failMsg
	}

	return true, "登录成功"
}

// GenerateFileName 生成文件名
func GenerateFileName(source string, imgName string) string {
	switch imgName {
	case "source":
		// source 模式使用原始文件名，但必须清理路径分隔符防止目录穿越
		return sanitizeFilename(source)
	case "date":
		return time.Now().Format("150405")
	case "unix":
		return fmt.Sprintf("%d", time.Now().Unix())
	case "md5":
		hash := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		return hex.EncodeToString(hash[:16])
	case "uuid":
		return generateUUID()
	default:
		// default: 时间+随机数转36进制
		randNum, err := rand.Int(rand.Reader, big.NewInt(9000))
		if err != nil {
			// crypto/rand 失败时降级为仅时间戳
			return time.Now().Format("150405070405")
		}
		return fmt.Sprintf("%s%04d", time.Now().Format("150405"), randNum.Int64()+1000)
	}
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// deriveKey 使用HKDF从密钥材料派生安全的加密密钥
func deriveKey(secret []byte, salt []byte) ([]byte, error) {
	key := make([]byte, 16) // AES-128需要16字节密钥
	hkdfReader := hkdf.New(sha256.New, secret, salt, []byte("easyimage-encryption-key"))
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}
	return key, nil
}

// EncryptHash 加密路径
func EncryptHash(data string, secret string) string {
	salt := []byte("easyimage-url-encrypt")
	key, err := deriveKey([]byte(secret), salt)
	if err != nil {
		return ""
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return ""
	}

	plaintext := []byte(data)
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	rand.Read(iv)

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], plaintext)

	return base64.URLEncoding.EncodeToString(ciphertext)
}

// DecryptHash 解密路径
func DecryptHash(encrypted string, secret string) (string, error) {
	salt := []byte("easyimage-url-encrypt")
	key, err := deriveKey([]byte(secret), salt)
	if err != nil {
		return "", err
	}

	ciphertext, err := base64.URLEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	if len(ciphertext) < aes.BlockSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)

	return string(ciphertext), nil
}

// EncryptHideKey 加密隐藏路径
func EncryptHideKey(path string, hideKey string) string {
	return EncryptHash(path, hideKey)
}

// DecryptHideKey 解密隐藏路径
func DecryptHideKey(key string, hideKey string) (string, error) {
	return DecryptHash(key, hideKey)
}

// GetFileList 获取文件列表
func GetFileList(pattern string, sortOrder int) []string {
	files, _ := GetFileListLimited(pattern, sortOrder, 0)
	return files
}

// GetFileListLimited returns matching file names and the total match count.
// It avoids building a full file-name slice when callers only need the first N
// entries, which keeps large public/history directories from dominating memory
// and template render time.
func GetFileListLimited(pattern string, sortOrder, limit int) ([]string, int) {
	if err := validateGlobPattern(pattern); err != nil {
		return nil, 0
	}
	dir, filePattern := filepath.Split(pattern)
	if dir == "" || filePattern == "" {
		return nil, 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0
	}

	files := make([]string, 0)
	if limit > 0 {
		files = make([]string, 0, limit)
	} else {
		files = make([]string, 0, len(entries))
	}
	appendMatch := func(name string) {
		if limit <= 0 || len(files) < limit {
			files = append(files, name)
		}
	}

	total := 0
	if sortOrder == 1 {
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			if entry.IsDir() {
				continue
			}
			matched, err := filepath.Match(filePattern, entry.Name())
			if err != nil {
				return nil, 0
			}
			if matched {
				total++
				appendMatch(entry.Name())
			}
		}
		return files, total
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matched, err := filepath.Match(filePattern, entry.Name())
		if err != nil {
			return nil, 0
		}
		if matched {
			total++
			appendMatch(entry.Name())
		}
	}
	return files, total
}

// internalDirs 系统内部目录集合，不应出现在文件浏览和 URL 列表中。
// 这些目录由系统自动管理（缩略图缓存、回收站、上传分片等），非用户上传内容。
var internalDirs = map[string]bool{
	"cache":   true,
	"suspic":  true,
	"recycle": true,
	"chunks":  true,
	"admin":   true,
	"webp":    true,
}

// isInternalPath 检查路径的顶层目录是否为系统内部目录。
// 例如 "cache/thumb_xxx" → true, "2026/05/08/file.jpg" → false
func isInternalPath(relPath string) bool {
	if internalDirs[relPath] {
		return true
	}
	if i := strings.Index(relPath, "/"); i > 0 {
		return internalDirs[relPath[:i]]
	}
	return false
}

// GetDirList 获取目录列表（排除系统内部目录）
func GetDirList(dirPath string) []string {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() && !internalDirs[entry.Name()] {
			dirs = append(dirs, entry.Name()+"/")
		}
	}
	return dirs
}

// GetFileCount 获取文件数量
func GetFileCount(pattern string) int {
	// 验证模式字符串安全性
	if err := validateGlobPattern(pattern); err != nil {
		return 0
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return 0
	}

	// filepath.Glob 不会返回目录，直接返回匹配数
	count := 0
	for _, match := range matches {
		if err := validateMatchedPath(match); err != nil {
			continue
		}
		count++
	}
	return count
}

// GetFileCountRecursive 递归获取文件数量
func GetFileCountRecursive(dir string) int {
	count := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}

// CountFilesByExt 按扩展名统计文件
func CountFilesByExt(dir, ext string) int {
	count := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), "."+ext) {
			count++
		}
		return nil
	})
	return count
}

// GetFileListRecursive 递归获取目录下所有文件的相对路径（跳过系统内部目录）
func GetFileListRecursive(dir string) []string {
	var files []string
	// 确保dir以/结尾
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// 跳过系统内部目录（cache、suspic、recycle、chunks、admin）
		if info.IsDir() && path != dir {
			rel, err := filepath.Rel(dir, path)
			if err == nil && isInternalPath(filepath.ToSlash(rel)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			// 返回相对于dir的路径
			rel, err := filepath.Rel(dir, path)
			if err == nil {
				files = append(files, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	return files
}

// imageExts 图片文件扩展名集合（包级变量，避免每次调用 IsImageFile 都分配新 map）
var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".bmp": true, ".webp": true, ".ico": true, ".jfif": true,
	".tif": true, ".tiff": true, ".tga": true, ".svg": true,
}

// IsImageFile 检查文件是否是图片
func IsImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return imageExts[ext]
}

// validateGlobPattern 验证glob模式字符串安全性
func validateGlobPattern(pattern string) error {
	// 检查是否包含路径遍历组件
	if strings.Contains(pattern, "..") {
		return fmt.Errorf("invalid pattern: contains path traversal")
	}
	return nil
}

// validateMatchedPath 验证匹配的文件路径安全性
func validateMatchedPath(path string) error {
	// 检查是否包含路径遍历组件
	if strings.Contains(path, "..") {
		return fmt.Errorf("invalid path: contains path traversal")
	}
	return nil
}

// GetDirectorySize 获取目录大小
func GetDirectorySize(path string) int64 {
	var size int64
	filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// DirStats 目录统计结果
type DirStats struct {
	TotalFiles int
	TotalSize  int64
	ByExt      map[string]int
}

// CollectDirStats 单次遍历收集目录的文件数、总大小和按扩展名统计。
// 替代多次 GetFileCountRecursive + GetDirectorySize + CountFilesByExt 调用。
func CollectDirStats(dir string) DirStats {
	stats := DirStats{ByExt: make(map[string]int)}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if path != dir {
				rel, err := filepath.Rel(dir, path)
				if err == nil && isInternalPath(filepath.ToSlash(rel)) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		stats.TotalFiles++
		stats.TotalSize += info.Size()
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if ext != "" {
			stats.ByExt[ext]++
		}
		return nil
	})
	return stats
}

// FormatSize 格式化文件大小
func FormatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2fGB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2fMB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2fKB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// DeleteFile 删除文件
func DeleteFile(path string) error {
	cfg := config.Get()
	cleanedPath := strings.TrimPrefix(path, cfg.Domain)
	if metadata, ok := GetImageMetadata(cleanedPath); ok && metadata.StorageType == "s3" {
		source, sourceOK := cfg.StorageSourceByID(metadata.StorageSource)
		if !sourceOK || source.Type != "s3" {
			return fmt.Errorf("storage source not found")
		}
		store, err := storage.NewS3Store(context.Background(), source)
		if err != nil {
			return err
		}
		if err := store.Delete(context.Background(), metadata.ObjectKey); err != nil {
			return err
		}
		DeleteImageMetadata(cleanedPath)
		return nil
	}

	// 验证并获取安全路径
	safePath, err := getSafePath(cleanedPath)
	if err != nil {
		return err
	}

	if err := os.Remove(safePath); err != nil {
		return err
	}
	DeleteImageMetadata(cleanedPath)
	return nil
}

// MoveToRecycle 移动到回收站
func MoveToRecycle(path string, cfg *config.Config) error {
	// 提取相对路径
	relPath := strings.TrimPrefix(path, cfg.Domain)
	relPath = strings.TrimPrefix(relPath, cfg.Path)

	// 验证相对路径安全性
	if strings.Contains(relPath, "..") {
		return fmt.Errorf("invalid path: contains path traversal")
	}

	// 构建源路径 - 使用getSafePath验证
	safeSrcPath, err := getSafePath(cfg.Path + relPath)
	if err != nil {
		return err
	}

	// 构建目标路径 - 使用安全文件名
	fileName := sanitizeFilename(strings.ReplaceAll(relPath, "/", "_"))
	dstPath := filepath.Join(".", cfg.Path, "recycle", fileName)

	// 创建回收站目录
	if err := os.MkdirAll(filepath.Join(".", cfg.Path, "recycle"), 0755); err != nil {
		return fmt.Errorf("failed to create recycle directory: %w", err)
	}

	if err := os.Rename(safeSrcPath, dstPath); err != nil {
		return err
	}
	DeleteImageMetadata(cfg.Path + relPath)
	return nil
}

// RestoreFromRecycle 从回收站恢复
func RestoreFromRecycle(name string, cfg *config.Config) error {
	// 验证文件名安全性
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid filename: contains path traversal")
	}

	// 还原路径
	relPath := strings.ReplaceAll(name, "_", "/")

	// 构建源路径 - 回收站中的文件
	srcPath := filepath.Join(".", cfg.Path, "recycle", filepath.Clean(name))

	// 构建目标路径 - 使用getSafePath验证
	safeDstPath, err := getSafePath(cfg.Path + relPath)
	if err != nil {
		return err
	}

	// 创建目标目录
	if err := os.MkdirAll(filepath.Dir(safeDstPath), 0755); err != nil {
		return fmt.Errorf("failed to create restore directory: %w", err)
	}

	return os.Rename(srcPath, safeDstPath)
}

// DeleteDirectory 删除目录
func DeleteDirectory(path string) error {
	// 验证并获取安全路径
	safePath, err := getSafePath(path)
	if err != nil {
		return err
	}

	return os.RemoveAll(safePath)
}

// GetImageInfo 获取图片信息
func GetImageInfo(img string, cfg *config.Config) (map[string]interface{}, error) {
	if metadata, ok := GetImageMetadata(img); ok && metadata.StorageType == "s3" {
		url := metadata.URL
		if url == "" {
			if source, sourceOK := cfg.StorageSourceByID(metadata.StorageSource); sourceOK {
				url = storage.PublicURL(source, metadata.ObjectKey)
			}
		}
		displayName := metadata.OriginalName
		if displayName == "" {
			displayName = metadata.StoredName
		}
		return map[string]interface{}{
			"name":         metadata.StoredName,
			"storedName":   metadata.StoredName,
			"originalName": metadata.OriginalName,
			"displayName":  displayName,
			"path":         img,
			"size":         FormatSize(metadata.Size),
			"modTime":      metadata.UploadedAt,
			"url":          url,
		}, nil
	}

	// 验证并获取安全路径
	safePath, err := getSafePathWithConfig(img, cfg)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(safePath)
	if err != nil {
		return nil, err
	}
	metadata, _ := GetImageMetadata(img)
	displayName := info.Name()
	if metadata.OriginalName != "" {
		displayName = metadata.OriginalName
	}

	return map[string]interface{}{
		"name":         info.Name(),
		"storedName":   info.Name(),
		"originalName": metadata.OriginalName,
		"displayName":  displayName,
		"path":         img,
		"size":         FormatSize(info.Size()),
		"modTime":      info.ModTime().Format("2006-01-02 15:04:05"),
		"url":          cfg.Domain + img,
	}, nil
}

// GenerateThumbnail 生成缩略图
func GenerateThumbnail(img string, cfg *config.Config) (string, error) {
	// 验证并获取安全路径（getSafePath 内部使用 Rel+Join 打断污点链）
	cleanPath, err := getSafePath(img)
	if err != nil {
		return "", err
	}

	// 缓存目录
	baseDir, _ := filepath.Abs(filepath.Join(".", cfg.Path))
	cacheDir := filepath.Join(baseDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	// 缩略图文件名 - 使用安全的文件名
	relFromBase, _ := filepath.Rel(baseDir, cleanPath)
	thumbName := sanitizeFilename(strings.ReplaceAll(relFromBase, string(filepath.Separator), "_"))
	thumbPath := filepath.Join(cacheDir, thumbName)

	// 检查缓存是否存在
	if _, err := os.Stat(thumbPath); err == nil {
		return thumbPath, nil
	}

	lockValue, _ := thumbnailLocks.LoadOrStore(thumbPath, &sync.Mutex{})
	thumbLock := lockValue.(*sync.Mutex)
	thumbLock.Lock()
	defer func() {
		thumbLock.Unlock()
		thumbnailLocks.Delete(thumbPath)
	}()

	// Double-check after acquiring the per-thumbnail lock. This prevents cache
	// stampedes when many clients request the same uncached thumbnail.
	if _, err := os.Stat(thumbPath); err == nil {
		return thumbPath, nil
	}

	// 打开原始图片（cleanPath 由 getSafePath 通过 Rel+Join 重建，非污点值）
	srcImg, err := imaging.Open(cleanPath)
	if err != nil {
		// 如果无法打开（如非图片文件），复制原文件
		src, err := os.Open(cleanPath)
		if err != nil {
			return "", err
		}
		defer src.Close()

		dst, err := os.Create(thumbPath)
		if err != nil {
			return "", err
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		return thumbPath, err
	}

	// 生成缩略图 - 居中裁剪
	thumbImg := imaging.Fill(srcImg, cfg.ThumbnailW, cfg.ThumbnailH, imaging.Center, imaging.Lanczos)

	// 保存缩略图
	err = imaging.Save(thumbImg, thumbPath)
	return thumbPath, err
}

// ValidateURLPath 验证 URL 路径参数的安全性。
// 必须在使用用户提供的路径参数前调用（Download、Filer、ImageURLList 等 handler）。
// path.Clean 规范化 URL 路径，原始路径段检查防止 "..\" 绕过。
func ValidateURLPath(pathStr, requiredPrefix string) (string, error) {
	for _, part := range strings.Split(strings.ReplaceAll(pathStr, "\\", "/"), "/") {
		if part == ".." {
			return "", fmt.Errorf("invalid path: contains path traversal")
		}
	}
	// Use path.Clean (not filepath.Clean) for URL paths — filepath.Clean
	// converts "/" to "\" on Windows, breaking the prefix check below.
	cleaned := path.Clean(pathStr)
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("invalid path: contains path traversal")
	}
	// Ensure trailing slash consistency for prefix matching
	if !strings.HasSuffix(cleaned, "/") {
		cleaned += "/"
	}
	prefix := requiredPrefix
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if !strings.HasPrefix(cleaned, prefix) {
		return "", fmt.Errorf("invalid path: outside allowed directory")
	}
	return cleaned, nil
}

// getSafePath 验证路径并返回安全的文件系统路径
// 使用 filepath.Rel + filepath.Join 模式打断 CodeQL 污点链：
// 先验证用户路径在允许目录内，再用 Rel 提取相对路径，最后从可信基目录重建。
func getSafePath(userPath string) (string, error) {
	return getSafePathWithConfig(userPath, config.Get())
}

func getSafePathWithConfig(userPath string, cfg *config.Config) (string, error) {
	if cfg == nil || cfg.Path == "" {
		return "", fmt.Errorf("invalid config path")
	}

	// 获取允许目录的绝对路径（可信基目录）
	allowedDir, err := filepath.Abs(filepath.Join(".", cfg.Path))
	if err != nil {
		return "", fmt.Errorf("invalid config path")
	}

	// 构建用户路径的绝对路径
	absPath, err := filepath.Abs(filepath.Join(".", userPath))
	if err != nil {
		return "", fmt.Errorf("invalid path")
	}

	// 使用 filepath.Rel 计算相对路径（产生新的非污点值）
	relPath, err := filepath.Rel(allowedDir, absPath)
	if err != nil {
		return "", fmt.Errorf("invalid path")
	}

	// 确保相对路径没有逃逸到上级目录
	if strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
		return "", fmt.Errorf("invalid path: outside allowed directory")
	}

	// 从可信基目录 + 经验证的相对路径重建完整路径
	// 此路径不再携带用户输入的污点标记
	return filepath.Join(allowedDir, relPath), nil
}

// SanitizePath validates that a constructed filesystem path is within the image storage
// directory and returns a cleaned absolute path. Use on paths built from user-influenced
// components (e.g. uploadId) to break CodeQL taint chains.
func SanitizePath(pathStr string) (string, error) {
	cfg := config.Get()
	allowedDir, err := filepath.Abs(filepath.Join(".", cfg.Path))
	if err != nil {
		return "", fmt.Errorf("invalid config path")
	}
	absPath, err := filepath.Abs(filepath.Clean(pathStr))
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	relPath, err := filepath.Rel(allowedDir, absPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
		return "", fmt.Errorf("invalid path: outside allowed directory")
	}
	return filepath.Join(allowedDir, relPath), nil
}

// sanitizeFilename 清理文件名，移除危险字符
func sanitizeFilename(name string) string {
	// 只保留字母、数字、下划线、连字符和点
	var result strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.' {
			result.WriteRune(c)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}

// uploadLogEntry 上传日志条目（内存缓冲用）
type uploadLogEntry struct {
	FileName string                 `json:"-"`
	Data     map[string]interface{} `json:"-"`
}

// uploadLogBuf 内存中的上传日志缓冲，定期刷盘。
// 避免每次上传都读写整个月度 JSON 文件。
var (
	uploadLogBuf   []uploadLogEntry
	uploadLogMonth string // 缓冲对应的月份 "2006-01"
	uploadLogMu    sync.Mutex
)

func init() {
	// 启动后台定时刷盘（每 60 秒）
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			flushUploadLog()
		}
	}()
}

// flushUploadLog 将内存中的上传日志写入磁盘
func flushUploadLog() {
	uploadLogMu.Lock()
	defer uploadLogMu.Unlock()
	flushUploadLogLocked()
}

func flushUploadLogLocked() {
	if len(uploadLogBuf) == 0 || uploadLogMonth == "" {
		return
	}
	cfg := config.Get()
	if cfg.UploadLogs == 0 {
		uploadLogBuf = nil
		return
	}

	logDir := "admin/logs/upload"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("[UploadLog] 创建日志目录失败: %v", err)
		return
	}
	logFile := filepath.Join(logDir, uploadLogMonth+".json")

	// 读取现有日志
	logs := make(map[string]interface{})
	if data, err := os.ReadFile(logFile); err == nil {
		if err := json.Unmarshal(data, &logs); err != nil {
			log.Printf("[UploadLog] 解析日志文件失败 %s: %v，将覆盖", logFile, err)
		}
	}

	// 合并缓冲中的条目
	for _, entry := range uploadLogBuf {
		logs[entry.FileName] = entry.Data
	}

	// 写入
	data, err := json.MarshalIndent(logs, "", "  ")
	if err != nil {
		log.Printf("[UploadLog] 序列化日志失败: %v", err)
		return
	}
	if err := os.WriteFile(logFile, data, 0644); err != nil {
		log.Printf("[UploadLog] 写入日志文件失败 %s: %v", logFile, err)
		return
	}
	uploadLogBuf = nil
}

// WriteUploadLog 写入上传日志（缓冲到内存，由后台定时刷盘）
func WriteUploadLog(filePath, sourceName, absolutePath string, fileSize int64, from string) {
	cfg := config.Get()
	if cfg.UploadLogs == 0 {
		return
	}

	now := time.Now()
	month := now.Format("2006-01")

	uploadLogMu.Lock()
	defer uploadLogMu.Unlock()

	// 月份切换时先刷盘旧数据
	if uploadLogMonth != month && uploadLogMonth != "" {
		flushUploadLogLocked()
	}
	uploadLogMonth = month

	uploadLogBuf = append(uploadLogBuf, uploadLogEntry{
		FileName: filepath.Base(filePath),
		Data: map[string]interface{}{
			"source": sourceName,
			"date":   now.Format("2006-01-02 15:04:05"),
			"path":   filePath,
			"size":   FormatSize(fileSize),
			"from":   from,
		},
	})
}

// FlushUploadLogNow 立即将上传日志刷盘（供关机前或需要时调用）
func FlushUploadLogNow() {
	flushUploadLog()
}

// IP 上传计数互斥锁，防止并发读写竞态条件
var ipCountMu sync.Mutex

// ipCountCache 内存缓存，避免每次上传都读写磁盘。
// key = "YYYY-MM-DD"，value = map[string]int (ip -> count)
var (
	ipCountCache    map[string]map[string]int
	ipCountCacheDay string // 当前缓存的日期，用于检测日期切换
)

// ipCountDirty 标记缓存是否有未写入磁盘的变更
var ipCountDirty bool

func init() {
	ipCountCache = make(map[string]map[string]int)
	// 启动后台定时刷盘（每 30 秒）
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			flushIPCounts()
		}
	}()
}

// loadIPCounts 从磁盘加载当日的 IP 计数到缓存（仅在首次访问或日期切换时调用，需持锁）
func loadIPCounts(day string) {
	if ipCountCacheDay == day {
		return
	}
	// 日期切换前先刷盘旧数据
	if ipCountCacheDay != "" {
		flushIPCountsLocked()
	}
	ipCountCacheDay = day
	logDir := "admin/logs/ipcounts"
	logFile := filepath.Join(logDir, day+".json")
	counts := make(map[string]int)
	if data, err := os.ReadFile(logFile); err == nil {
		if err := json.Unmarshal(data, &counts); err != nil {
			log.Printf("[IPCount] 解析IP计数文件失败 %s: %v", logFile, err)
		}
	}
	ipCountCache[day] = counts
}

// flushIPCounts 将缓存写入磁盘（无锁版本，由 flushIPCounts 和后台 goroutine 调用）
func flushIPCountsLocked() {
	if !ipCountDirty || ipCountCacheDay == "" {
		return
	}
	logDir := "admin/logs/ipcounts"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("[IPCount] 创建日志目录失败: %v", err)
		return
	}
	logFile := filepath.Join(logDir, ipCountCacheDay+".json")
	counts := ipCountCache[ipCountCacheDay]
	if counts == nil {
		counts = make(map[string]int)
	}
	data, err := json.MarshalIndent(counts, "", "  ")
	if err != nil {
		log.Printf("[IPCount] 序列化IP计数失败: %v", err)
		return
	}
	if err := os.WriteFile(logFile, data, 0644); err != nil {
		log.Printf("[IPCount] 写入IP计数文件失败 %s: %v", logFile, err)
		return
	}
	ipCountDirty = false
}

// flushIPCounts 刷盘（带锁）
func flushIPCounts() {
	ipCountMu.Lock()
	defer ipCountMu.Unlock()
	flushIPCountsLocked()
}

// CheckIPUploadCount 检查IP上传次数（内存缓存，避免每次读写磁盘）
func CheckIPUploadCount(ip string, limit int) bool {
	if limit <= 0 {
		return true
	}

	ipCountMu.Lock()
	defer ipCountMu.Unlock()

	today := time.Now().Format("2006-01-02")
	loadIPCounts(today)

	counts := ipCountCache[today]
	if counts == nil {
		return true
	}
	return counts[ip] < limit
}

// IncrementIPUploadCount 增加IP上传次数（写入内存缓存，由后台定时刷盘）
func IncrementIPUploadCount(ip string) {
	ipCountMu.Lock()
	defer ipCountMu.Unlock()

	today := time.Now().Format("2006-01-02")
	loadIPCounts(today)

	counts := ipCountCache[today]
	if counts == nil {
		counts = make(map[string]int)
		ipCountCache[today] = counts
	}
	counts[ip]++
	ipCountDirty = true
}

// FlushIPCountsNow 立即将 IP 计数刷盘（供关机前或需要时调用）
func FlushIPCountsNow() {
	flushIPCounts()
}
