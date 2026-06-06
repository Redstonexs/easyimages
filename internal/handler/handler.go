package handler

import (
	"easyimage/config"
	"easyimage/internal/service"
	"easyimage/internal/storage"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func Index(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg.MustLogin == 1 && !service.IsLoggedIn(c) {
			c.Redirect(http.StatusFound, loginRedirectURL(c))
			return
		}

		// 检查登录状态
		isAdmin := service.IsAdmin(c)

		captchaData := service.GenerateCaptcha(cfg)
		frontend := gin.H{
			"config": gin.H{
				"title":                  cfg.Title,
				"description":            cfg.Description,
				"max_size":               cfg.MaxSize,
				"chunk_size":             publicChunkSize(cfg),
				"api_status":             cfg.APIStatus,
				"default_storage_source": defaultPublicStorageSource(cfg),
				"storage_sources":        publicStorageSources(cfg),
			},
			"version":    config.Version,
			"is_admin":   isAdmin,
			"must_login": cfg.MustLogin,
			"captcha":    captchaData,
		}
		data := gin.H{
			"config":    cfg,
			"version":   config.Version,
			"isAdmin":   isAdmin,
			"captcha":   captchaData,
			"mustLogin": cfg.MustLogin,
			"frontend":  frontend,
		}
		c.HTML(http.StatusOK, "index.html", data)
	}
}

func loginRedirectURL(c *gin.Context) string {
	return "/admin/index?redirect=" + url.QueryEscape(c.Request.URL.RequestURI())
}

func defaultPublicStorageSource(cfg *config.Config) string {
	source, _ := cfg.StorageSourceByID(cfg.DefaultStorageSource)
	if source.ID == "" {
		return "local"
	}
	return source.ID
}

func publicStorageSources(cfg *config.Config) []gin.H {
	sources := cfg.EnabledStorageSources()
	items := make([]gin.H, 0, len(sources))
	for _, source := range sources {
		items = append(items, gin.H{"id": source.ID, "name": source.Name, "type": source.Type})
	}
	return items
}

func publicChunkSize(cfg *config.Config) int64 {
	const defaultChunkSize = 16 * 1024 * 1024
	if cfg != nil && cfg.Chunks > 0 {
		return int64(cfg.Chunks) * 1024 * 1024
	}
	return defaultChunkSize
}

// CaptchaAPI 获取验证码
func CaptchaAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		data := service.GenerateCaptcha(cfg)
		c.JSON(http.StatusOK, data)
	}
}

// AdminLoginAPI 管理员登录API
func AdminLoginAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查登录速率限制
		clientIP := c.ClientIP()
		if !service.CheckLoginRateLimit(clientIP) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"result":  "failed",
				"message": "登录尝试过于频繁，请5分钟后再试",
			})
			return
		}

		// 验证验证码
		if cfg.Captcha == 1 {
			log.Printf("[API Login] captcha=%d, captcha_type=%d, answer=%q, token=%q",
				cfg.Captcha, cfg.CaptchaType,
				c.PostForm("captcha_answer"), c.PostForm("captcha_token"))
			ok, msg := service.VerifyCaptcha(cfg,
				c.PostForm("captcha_answer"),
				c.PostForm("captcha_token"),
				c.PostForm("cf_turnstile_response"),
				c.PostForm("g_recaptcha_response"),
			)
			if !ok {
				c.JSON(http.StatusBadRequest, gin.H{
					"result":  "failed",
					"message": msg,
				})
				return
			}
		}

		user := c.PostForm("user")
		password := c.PostForm("password")

		// 验证用户名密码
		success, message := service.ValidateLogin(user, password, cfg)
		if success {
			service.ResetLoginAttempts(clientIP)
			service.SetAdminSession(c, user)
			c.JSON(http.StatusOK, gin.H{
				"result":  "success",
				"message": "登录成功",
			})
			return
		}

		service.RecordFailedLogin(clientIP)
		c.JSON(http.StatusUnauthorized, gin.H{
			"result":  "failed",
			"message": message,
		})
	}
}

func List(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		initial, err := imageListPayload(c, cfg)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.HTML(http.StatusOK, "list.html", gin.H{
			"config":  cfg,
			"initial": initial,
		})
	}
}

// ImageListAPI returns public gallery data for frontend rendering and caching.
func ImageListAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, err := imageListPayload(c, cfg)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		setFrontendAPIHeaders(c)
		c.JSON(http.StatusOK, payload)
	}
}

