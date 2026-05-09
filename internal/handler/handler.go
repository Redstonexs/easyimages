package handler

import (
	"easyimage/config"
	"easyimage/internal/service"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
		user := c.PostForm("user")
		password := c.PostForm("password")

		// 验证用户名密码
		if user == cfg.User && service.CheckPassword(password, cfg.Password) {
			service.SetAdminSession(c, user)
			c.JSON(http.StatusOK, gin.H{
				"result":  "success",
				"message": "登录成功",
			})
			return
		}

		c.JSON(http.StatusUnauthorized, gin.H{
			"result":  "failed",
			"message": "用户名或密码错误",
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
			c.JSON(http.StatusBadRequest, gin.H{
				"result":  "failed",
				"code":    204,
				"message": "没有选择上传的文件",
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
		hash := c.Query("hash")
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

		// 验证路径安全性，防止路径遍历攻击
		if strings.Contains(dw, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// 确保路径在允许的目录下
		if !strings.HasPrefix(dw, cfg.Path) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// 使用验证后的安全路径
		safePath := filepath.Join(".", filepath.Clean(dw))
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

		// 验证路径安全性，防止路径遍历攻击
		if strings.Contains(path, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// 确保路径在允许的目录下
		if !strings.HasPrefix(path, cfg.Path) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// 使用验证后的安全路径
		safePath := filepath.Join(".", filepath.Clean(path))
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
		os.WriteFile("config/install.lock", []byte("installed"), 0644)

		c.Redirect(http.StatusFound, "/")
	}
}

func AdminIndex(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if service.IsAdmin(c) {
			c.Redirect(http.StatusFound, "/admin/manager.php")
			return
		}

		c.HTML(http.StatusOK, "admin_login.html", gin.H{
			"config": cfg,
		})
	}
}

func AdminLogin(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.PostForm("user")
		password := c.PostForm("password")

		if user == cfg.User && service.CheckPassword(password, cfg.Password) {
			service.SetAdminSession(c, user)
			c.Redirect(http.StatusFound, "/admin/manager.php")
			return
		}

		c.HTML(http.StatusOK, "admin_login.html", gin.H{
			"config":  cfg,
			"error":   "用户名或密码错误",
		})
	}
}

func Manager(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 统计信息
		totalFiles := service.GetFileCount(cfg.Path + "*.*")
		usedSpace := service.GetDirectorySize(cfg.Path)

		c.HTML(http.StatusOK, "admin_manager.html", gin.H{
			"config":      cfg,
			"totalFiles":  totalFiles,
			"usedSpace":   usedSpace,
			"version":     config.Version,
		})
	}
}

func ManagerAction(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		action := c.PostForm("action")

		switch action {
		case "save_config":
			// 保存配置
			if err := c.ShouldBind(cfg); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"result": "error",
					"msg":    "配置格式错误",
				})
				return
			}

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
		c.HTML(http.StatusOK, "admin_chart.html", gin.H{
			"config": cfg,
		})
	}
}

func Filer(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.DefaultQuery("path", cfg.Path)
		files := service.GetFileList(path+"*", cfg.ShowSort)

		c.HTML(http.StatusOK, "admin_filer.html", gin.H{
			"config": cfg,
			"files":  files,
			"path":   path,
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
	loc, _ := time.LoadLocation("Asia/Shanghai")
	time.Local = loc
}
