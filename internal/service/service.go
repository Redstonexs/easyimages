package service

import (
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
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"easyimage/config"

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
	maxLoginAttempts    = 5             // 最大尝试次数
	loginLockoutWindow  = 5 * time.Minute // 锁定窗口
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

	// 清理过期 session
	cleanExpiredSessions()

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
		return source
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
		randNum, _ := rand.Int(rand.Reader, big.NewInt(9000))
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
	// 验证模式字符串安全性
	if err := validateGlobPattern(pattern); err != nil {
		return nil
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	var files []string
	for _, match := range matches {
		// 验证匹配的文件路径
		if err := validateMatchedPath(match); err != nil {
			continue
		}
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			files = append(files, filepath.Base(match))
		}
	}

	if sortOrder == 1 {
		// 反转排序
		for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
			files[i], files[j] = files[j], files[i]
		}
	}

	return files
}

// GetDirList 获取目录列表
func GetDirList(dirPath string) []string {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
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

	count := 0
	for _, match := range matches {
		// 验证匹配的文件路径
		if err := validateMatchedPath(match); err != nil {
			continue
		}
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			count++
		}
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

	// 验证并获取安全路径
	safePath, err := getSafePath(cleanedPath)
	if err != nil {
		return err
	}

	return os.Remove(safePath)
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
	os.MkdirAll(filepath.Join(".", cfg.Path, "recycle"), 0755)

	return os.Rename(safeSrcPath, dstPath)
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
	os.MkdirAll(filepath.Dir(safeDstPath), 0755)

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
	// 验证并获取安全路径
	safePath, err := getSafePath(img)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(safePath)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"name":    info.Name(),
		"path":    img,
		"size":    FormatSize(info.Size()),
		"modTime": info.ModTime().Format("2006-01-02 15:04:05"),
		"url":     cfg.Domain + img,
	}, nil
}

// GenerateThumbnail 生成缩略图
func GenerateThumbnail(img string, cfg *config.Config) (string, error) {
	// 验证并获取安全路径
	safeSrcPath, err := getSafePath(img)
	if err != nil {
		return "", err
	}

	// 获取允许的基目录绝对路径
	baseDir, err := filepath.Abs(filepath.Join(".", cfg.Path))
	if err != nil {
		return "", fmt.Errorf("invalid config path")
	}

	// 使用 filepath.Rel 打断 CodeQL 污点链：
	// Rel 计算从 baseDir 到 safeSrcPath 的相对路径，生成全新的非污点字符串
	relPath, err := filepath.Rel(baseDir, safeSrcPath)
	if err != nil {
		return "", fmt.Errorf("invalid path")
	}

	// 确保相对路径没有逃逸到上级目录
	if strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
		return "", fmt.Errorf("path escapes allowed directory")
	}

	// 从可信基目录 + 经验证的相对路径重建完整路径（非污点）
	cleanPath := filepath.Join(baseDir, relPath)

	// 缓存目录
	cacheDir := filepath.Join(baseDir, "cache")
	os.MkdirAll(cacheDir, 0755)

	// 缩略图文件名 - 使用安全的文件名
	thumbName := sanitizeFilename(strings.ReplaceAll(relPath, string(filepath.Separator), "_"))
	thumbPath := filepath.Join(cacheDir, thumbName)

	// 检查缓存是否存在
	if _, err := os.Stat(thumbPath); err == nil {
		return thumbPath, nil
	}

	// 打开原始图片（使用从可信基目录重建的路径）
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

// getSafePath 验证路径并返回安全的文件系统路径
// 使用绝对路径比较，防止 Windows 下路径分隔符差异导致的绕过
func getSafePath(userPath string) (string, error) {
	cfg := config.Get()

	// 构建文件系统路径
	fsPath := filepath.Join(".", userPath)

	// 获取绝对路径
	absPath, err := filepath.Abs(fsPath)
	if err != nil {
		return "", fmt.Errorf("invalid path")
	}

	// 获取允许目录的绝对路径
	allowedDir, err := filepath.Abs(filepath.Join(".", cfg.Path))
	if err != nil {
		return "", fmt.Errorf("invalid config path")
	}

	// 确保绝对路径在允许的目录下（加分隔符防止前缀匹配欺骗）
	// 例如防止 /i-evil/ 匹配 /i/ 的前缀检查
	if !strings.HasPrefix(absPath, allowedDir+string(filepath.Separator)) && absPath != allowedDir {
		return "", fmt.Errorf("invalid path: outside allowed directory")
	}

	return absPath, nil
}

// sanitizeFilename 清理文件名，移除危险字符
func sanitizeFilename(name string) string	{
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

// WriteUploadLog 写入上传日志
func WriteUploadLog(filePath, sourceName, absolutePath string, fileSize int64, from string) {
	cfg := config.Get()
	if cfg.UploadLogs == 0 {
		return
	}

	logDir := "admin/logs/upload"
	os.MkdirAll(logDir, 0755)

	logFile := filepath.Join(logDir, time.Now().Format("2006-01")+".json")

	// 读取现有日志
	logs := make(map[string]interface{})
	if data, err := os.ReadFile(logFile); err == nil {
		json.Unmarshal(data, &logs)
	}

	// 添加新日志
	logs[filepath.Base(filePath)] = map[string]interface{}{
		"source":   sourceName,
		"date":     time.Now().Format("2006-01-02 15:04:05"),
		"path":     filePath,
		"size":     FormatSize(fileSize),
		"from":     from,
	}

	// 写入日志
	data, _ := json.MarshalIndent(logs, "", "  ")
	os.WriteFile(logFile, data, 0644)
}

// IP 上传计数互斥锁，防止并发读写竞态条件
var ipCountMu sync.Mutex

// CheckIPUploadCount 检查IP上传次数
func CheckIPUploadCount(ip string, limit int) bool {
	if limit <= 0 {
		return true
	}

	ipCountMu.Lock()
	defer ipCountMu.Unlock()

	logDir := "admin/logs/ipcounts"
	logFile := filepath.Join(logDir, time.Now().Format("2006-01-02")+".json")

	counts := make(map[string]int)
	if data, err := os.ReadFile(logFile); err == nil {
		json.Unmarshal(data, &counts)
	}

	return counts[ip] < limit
}

// IncrementIPUploadCount 增加IP上传次数
func IncrementIPUploadCount(ip string) {
	ipCountMu.Lock()
	defer ipCountMu.Unlock()

	logDir := "admin/logs/ipcounts"
	os.MkdirAll(logDir, 0755)

	logFile := filepath.Join(logDir, time.Now().Format("2006-01-02")+".json")

	counts := make(map[string]int)
	if data, err := os.ReadFile(logFile); err == nil {
		json.Unmarshal(data, &counts)
	}

	counts[ip]++

	data, _ := json.MarshalIndent(counts, "", "  ")
	os.WriteFile(logFile, data, 0644)
}