func Upload(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查登录
		if cfg.MustLogin == 1 {
			if !service.IsLoggedIn(c) {
				c.JSON(http.StatusUnauthorized, gin.H{
					"result":  "failed",
					"code":    401,
					"message": "本站已开启登陆上传,您尚未登陆",
				})
				return
			}
		}

		// 获取上传文件
		form, err := c.MultipartForm()
		if err != nil {
			errMsg := "没有选择上传的文件"
			if strings.Contains(err.Error(), "too large") || strings.Contains(err.Error(), "too many bytes") {
				errMsg = fmt.Sprintf("上传文件过大，单文件限制 %s", service.FormatSize(cfg.MaxSize))
			}
			c.JSON(http.StatusBadRequest, gin.H{
				"result":  "failed",
				"code":    204,
				"message": errMsg,
			})
			return
		}

		files := form.File["file"]
		if len(files) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"result":  "failed",
				"code":    204,
				"message": "没有选择上传的文件",
			})
			return
		}

		// 限制上传数量
		if len(files) > cfg.MaxUploadFiles {
			files = files[:cfg.MaxUploadFiles]
		}

		var results []gin.H
		for _, file := range files {
			result := service.ProcessUpload(c, file, cfg, "web")
			results = append(results, result)
		}

		if len(results) == 1 {
			c.JSON(http.StatusOK, results[0])
		} else {
			c.JSON(http.StatusOK, gin.H{
				"result": "success",
				"code":   200,
				"files":  results,
			})
		}
	}
}

