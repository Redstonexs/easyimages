package handler

import (
	"easyimage/config"
	"easyimage/internal/service"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func Index(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查登录状态
		isAdmin := service.IsAdmin(c)

		data := gin.H{
			"config":  cfg,
			"version": config.Version,
			"isAdmin": isAdmin,
		}
		c.HTML(http.StatusOK, "index.html", data)
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
		listDate := cfg.ListDate
		datePath := c.DefaultQuery("date", time.Now().Format("2006/01/02/"))

		// 限制日期范围
		if datePath < time.Now().AddDate(0, 0, -listDate).Format("2006/01/02/") {
			datePath = time.Now().Format("2006/01/02/")
		}

		num := c.DefaultQuery("num", fmt.Sprintf("%d", cfg.ListNumber))
		search := c.Query("search")

		// 验证 search 参数只包含字母数字（防止 glob 注入）
		if search != "" {
			for _, ch := range search {
				if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid search parameter"})
					return
				}
			}
		}

		// 获取文件列表
		basePath := cfg.Path + datePath
		if search != "" {
			basePath += "*." + search
		} else {
			basePath += "*.*"
		}

		files := service.GetFileList(basePath, cfg.ShowSort)
		allUpload := service.GetFileCount(cfg.Path + datePath)

		// 生成日期链接数据
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006/01/02/")
		dateLinks := make([]gin.H, 0, listDate)
		for i := 2; i <= listDate; i++ {
			date := time.Now().AddDate(0, 0, -i).Format("2006/01/02/")
			dateLinks = append(dateLinks, gin.H{
				"Date":  date,
				"Label": fmt.Sprintf("%d天前", i),
			})
		}

		c.HTML(http.StatusOK, "list.html", gin.H{
			"config":     cfg,
			"files":      files,
			"date":       datePath,
			"num":        num,
			"allUpload":  allUpload,
			"search":     search,
			"listDate":   listDate,
			"yesterday":  yesterday,
			"dateLinks":  dateLinks,
		})
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
				"result":  "success",
				"code":    200,
				"files":   results,
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
			if err != nil || chunkIndex < 0 {
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

		chunk, _ := c.FormFile("chunk")

		// 保存分片（如果有实际文件数据）
		chunkDir := filepath.Join(".", cfg.Path, "chunks", uploadId)
		if chunk != nil {
			if err := os.MkdirAll(chunkDir, 0755); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "创建分片目录失败"})
				return
			}
			chunkPath := filepath.Join(chunkDir, fmt.Sprintf("%06d", chunkIndex))
			if err := c.SaveUploadedFile(chunk, chunkPath); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "保存分片失败"})
				return
			}
		}

		// 并发上传时，最后一个到达的分片不一定是 totalChunks-1。
		// 不再自动合并，客户端需发送 merge=true 参数显式触发合并。
		if !isMerge {
			if chunk == nil {
				c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "缺少分片数据"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"result": "success", "code": 200, "message": "分片上传成功", "chunkIndex": chunkIndex})
			return
		}

		// === 合并所有分片 ===
		// 先验证所有分片都已上传完成
		for i := 0; i < totalChunks; i++ {
			partPath := filepath.Join(chunkDir, fmt.Sprintf("%06d", i))
			if _, err := os.Stat(partPath); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"result": "failed", "code": 400,
					"message": fmt.Sprintf("分片 %d 尚未上传，无法合并", i),
				})
				return
			}
		}

		if filename == "" {
			filename = "upload"
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
		if !service.IsAllowedExtension(filename, cfg) {
			os.RemoveAll(chunkDir)
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "不支持的文件格式: " + ext})
			return
		}

		baseName := strings.TrimSuffix(filename, filepath.Ext(filename))
		newFileName := service.GenerateFileName(baseName, cfg.ImgName) + "." + ext
		now := time.Now()
		storagePath := cfg.StoragePath
		storagePath = strings.Replace(storagePath, "Y", fmt.Sprintf("%04d", now.Year()), 1)
		storagePath = strings.Replace(storagePath, "m", fmt.Sprintf("%02d", now.Month()), 1)
		storagePath = strings.Replace(storagePath, "d", fmt.Sprintf("%02d", now.Day()), 1)
		uploadDir := filepath.Join(".", cfg.Path, storagePath)
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			os.RemoveAll(chunkDir)
			c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "创建存储目录失败"})
			return
		}
		finalPath := filepath.Join(uploadDir, newFileName)

		// 合并分片到目标文件
		outFile, err := os.Create(finalPath)
		if err != nil {
			os.RemoveAll(chunkDir)
			c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": "创建目标文件失败"})
			return
		}
		var totalSize int64
		for i := 0; i < totalChunks; i++ {
			partPath := filepath.Join(chunkDir, fmt.Sprintf("%06d", i))
			partFile, err := os.Open(partPath)
			if err != nil {
				outFile.Close()
				os.Remove(finalPath)
				os.RemoveAll(chunkDir)
				c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": fmt.Sprintf("缺少分片 %d", i)})
				return
			}
			n, err := io.Copy(outFile, partFile)
			partFile.Close()
			if err != nil {
				outFile.Close()
				os.Remove(finalPath)
				os.RemoveAll(chunkDir)
				c.JSON(http.StatusInternalServerError, gin.H{"result": "failed", "code": 500, "message": fmt.Sprintf("合并分片 %d 失败: %v", i, err)})
				return
			}
			totalSize += n
		}
		outFile.Close()
		os.RemoveAll(chunkDir) // 清理分片

		// 检查文件大小
		if totalSize > cfg.MaxSize {
			os.Remove(finalPath)
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": fmt.Sprintf("文件大小超过限制: %s", service.FormatSize(cfg.MaxSize))})
			return
		}

		// SVG 安全检查
		if ext == "svg" && !service.CheckSVGSecurity(finalPath) {
			os.Remove(finalPath)
			c.JSON(http.StatusBadRequest, gin.H{"result": "failed", "code": 400, "message": "SVG文件包含不安全内容"})
			return
		}

		// 生成URL
		relativePath := cfg.Path + storagePath + newFileName
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

		go service.ProcessImageAfterUpload(finalPath, cfg)

		// 生成WebP URL（与 ProcessUpload 保持一致）
		webpURL := ""
		if cfg.WebpConvert == 1 {
			webpRelativePath := cfg.Path + "webp/" + storagePath + newFileName
			webpURL = cfg.Domain + webpRelativePath
			if cfg.HidePath == 1 {
				webpURL = strings.Replace(webpURL, cfg.Path, "/", 1)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"result": "success", "code": 200,
			"url": imageURL, "srcName": baseName, "thumb": thumbURL, "del": delURL, "webp_url": webpURL,
		})
	}
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

		chunkDir := filepath.Join(".", cfg.Path, "chunks", uploadId)
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
				"message": "图片参数错误",
			})
			return
		}

		// 获取图片信息
		info, err := service.GetImageInfo(img, cfg)
		if err != nil {
			c.HTML(http.StatusNotFound, "error.html", gin.H{
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
		if service.IsAdmin(c) {
			c.Redirect(http.StatusFound, "/admin/manager")
			return
		}

		c.HTML(http.StatusOK, "admin_login.html", gin.H{
			"config": cfg,
		})
	}
}

func AdminLogin(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查登录速率限制
		clientIP := c.ClientIP()
		if !service.CheckLoginRateLimit(clientIP) {
			c.HTML(http.StatusTooManyRequests, "admin_login.html", gin.H{
				"config": cfg,
				"error":  "登录尝试过于频繁，请5分钟后再试",
			})
			return
		}

		user := c.PostForm("user")
		password := c.PostForm("password")

		success, message := service.ValidateLogin(user, password, cfg)
		if success {
			service.ResetLoginAttempts(clientIP)
			service.SetAdminSession(c, user)
			c.Redirect(http.StatusFound, "/admin/manager")
			return
		}

		service.RecordFailedLogin(clientIP)
		c.HTML(http.StatusOK, "admin_login.html", gin.H{
			"config":  cfg,
			"error":   message,
		})
	}
}

