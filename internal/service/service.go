package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"easyimage/config"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/hkdf"
)

// IsLoggedIn 检查是否已登录
func IsLoggedIn(c *gin.Context) bool {
	auth, err := c.Cookie("auth")
	if err != nil {
		return false
	}

	var creds []string
	if err := json.Unmarshal([]byte(auth), &creds); err != nil {
		return false
	}

	if len(creds) < 2 {
		return false
	}

	cfg := config.Get()
	guests, _ := config.LoadGuestConfig()

	// 检查管理员
	if creds[0] == cfg.User && creds[1] == cfg.Password {
		return true
	}

	// 检查上传者
	if guest, exists := guests[creds[0]]; exists {
		if guest.Password == creds[1] && guest.Expired > time.Now().Unix() {
			return true
		}
	}

	return false
}

// IsAdmin 检查是否管理员
func IsAdmin(c *gin.Context) bool {
	auth, err := c.Cookie("auth")
	if err != nil {
		return false
	}

	var creds []string
	if err := json.Unmarshal([]byte(auth), &creds); err != nil {
		return false
	}

	if len(creds) < 2 {
		return false
	}

	cfg := config.Get()
	return creds[0] == cfg.User && creds[1] == cfg.Password
}

// SetAdminSession 设置管理员会话
func SetAdminSession(c *gin.Context, user string) {
	cfg := config.Get()
	creds, _ := json.Marshal([]string{user, cfg.Password})
	// 设置HttpOnly标志，防止客户端脚本访问cookie
	// secure参数根据域名是否为https来判断
	secure := strings.HasPrefix(cfg.Domain, "https")
	c.SetCookie("auth", string(creds), 3600*24*14, "/", "", secure, true)
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
	// 检查用户名
	if user != cfg.User {
		return false, "用户名不存在"
	}

	// 检查密码
	if !CheckPassword(password, cfg.Password) {
		return false, "密码错误"
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

	// 缓存目录
	cacheDir := filepath.Join(".", cfg.Path, "cache")
	os.MkdirAll(cacheDir, 0755)

	// 缩略图文件名 - 使用安全的文件名
	relPath := strings.TrimPrefix(img, cfg.Path)
	thumbName := sanitizeFilename(strings.ReplaceAll(relPath, "/", "_"))
	thumbPath := filepath.Join(cacheDir, thumbName)

	// 检查缓存是否存在
	if _, err := os.Stat(thumbPath); err == nil {
		return thumbPath, nil
	}

	// 使用imaging库生成缩略图
	if _, err := os.Stat(safeSrcPath); err != nil {
		return "", err
	}

	// 打开原始图片
	srcImg, err := imaging.Open(safeSrcPath)
	if err != nil {
		// 如果无法打开（如非图片文件），复制原文件
		src, err := os.Open(safeSrcPath)
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

// validatePath 验证路径安全性，防止路径遍历攻击
func validatePath(path, allowedPrefix string) error {
	// 清理路径
	cleaned := filepath.Clean(path)

	// 检查是否包含路径遍历组件
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("invalid path: contains path traversal")
	}

	// 确保路径在允许的目录下
	if !strings.HasPrefix(cleaned, allowedPrefix) {
		return fmt.Errorf("invalid path: outside allowed directory")
	}

	return nil
}

// getSafePath 验证路径并返回安全的绝对路径
// 此函数用于替代直接使用用户输入构建路径
func getSafePath(userPath string) (string, error) {
	cfg := config.Get()

	// 清理路径
	cleaned := filepath.Clean(userPath)

	// 检查是否包含路径遍历组件
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("invalid path: contains path traversal")
	}

	// 确保路径在允许的目录下
	if !strings.HasPrefix(cleaned, cfg.Path) {
		return "", fmt.Errorf("invalid path: outside allowed directory")
	}

	// 返回安全的绝对路径
	return filepath.Join(".", cleaned), nil
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

// CheckIPUploadCount 检查IP上传次数
func CheckIPUploadCount(ip string, limit int) bool {
	if limit <= 0 {
		return true
	}

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