// ChunkUpload 分片上传处理
// 大文件被前端切分为小块逐个上传，避免单个请求过大触发网关超时(如Cloudflare 524)
func ChunkUpload(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查登录
		if cfg.MustLogin == 1 {
			if !service.IsLoggedIn(c) {
				c.JSON(http.StatusUnauthorized, gin.H{
					"result": "failed", "code": 401, "message": "本站已开启登陆上传,您尚未登陆",
				})
				return
			}
		}

		uploadId := c.PostForm("uploadId")
		totalChunksStr := c.PostForm("totalChunks")
		filename := c.PostForm("filename")
		isMerge := c.PostForm("merge") == "true"

		if uploadId == "" || totalChunksStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "缺少分片参数"})
			return
		}

		totalChunks, err := strconv.Atoi(totalChunksStr)
		if err != nil || totalChunks <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "无效的分片总数"})
			return
		}

		// chunkIndex 仅在非 merge 请求时需要
		var chunkIndex int
		if !isMerge {
			chunkIndexStr := c.PostForm("chunkIndex")
			if chunkIndexStr == "" {
				c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "缺少分片索引"})
				return
			}
			chunkIndex, err = strconv.Atoi(chunkIndexStr)
			if err != nil || chunkIndex < 0 || chunkIndex >= totalChunks {
				c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "无效的分片索引"})
				return
			}
		}

		// 安全校验 uploadId
		for _, ch := range uploadId {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-') {
				c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "无效的上传ID"})
				return
			}
		}

		source, _ := cfg.StorageSourceByID(c.PostForm("storage_source"))
		if source.Type == "s3" {
			handleS3ChunkUpload(c, cfg, source, uploadId, totalChunks, chunkIndex, isMerge, filename)
			return
		}

		chunkDir, pathErr := service.SanitizePathForConfig(storagePathCandidate(cfg.Path, "chunks", uploadId), cfg)
		if pathErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "无效的上传路径"})
			return
		}

		// 并发上传时，最后一个到达的分片不一定是 totalChunks-1。
		// 不再自动合并，客户端需发送 merge=true 参数显式触发合并。
		if !isMerge {
			chunk, formFileErr := c.FormFile("chunk")
			if chunk == nil {
				errMsg := "缺少分片数据"
				if formFileErr != nil {
					errMsg = fmt.Sprintf("读取分片数据失败: %v", formFileErr)
					log.Printf("[chunk upload] FormFile error for uploadId=%s chunkIndex=%d: %v", uploadId, chunkIndex, formFileErr)
				}
				c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": errMsg})
				return
			}
			if chunk.Size == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": fmt.Sprintf("分片 %d 为空文件", chunkIndex)})
				return
			}
			if err := os.MkdirAll(chunkDir, 0755); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "创建分片目录失败"})
				return
			}
			chunkPath := filepath.Join(chunkDir, fmt.Sprintf("%06d", chunkIndex))
			saveStarted := time.Now()
			if err := c.SaveUploadedFile(chunk, chunkPath); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "保存分片失败"})
				return
			}
			if elapsed := time.Since(saveStarted); elapsed > 2*time.Second {
				log.Printf("[chunk upload] slow chunk save uploadId=%s chunkIndex=%d size=%d elapsed=%s", uploadId, chunkIndex, chunk.Size, elapsed)
			}
			c.JSON(http.StatusOK, gin.H{"result": "success", "code": 200, "message": "分片上传成功", "chunkIndex": chunkIndex})
			return
		}

		// === 合并所有分片 ===
		mergeStarted := time.Now()
		partPaths := make([]string, totalChunks)
		var totalSize int64
		for i := 0; i < totalChunks; i++ {
			partPath := filepath.Join(chunkDir, fmt.Sprintf("%06d", i))
			info, err := os.Stat(partPath)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"result": "failed", "code": 400,
					"message": fmt.Sprintf("分片 %d 尚未上传，无法合并", i),
				})
				return
			}
			if info.Size() == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": fmt.Sprintf("分片 %d 为空文件", i)})
				return
			}
			totalSize += info.Size()
			partPaths[i] = partPath
		}
		if totalSize > cfg.MaxSize {
			os.RemoveAll(chunkDir)
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": fmt.Sprintf("文件大小超过限制: %s", service.FormatSize(cfg.MaxSize))})
			return
		}

		target, targetErr := service.BuildUploadTarget(filename, cfg, config.StorageSourceConfig{ID: "local", Type: "local"})
		if targetErr != nil {
			os.RemoveAll(chunkDir)
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": targetErr.Error()})
			return
		}
		uploadDir, dirErr := service.SanitizePathForConfig(storagePathCandidate(cfg.Path, target.StoragePath), cfg)
		if dirErr != nil {
			os.RemoveAll(chunkDir)
			c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "存储路径无效"})
			return
		}
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			os.RemoveAll(chunkDir)
			c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "创建存储目录失败"})
			return
		}
		finalPath, finalPathErr := service.SanitizePathForConfig(filepath.Join(uploadDir, target.FileName), cfg)
		if finalPathErr != nil {
			os.RemoveAll(chunkDir)
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "无效的文件路径"})
			return
		}

		// 合并分片到目标文件
		outFile, err := os.Create(finalPath)
		if err != nil {
			os.RemoveAll(chunkDir)
			c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "创建目标文件失败"})
			return
		}
		buf := make([]byte, 1024*1024)
		for i := 0; i < totalChunks; i++ {
			partFile, err := os.Open(partPaths[i])
			if err != nil {
				outFile.Close()
				os.Remove(finalPath)
				os.RemoveAll(chunkDir)
				c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": fmt.Sprintf("缺少分片 %d", i)})
				return
			}
			_, err = io.CopyBuffer(outFile, partFile, buf)
			partFile.Close()
			if err != nil {
				outFile.Close()
				os.Remove(finalPath)
				os.RemoveAll(chunkDir)
				c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": fmt.Sprintf("合并分片 %d 失败: %v", i, err)})
				return
			}
		}
		if err := outFile.Close(); err != nil {
			os.Remove(finalPath)
			os.RemoveAll(chunkDir)
			c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "写入目标文件失败"})
			return
		}
		os.RemoveAll(chunkDir) // 清理分片

		// SVG 安全检查
		if target.Ext == "svg" && !service.CheckSVGSecurity(finalPath) {
			os.Remove(finalPath)
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "SVG文件包含不安全内容"})
			return
		}

		// 生成URL
		relativePath := target.RelativePath
		imageURL := cfg.Domain + relativePath
		thumbURL := cfg.Domain + "/app/thumb?img=" + relativePath
		delURL := ""
		if cfg.ShowUserHashDel == 1 {
			delURL = cfg.Domain + "/app/del_hash?hash=" + service.EncryptHash(relativePath, cfg.Password)
		}
		if cfg.HidePath == 1 {
			imageURL = strings.Replace(imageURL, cfg.Path, "/", 1)
		}
		if cfg.Hide == 1 {
			imageURL = cfg.Domain + "/app/hide?key=" + service.EncryptHideKey(relativePath, cfg.HideKey)
		}

		service.StartImagePostProcess(finalPath, cfg)
		service.SaveImageMetadataWithStorage(relativePath, target.OriginalName, totalSize, "web", target.UploadedAt, "local", "local", target.ObjectKey, imageURL, thumbURL)

		// 生成WebP URL（与 ProcessUpload 保持一致）
		webpURL := ""
		if cfg.WebpConvert == 1 {
			webpRelativePath := cfg.Path + "webp/" + target.StoragePath + target.BaseName + ".webp"
			webpURL = cfg.Domain + webpRelativePath
			if cfg.HidePath == 1 {
				webpURL = strings.Replace(webpURL, cfg.Path, "/", 1)
			}
		}
		if elapsed := time.Since(mergeStarted); elapsed > 2*time.Second {
			log.Printf("[chunk upload] slow merge uploadId=%s chunks=%d size=%d elapsed=%s", uploadId, totalChunks, totalSize, elapsed)
		}

		c.JSON(http.StatusOK, gin.H{
			"result": "success", "code": 200,
			"url": imageURL, "srcName": strings.TrimSuffix(target.OriginalName, filepath.Ext(target.OriginalName)), "original_name": target.OriginalName, "thumb": thumbURL, "del": delURL, "webp_url": webpURL,
		})
	}
}