func Manager(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 转换为文件系统路径
		fsPath := "." + cfg.Path

		// 单次遍历统计文件数和总大小
		stats := service.CollectDirStats(fsPath)

		c.HTML(http.StatusOK, "admin_manager.html", gin.H{
			"config":      cfg,
			"totalFiles":  stats.TotalFiles,
			"usedSpace":   stats.TotalSize,
			"version":     config.Version,
		})
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

func Chart(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 转换为文件系统路径
		fsPath := "." + cfg.Path

		// 单次遍历收集文件数、总大小和按扩展名统计
		stats := service.CollectDirStats(fsPath)

		// 获取最近30天的上传统计
		dailyStats := make([]gin.H, 0, 30)
		for i := 0; i < 30; i++ {
			date := time.Now().AddDate(0, 0, -i)
			datePath := date.Format("2006/01/02/")
			count := service.GetFileCount(fsPath + datePath + "*.*")
			dailyStats = append(dailyStats, gin.H{
				"Date":  date.Format("01-02"),
				"Count": count,
			})
		}

		// 反转顺序（从旧到新）
		slices.Reverse(dailyStats)

		c.HTML(http.StatusOK, "admin_chart.html", gin.H{
			"config":      cfg,
			"totalFiles":  stats.TotalFiles,
			"usedSpace":   stats.TotalSize,
			"dailyStats":  dailyStats,
			"formatStats": stats.ByExt,
		})
	}
}