func storagePathCandidate(pathPrefix string, elems ...string) string {
	parts := make([]string, 0, len(elems)+2)
	parts = append(parts, ".")
	if trimmed := strings.Trim(pathPrefix, `/\`); trimmed != "" {
		parts = append(parts, trimmed)
	}
	parts = append(parts, elems...)
	return filepath.Join(parts...)
}

// ChunkCleanup 清理失败上传的分片目录
func ChunkCleanup(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		uploadId := c.PostForm("uploadId")
		if uploadId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "缺少uploadId"})
			return
		}

		// 安全校验 uploadId（与 ChunkUpload 一致）
		for _, ch := range uploadId {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-') {
				c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "无效的上传ID"})
				return
			}
		}
		unlockS3State := lockS3MultipartState(uploadId)
		if state, err := storage.LoadMultipartState(uploadId); err == nil && state.S3UploadID != "" {
			if source, ok := cfg.StorageSourceByID(state.SourceID); ok && source.Type == "s3" {
				if store, err := newS3MultipartStore(c.Request.Context(), source); err == nil {
					_ = store.AbortMultipart(c.Request.Context(), state.ObjectKey, state.S3UploadID)
				}
			}
			_ = storage.DeleteMultipartState(uploadId)
			unlockS3State()
			c.JSON(http.StatusOK, gin.H{"result": "success", "code": 200})
			return
		}
		unlockS3State()

		chunkDir, pathErr := service.SanitizePathForConfig(storagePathCandidate(cfg.Path, "chunks", uploadId), cfg)
		if pathErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "无效的路径"})
			return
		}
		os.RemoveAll(chunkDir)
		c.JSON(http.StatusOK, gin.H{"result": "success", "code": 200})
	}
}

func APIUpload(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// CORS头
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")

		// 检查API状态
		if cfg.APIStatus == 0 {
			c.JSON(http.StatusOK, gin.H{
				"result":  "failed",
				"code":    201,
				"message": "API Closed",
			})
			return
		}

		// 验证Token
		token := c.PostForm("token")
		if token == "" {
			c.JSON(http.StatusOK, gin.H{
				"result":  "failed",
				"code":    202,
				"message": "Token required",
			})
			return
		}

		apiKeys, err := config.LoadAPIKeys()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"result":  "failed",
				"code":    500,
				"message": "Server error",
			})
			return
		}

		key, exists := apiKeys[token]
		if !exists {
			c.JSON(http.StatusOK, gin.H{
				"result":  "failed",
				"code":    202,
				"message": "Token Error",
			})
			return
		}

		if key.Expired < time.Now().Unix() {
			c.JSON(http.StatusOK, gin.H{
				"result":  "failed",
				"code":    203,
				"message": "Token Expired",
			})
			return
		}

		// 获取上传文件
		file, err := c.FormFile("image")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"result":  "failed",
				"code":    204,
				"message": "没有选择上传的文件",
			})
			return
		}

		result := service.ProcessUpload(c, file, cfg, fmt.Sprintf("api_%d", key.ID))
		result["id"] = key.ID
		c.JSON(http.StatusOK, result)
	}
}

func DeleteByHash(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 支持从查询参数和POST表单获取hash
		hash := c.PostForm("hash")
		if hash == "" {
			hash = c.Query("hash")
		}
		if hash == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code": 400,
				"msg":  "Invalid request",
			})
			return
		}

		// 解密hash
		path, err := service.DecryptHash(hash, cfg.Password)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code": 404,
				"msg":  "Invalid hash",
			})
			return
		}

		// 删除文件
		if cfg.ImageRecycle == 1 {
			err = service.MoveToRecycle(path, cfg)
		} else {
			err = service.DeleteFile(path)
		}

		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": 404,
				"msg":  "删除失败",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code": 200,
			"msg":  "删除成功",
		})
	}
}

func DeleteAction(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查管理员权限
		if !service.IsAdmin(c) {
			c.JSON(http.StatusForbidden, gin.H{
				"code": 403,
				"msg":  "Permission denied",
			})
			return
		}

		mode := c.PostForm("mode")
		url := c.PostForm("url")

		switch mode {
		case "delete":
			if err := service.DeleteFile(url); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"code": 404,
					"msg":  "删除失败",
					"type": "danger",
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"code": 200,
				"msg":  "删除成功",
				"type": "success",
			})

		case "recycle":
			if err := service.MoveToRecycle(url, cfg); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"code": 404,
					"msg":  "回收失败",
					"type": "danger",
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"code": 200,
				"msg":  "回收成功",
				"type": "success",
			})

		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"code": 400,
				"msg":  "Invalid mode",
			})
		}
	}
}

func Info(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		img := c.Query("img")
		if img == "" {
			c.HTML(http.StatusBadRequest, "error.html", gin.H{
				"config":  cfg,
				"message": "图片参数错误",
			})
			return
		}

		// 获取图片信息
		info, err := service.GetImageInfo(img, cfg)
		if err != nil {
			c.HTML(http.StatusNotFound, "error.html", gin.H{
				"config":  cfg,
				"message": "图片不存在",
			})
			return
		}

		c.HTML(http.StatusOK, "info.html", gin.H{
			"config": cfg,
			"info":   info,
		})
	}
}

func Download(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		dw := c.Query("dw")
		if dw == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid parameter"})
			return
		}
		if metadata, ok := service.GetImageMetadata(dw); ok && metadata.StorageType == "s3" {
			if metadata.URL != "" {
				c.Redirect(http.StatusFound, metadata.URL)
				return
			}
		}

		// 验证路径安全性（filepath.Clean 规范化后检查 ".."，防止 Windows 反斜杠绕过）
		cleanPath, err := service.ValidateURLPath(dw, cfg.Path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// 使用验证后的安全路径
		safePath := filepath.Join(".", cleanPath)
		if _, err := os.Stat(safePath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
			return
		}

		c.FileAttachment(safePath, filepath.Base(safePath))
	}
}

func Thumbnail(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		img := c.Query("img")
		if img == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid parameter"})
			return
		}
		if metadata, ok := service.GetImageMetadata(img); ok && metadata.StorageType == "s3" {
			if metadata.ThumbURL != "" {
				c.Redirect(http.StatusFound, metadata.ThumbURL)
				return
			}
			if metadata.URL != "" {
				c.Redirect(http.StatusFound, metadata.URL)
				return
			}
		}

		// 生成缩略图
		thumbPath, err := service.GenerateThumbnail(img, cfg)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate thumbnail"})
			return
		}

		c.File(thumbPath)
	}
}

func HideImage(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.Query("key")
		if key == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid parameter"})
			return
		}

		// 解密key获取图片路径
		path, err := service.DecryptHideKey(key, cfg.HideKey)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invalid key"})
			return
		}

		// 验证路径安全性（filepath.Clean 规范化后检查 ".."，防止 Windows 反斜杠绕过）
		cleanPath, err := service.ValidateURLPath(path, cfg.Path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// 使用验证后的安全路径
		safePath := filepath.Join(".", cleanPath)
		if _, err := os.Stat(safePath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
			return
		}

		c.File(safePath)
	}
}

func Install(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查是否已安装
		if _, err := os.Stat("config/install.lock"); err == nil {
			c.Redirect(http.StatusFound, "/")
			return
		}

		c.HTML(http.StatusOK, "install.html", gin.H{
			"config": cfg,
		})
	}
}

func InstallAction(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查是否已安装（防止安装锁绕过）
		if _, err := os.Stat("config/install.lock"); err == nil {
			c.JSON(http.StatusForbidden, gin.H{
				"result":  "failed",
				"message": "系统已安装，禁止重复安装",
			})
			return
		}

		// 处理安装
		domain := c.PostForm("domain")
		user := c.PostForm("user")
		password := c.PostForm("password")

		if domain != "" {
			cfg.Domain = domain
			cfg.ImageURL = domain
		}
		if user != "" {
			cfg.User = user
		}
		if password != "" {
			cfg.Password = service.HashPassword(password)
		}

		if err := config.Save(cfg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"result":  "failed",
				"message": "保存配置失败",
			})
			return
		}

		// 创建安装锁
		if err := os.WriteFile("config/install.lock", []byte("installed"), 0644); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"result":  "failed",
				"message": "创建安装锁失败: " + err.Error(),
			})
			return
		}

		c.Redirect(http.StatusFound, "/")
	}
}

func AdminIndex(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		redirect := safeLoginRedirect(c.Query("redirect"), "/admin/manager")
		if service.IsAdmin(c) {
			c.Redirect(http.StatusFound, redirect)
			return
		}

		captchaData := service.GenerateCaptcha(cfg)
		c.HTML(http.StatusOK, "admin_login.html", gin.H{
			"config":   cfg,
			"captcha":  captchaData,
			"redirect": redirect,
		})
	}
}

func AdminLogin(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		redirect := safeLoginRedirect(c.PostForm("redirect"), "/admin/manager")

		// 检查登录速率限制
		clientIP := c.ClientIP()
		if !service.CheckLoginRateLimit(clientIP) {
			c.HTML(http.StatusTooManyRequests, "admin_login.html", gin.H{
				"config":   cfg,
				"error":    "登录尝试过于频繁，请5分钟后再试",
				"redirect": redirect,
			})
			return
		}

		// 验证验证码
		log.Printf("[Login] captcha=%d, captcha_type=%d, answer=%q, token=%q",
			cfg.Captcha, cfg.CaptchaType,
			c.PostForm("captcha_answer"), c.PostForm("captcha_token"))
		if cfg.Captcha == 1 {
			ok, msg := service.VerifyCaptcha(cfg,
				c.PostForm("captcha_answer"),
				c.PostForm("captcha_token"),
				c.PostForm("cf_turnstile_response"),
				c.PostForm("g_recaptcha_response"),
			)
			if !ok {
				captchaData := service.GenerateCaptcha(cfg)
				log.Printf("[Login page] captcha=%d, captcha_type=%d, captcha_data_type=%s, question=%q",
					cfg.Captcha, cfg.CaptchaType, captchaData.Type, captchaData.Question)
				c.HTML(http.StatusOK, "admin_login.html", gin.H{
					"config":   cfg,
					"error":    msg,
					"captcha":  captchaData,
					"redirect": redirect,
				})
				return
			}
		}

		user := c.PostForm("user")
		password := c.PostForm("password")

		success, message := service.ValidateLogin(user, password, cfg)
		if success {
			service.ResetLoginAttempts(clientIP)
			service.SetAdminSession(c, user)
			c.Redirect(http.StatusFound, redirect)
			return
		}

		service.RecordFailedLogin(clientIP)
		captchaData := service.GenerateCaptcha(cfg)
		c.HTML(http.StatusOK, "admin_login.html", gin.H{
			"config":   cfg,
			"error":    message,
			"captcha":  captchaData,
			"redirect": redirect,
		})
	}
}

func safeLoginRedirect(raw, fallback string) string {
	redirect := strings.TrimSpace(raw)
	if redirect == "" {
		return fallback
	}
	if !strings.HasPrefix(redirect, "/") || strings.HasPrefix(redirect, "//") || strings.Contains(redirect, "\\") {
		return fallback
	}
	return redirect
}

func Manager(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.HTML(http.StatusOK, "admin_manager.html", adminShellData(c, cfg, "manager"))
	}
}

func ManagerAction(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		action := c.PostForm("action")

		switch action {
		case "save_config":
			// 直接读取表单值并更新白名单字段。不使用 ShouldBind，因为无法区分
			// "字段未提交"和"字段提交为空值"（两者都变成零值）。
			// 字符串字段：仅在非空时更新（防止意外清空）。
			// 数值字段：解析成功且 > 0 时更新（防止零值覆盖）。
			// 选择框字段：始终有值，直接更新。

			if v := c.PostForm("title"); v != "" {
				cfg.Title = v
			}
			if v := c.PostForm("keywords"); v != "" {
				cfg.Keywords = v
			}
			if v := c.PostForm("description"); v != "" {
				cfg.Description = v
			}
			if v := c.PostForm("tips"); v != "" {
				cfg.Tips = v
			}
			if v := c.PostForm("notice"); v != "" {
				cfg.Notice = v
			}
			if v := c.PostForm("domain"); v != "" {
				cfg.Domain = v
				cfg.ImageURL = v
			}
			if v := c.PostForm("maxSize"); v != "" {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
					cfg.MaxSize = n
				}
			}
			if v := c.PostForm("maxUploadFiles"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					cfg.MaxUploadFiles = n
				}
			}
			if v := c.PostForm("extensions"); v != "" {
				cfg.Extensions = v
			}
			if v := c.PostForm("compress_ratio"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					cfg.CompressRatio = n
				}
			}
			if v := c.PostForm("thumbnail"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					cfg.Thumbnail = n
				}
			}
			if v := c.PostForm("thumbnail_w"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					cfg.ThumbnailW = n
				}
			}
			if v := c.PostForm("thumbnail_h"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					cfg.ThumbnailH = n
				}
			}
			if v := c.PostForm("webp_convert"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					cfg.WebpConvert = n
				}
			}
			if v := c.PostForm("webp_quality"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					cfg.WebpQuality = n
				}
			}
			if v := c.PostForm("watermark"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					cfg.Watermark = n
				}
			}
			if v := c.PostForm("waterText"); v != "" {
				cfg.WaterText = v
			}
			if v := c.PostForm("waterPosition"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					cfg.WaterPosition = n
				}
			}
			if v := c.PostForm("textColor"); v != "" {
				cfg.TextColor = v
			}
			if v := c.PostForm("textSize"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					cfg.TextSize = n
				}
			}
			if v := c.PostForm("textFont"); v != "" {
				cfg.TextFont = v
			}
			if v := c.PostForm("waterImg"); v != "" {
				cfg.WaterImg = v
			}
			if v := c.PostForm("mime"); v != "" {
				cfg.Mime = v
			}
			if v := c.PostForm("storage_path"); v != "" {
				cfg.StoragePath = v
			}
			if v := c.PostForm("time_format"); v != "" {
				cfg.TimeFormat = v
			}
			if v := c.PostForm("auto_delete"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					cfg.AutoDelete = n
				}
			}
			if v := c.PostForm("mustLogin"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					cfg.MustLogin = n
				}
			}
			if v := c.PostForm("captcha"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					cfg.Captcha = n
				}
			}
			if v := c.PostForm("captcha_type"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					cfg.CaptchaType = n
				}
			}
			if v := c.PostForm("turnstile_site_key"); v != "" {
				cfg.TurnstileSiteKey = v
			}
			if v := c.PostForm("turnstile_secret_key"); v != "" {
				cfg.TurnstileSecretKey = v
			}
			if v := c.PostForm("recaptcha_site_key"); v != "" {
				cfg.RecaptchaSiteKey = v
			}
			if v := c.PostForm("recaptcha_secret_key"); v != "" {
				cfg.RecaptchaSecretKey = v
			}
			if v := c.PostForm("hotlink_protect"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					cfg.HotlinkProtect = n
				}
			}
			// hotlink_domains 允许清空（textarea），始终更新
			cfg.HotlinkDomains = c.PostForm("hotlink_domains")
			// 注意：不更新 Password, User, Path, Port, HideKey 等敏感字段

			if err := config.Save(cfg); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"result": "error",
					"msg":    "保存失败",
				})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"result": "success",
				"msg":    "保存成功",
			})

		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"result": "error",
				"msg":    "未知操作",
			})
		}
	}
}

// BatchWebP 批量为存量图片生成 WebP 格式
func BatchWebP(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg.WebpConvert == 0 {
			c.JSON(http.StatusOK, gin.H{
				"result":  "failed",
				"message": "WebP转换未开启，请先在配置中开启",
			})
			return
		}

		result := service.BatchConvertToWebP(cfg)

		c.JSON(http.StatusOK, gin.H{
			"result":    "success",
			"total":     result.Total,
			"skipped":   result.Skipped,
			"converted": result.Converted,
			"failed":    result.Failed,
			"message":   fmt.Sprintf("扫描 %d 张图片，转换 %d 张，跳过 %d 张，失败 %d 张", result.Total, result.Converted, result.Skipped, result.Failed),
		})
	}
}

func Chart(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.HTML(http.StatusOK, "admin_chart.html", adminShellData(c, cfg, "chart"))
	}
}

// History 历史上传图片（原广场功能）
func History(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.HTML(http.StatusOK, "admin_history.html", adminShellData(c, cfg, "history"))
	}
}

// HistoryDelete 历史图片删除（支持POST表单）
func HistoryDelete(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 支持POST表单
		url := c.PostForm("url")
		mode := c.DefaultPostForm("mode", "delete")

		if url == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
			return
		}

		switch mode {
		case "delete":
			if err := service.DeleteFile(url); err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "删除失败", "type": "danger", "icon": "exclamation-sign"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功", "type": "success", "icon": "ok-sign"})
		case "recycle":
			if err := service.MoveToRecycle(url, cfg); err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "回收失败", "type": "danger", "icon": "exclamation-sign"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "回收成功", "type": "success", "icon": "ok-sign"})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "无效操作"})
		}
	}
}

func Filer(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.HTML(http.StatusOK, "admin_filer.html", adminShellData(c, cfg, "filer"))
	}
}

// ImageURLList 图片URL列表页面
func ImageURLList(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.HTML(http.StatusOK, "admin_urllist.html", adminShellData(c, cfg, "urllist"))
	}
}

// ImageURLListAPI 图片URL列表API（JSON）
func ImageURLListAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqPath := c.DefaultQuery("path", cfg.Path)
		pageStr := c.DefaultQuery("page", "1")
		pageSizeStr := c.DefaultQuery("page_size", "100")

		page, _ := strconv.Atoi(pageStr)
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(pageSizeStr)
		if pageSize < 1 || pageSize > 500 {
			pageSize = 100
		}

		// 验证路径安全性（filepath.Clean 规范化后检查 ".."，防止 Windows 反斜杠绕过）
		reqPath, err := service.ValidateURLPath(reqPath, cfg.Path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}
		if !strings.HasSuffix(reqPath, "/") {
			reqPath += "/"
		}

		fsPath := "." + reqPath
		// 获取所有文件并过滤图片（GetFileListRecursive 已跳过系统内部目录）
		rawFiles := service.GetFileListRecursive(fsPath)
		var allFiles []string
		for _, name := range rawFiles {
			// 排除webp目录下的重复文件和非图片文件
			if !strings.HasPrefix(name, "webp/") && service.IsImageFile(name) {
				allFiles = append(allFiles, name)
			}
		}

		total := len(allFiles)
		start := (page - 1) * pageSize
		end := start + pageSize
		if start > total {
			start = total
		}
		if end > total {
			end = total
		}

		type FileInfo struct {
			Name    string `json:"name"`
			URL     string `json:"url"`
			WebPURL string `json:"webp_url,omitempty"`
		}

		files := make([]FileInfo, 0, end-start)
		for _, name := range allFiles[start:end] {
			relativePath := reqPath + name
			url := cfg.Domain + relativePath
			webpURL := service.GetWebPURL(relativePath, cfg)

			files = append(files, FileInfo{
				Name:    name,
				URL:     url,
				WebPURL: webpURL,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"total":       total,
			"page":        page,
			"page_size":   pageSize,
			"total_pages": (total + pageSize - 1) / pageSize,
			"files":       files,
		})
	}
}

func AdminDelete(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			URL  string `json:"url"`
			Mode string `json:"mode"`
			Date string `json:"date"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code": 400,
				"msg":  "Invalid request",
			})
			return
		}

		switch req.Mode {
		case "delete":
			if err := service.DeleteFile(req.URL); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"code": 404,
					"msg":  "删除失败",
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"code": 200,
				"msg":  "删除成功",
			})

		case "recycle":
			if err := service.MoveToRecycle(req.URL, cfg); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"code": 404,
					"msg":  "回收失败",
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"code": 200,
				"msg":  "回收成功",
			})

		case "delDir":
			dirPath := cfg.Path + req.URL
			if err := service.DeleteDirectory(dirPath); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"code": 404,
					"msg":  "删除文件夹失败",
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"code": 200,
				"msg":  "删除文件夹成功",
			})

		case "recycle_reimg":
			if err := service.RestoreFromRecycle(req.URL, cfg); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"code": 404,
					"msg":  "恢复失败",
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"code": 200,
				"msg":  "恢复成功",
			})

		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"code": 400,
				"msg":  "Invalid mode",
			})
		}
	}
}

func init() {
	// 设置时区
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	time.Local = loc
}