// History 历史上传图片（原广场功能）
func History(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		listDate := cfg.ListDate
		datePath := c.DefaultQuery("date", time.Now().Format("2006/01/02/"))

		// 限制日期范围
		if datePath < time.Now().AddDate(0, 0, -listDate).Format("2006/01/02/") {
			datePath = time.Now().Format("2006/01/02/")
		}

		// 确保日期路径格式正确（末尾加/）
		if !strings.HasSuffix(datePath, "/") {
			datePath += "/"
		}

		search := c.Query("search")

		// 验证 search 参数只包含字母数字（防止 glob 注入）
		if search != "" {
			for _, ch := range search {
				if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid search parameter"})
					return
				}
			}
		}

		// 转换为文件系统路径
		fsPath := "." + cfg.Path

		// 获取文件列表
		basePath := fsPath + datePath
		if search != "" {
			basePath += "*." + search
		} else {
			basePath += "*.*"
		}

		files := service.GetFileList(basePath, cfg.ShowSort)
		allUpload := service.GetFileCount(fsPath + datePath + "*.*")

		// 生成日期链接数据
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006/01/02/")
		dateLinks := make([]gin.H, 0, listDate)
		for i := 2; i <= listDate; i++ {
			date := time.Now().AddDate(0, 0, -i)
			dateLinks = append(dateLinks, gin.H{
				"Date":  date.Format("2006/01/02/"),
				"Label": fmt.Sprintf("%d天前", i),
			})
		}

		c.HTML(http.StatusOK, "admin_history.html", gin.H{
			"config":     cfg,
			"files":      files,
			"date":       datePath,
			"allUpload":  allUpload,
			"search":     search,
			"listDate":   listDate,
			"yesterday":  yesterday,
			"dateLinks":  dateLinks,
		})
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
		reqPath := c.DefaultQuery("path", cfg.Path)

		// 验证路径安全性（filepath.Clean 规范化后检查 ".."，防止 Windows 反斜杠绕过）
		reqPath, err := service.ValidateURLPath(reqPath, cfg.Path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// 确保路径以/结尾
		if !strings.HasSuffix(reqPath, "/") {
			reqPath += "/"
		}

		// 获取目录列表和文件列表
		dirs := service.GetDirList("." + reqPath)
		files := service.GetFileList("." + reqPath + "*.*", cfg.ShowSort)

		// 计算上级目录
		parentPath := ""
		if reqPath != cfg.Path {
			parentPath = filepath.Dir(strings.TrimSuffix(reqPath, "/"))
			if !strings.HasSuffix(parentPath, "/") {
				parentPath += "/"
			}
			// 确保上级目录不超出允许范围
			if !strings.HasPrefix(parentPath, cfg.Path) {
				parentPath = cfg.Path
			}
		}

		c.HTML(http.StatusOK, "admin_filer.html", gin.H{
			"config":     cfg,
			"files":      files,
			"dirs":       dirs,
			"path":       reqPath,
			"parentPath": parentPath,
		})
	}
}

// ImageURLList 图片URL列表页面
func ImageURLList(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqPath := c.DefaultQuery("path", cfg.Path)
		pageStr := c.DefaultQuery("page", "1")
		pageSizeStr := c.DefaultQuery("page_size", "50")

		page, _ := strconv.Atoi(pageStr)
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(pageSizeStr)
		if pageSize < 1 || pageSize > 200 {
			pageSize = 50
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
		allFiles := service.GetFileListRecursive(fsPath)
		var imageFiles []string
		for _, name := range allFiles {
			// 排除webp目录下的重复文件和非图片文件
			if !strings.HasPrefix(name, "webp/") && service.IsImageFile(name) {
				imageFiles = append(imageFiles, name)
			}
		}
		allFiles = imageFiles

		// 计算分页
		total := len(allFiles)
		totalPages := (total + pageSize - 1) / pageSize
		start := (page - 1) * pageSize
		end := start + pageSize
		if start > total {
			start = total
		}
		if end > total {
			end = total
		}

		// 构建文件URL列表
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

		c.HTML(http.StatusOK, "admin_urllist.html", gin.H{
			"config":     cfg,
			"files":      files,
			"path":       reqPath,
			"page":       page,
			"pageSize":   pageSize,
			"total":      total,
			"totalPages": totalPages,
		})
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
			"total":      total,
			"page":       page,
			"page_size":  pageSize,
			"total_pages": (total + pageSize - 1) / pageSize,
			"files":      files,
		})
	}
}

func AdminDelete(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			URL   string `json:"url"`
			Mode  string `json:"mode"`
			Date  string `json:"date"`
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
