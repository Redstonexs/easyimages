package main

import (
	"easyimage/config"
	"easyimage/internal/handler"
	"easyimage/internal/middleware"
	"easyimage/internal/service"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// 自动迁移检测
	if err := checkAndMigrate(); err != nil {
		log.Printf("Warning: auto migration failed: %v", err)
	}

	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 创建必要目录（cfg.Path 是 URL 路径如 "/i/"，文件系统路径需要 "." 前缀）
	dirs := []string{
		"." + cfg.Path,
		"." + cfg.Path + "/cache",
		"." + cfg.Path + "/suspic",
		"." + cfg.Path + "/recycle",
		"." + cfg.Path + "/webp",
		"admin/logs/upload",
		"admin/logs/ipcounts",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("Warning: failed to create directory %s: %v", dir, err)
		}
	}

	// 检查 cwebp 是否可用（WebP 转换依赖此 CLI 工具）
	if cfg.WebpConvert == 1 {
		if _, err := exec.LookPath("cwebp"); err != nil {
			log.Printf("WARNING: WebP conversion is enabled but 'cwebp' is not found in PATH. "+
				"WebP files will NOT be generated. Install libwebp-tools (apk add libwebp-tools) "+
				"or disable WebP conversion in settings. Error: %v", err)
		}
	}

	// 定期清理过期的分片目录（浏览器崩溃等异常情况无法触发 cleanup）
	go func() {
		chunksDir := "." + cfg.Path + "/chunks"
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			entries, err := os.ReadDir(chunksDir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				info, err := entry.Info()
				if err != nil {
					continue
				}
				// 超过1小时的分片目录视为过期
				if time.Since(info.ModTime()) > time.Hour {
					os.RemoveAll(filepath.Join(chunksDir, entry.Name()))
				}
			}
		}
	}()

	// 设置Gin模式
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(frontendCacheHeaders())
	vite := loadViteAssets("public/dist/.vite/manifest.json")

	// 设置 multipart 表单的内存阈值。高并发上传时，过大的内存缓冲会快速放大
	// RSS 和 GC 压力；超过阈值的部分由 net/http 使用临时文件承接。
	r.MaxMultipartMemory = 16 << 20 // 16MB

	// 注册自定义模板函数
	r.SetFuncMap(template.FuncMap{
		"vite":        vite.Tags,
		"json_script": jsonForTemplate,
		"format_size": service.FormatSize,
		"mul": func(a, b interface{}) int {
			var ai, bi int
			switch v := a.(type) {
			case int:
				ai = v
			case float64:
				ai = int(v)
			}
			switch v := b.(type) {
			case int:
				bi = v
			case float64:
				bi = int(v)
			}
			return ai * bi
		},
		"div": func(a, b interface{}) int {
			var ai, bi int
			switch v := a.(type) {
			case int:
				ai = v
			case float64:
				ai = int(v)
			}
			switch v := b.(type) {
			case int:
				bi = v
			case float64:
				bi = int(v)
			}
			if bi == 0 {
				return 0
			}
			return ai / bi
		},
		"minus": func(a, b int) int {
			return a - b
		},
		"len": func(v interface{}) int {
			switch val := v.(type) {
			case []interface{}:
				return len(val)
			case []gin.H:
				return len(val)
			default:
				return 0
			}
		},
		"index": func(arr interface{}, keys ...interface{}) interface{} {
			if len(keys) == 0 {
				return nil
			}
			current := arr
			for _, key := range keys {
				switch c := current.(type) {
				case []gin.H:
					if i, ok := key.(int); ok && i >= 0 && i < len(c) {
						current = c[i]
					} else {
						return nil
					}
				case []interface{}:
					if i, ok := key.(int); ok && i >= 0 && i < len(c) {
						current = c[i]
					} else {
						return nil
					}
				case gin.H:
					current = c[key.(string)]
				case map[string]interface{}:
					current = c[key.(string)]
				default:
					return nil
				}
			}
			return current
		},
		"trimSuffix": func(suffix, s string) string {
			return strings.TrimSuffix(s, suffix)
		},
		"now": func() string {
			return time.Now().Format("2006/01/02/")
		},
	})

	// 加载HTML模板
	r.LoadHTMLGlob("templates/*")

	// 静态文件
	r.Static("/public", "./public")
	// 图片静态文件服务
	// cfg.Path 通常是 "/i/"，需要注册 "/i" 路由来匹配请求
	imgRoutePath := strings.TrimRight(cfg.Path, "/")
	imgGroup := r.Group(imgRoutePath)
	imgGroup.Use(middleware.HotlinkProtection(cfg))
	imgGroup.Static("/", "."+cfg.Path)

	// favicon
	r.GET("/favicon.ico", handler.SiteIcon(cfg))

	// 页面路由
	r.GET("/", handler.Index(cfg))
	r.GET("/app/list", handler.List(cfg))
	r.GET("/app/info", handler.Info(cfg))
	r.GET("/app/down", handler.Download(cfg))
	r.GET("/app/thumb", handler.Thumbnail(cfg))
	r.GET("/app/hide", handler.HideImage(cfg))
	r.POST("/app/del_hash", handler.DeleteByHash(cfg))

	// 登录API
	r.POST("/api/login", handler.AdminLoginAPI(cfg))
	r.GET("/api/captcha", handler.CaptchaAPI(cfg))

	// 上传路由
	r.POST("/app/upload", middleware.CheckLogin(cfg), handler.Upload(cfg))
	r.POST("/app/upload/chunk", middleware.CheckLogin(cfg), handler.ChunkUpload(cfg))
	r.POST("/app/upload/chunk/cleanup", middleware.CheckLogin(cfg), handler.ChunkCleanup(cfg))

	// API路由
	r.POST("/api/index", handler.APIUpload(cfg))
	r.GET("/api/list", handler.ImageListAPI(cfg))
	r.GET("/api/urllist", handler.ImageURLListAPI(cfg))
	r.OPTIONS("/api/index", func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Status(200)
	})

	// 管理后台
	admin := r.Group("/admin")
	{
		admin.GET("/index", handler.AdminIndex(cfg))
		admin.POST("/index", handler.AdminLogin(cfg))
		admin.GET("/manager", middleware.RequireAdmin(cfg), handler.Manager(cfg))
		admin.POST("/manager", middleware.RequireAdmin(cfg), handler.ManagerAction(cfg))
		admin.POST("/batch-webp", middleware.RequireAdmin(cfg), handler.BatchWebP(cfg))
		admin.GET("/chart", middleware.RequireAdmin(cfg), handler.Chart(cfg))
		admin.GET("/history", middleware.RequireAdmin(cfg), handler.History(cfg))
		admin.POST("/history", middleware.RequireAdmin(cfg), handler.HistoryDelete(cfg))
		admin.GET("/urllist", middleware.RequireAdmin(cfg), handler.ImageURLList(cfg))
		admin.GET("/filer", middleware.RequireAdmin(cfg), handler.Filer(cfg))
		admin.POST("/del", middleware.RequireAdmin(cfg), handler.AdminDelete(cfg))

		api := admin.Group("/api", middleware.RequireAdmin(cfg))
		api.GET("/overview", handler.AdminOverviewAPI(cfg))
		api.GET("/config", handler.AdminConfigAPI(cfg))
		api.POST("/config", handler.AdminConfigSaveAPI(cfg))
		api.POST("/site-icon", handler.AdminSiteIconUploadAPI(cfg))
		api.POST("/batch-webp", handler.AdminBatchWebPAPI(cfg))
		api.GET("/chart", handler.AdminChartAPI(cfg))
		api.GET("/history", handler.AdminHistoryAPI(cfg))
		api.POST("/history/delete", handler.AdminHistoryDeleteAPI(cfg))
		api.GET("/urllist", handler.AdminURLListAPI(cfg))
		api.GET("/filer", handler.AdminFilerAPI(cfg))
		api.POST("/delete", handler.AdminDeleteAPI(cfg))
	}

	// 删除操作
	r.POST("/app/del", middleware.CheckLogin(cfg), handler.DeleteAction(cfg))

	// 安装页面
	r.GET("/install/", handler.Install(cfg))
	r.POST("/install/", handler.InstallAction(cfg))

	// 启动服务器 - 使用自定义 http.Server 以设置超时
	// 大文件上传场景下，默认无超时或过短超时会导致连接断开
	addr := fmt.Sprintf(":%d", cfg.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 30 * time.Second,  // 读取请求头超时
		ReadTimeout:       5 * time.Minute,   // 读取整个请求体超时（含大文件上传）
		WriteTimeout:      5 * time.Minute,   // 写入响应超时
		IdleTimeout:       120 * time.Second, // 空闲连接超时
		MaxHeaderBytes:    1 << 20,           // 1MB 最大请求头
	}
	log.Printf("EasyImage starting on %s", addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// checkAndMigrate 检测并执行自动迁移
func checkAndMigrate() error {
	// 检查是否已有Go配置（非默认配置）
	if config.HasInstalledOrCustomizedGoConfig() {
		log.Println("Valid Go config exists, skipping migration")
		return nil
	}

	// 检查是否存在可迁移的PHP配置。仓库内置的默认PHP配置只作为示例，
	// 不能触发自动迁移，否则全新安装会被安装锁跳过。
	if !config.HasMigratablePHPConfig() {
		log.Println("No migratable PHP config found, skipping migration")
		return nil
	}

	log.Println("========================================")
	log.Println("检测到PHP版本配置，开始自动迁移...")
	log.Println("========================================")

	// 备份PHP配置
	if err := config.BackupPHPConfig(); err != nil {
		log.Printf("Warning: failed to backup PHP config: %v", err)
	}

	// 迁移数据目录
	if err := config.MigratePHPData(); err != nil {
		return fmt.Errorf("failed to migrate PHP data: %w", err)
	}

	// 执行自动迁移
	if err := config.AutoMigrate(); err != nil {
		return fmt.Errorf("failed to auto migrate: %w", err)
	}

	log.Println("========================================")
	log.Println("自动迁移完成！")
	log.Println("PHP配置已备份到 config/php_backup/ 目录")
	log.Println("========================================")

	return nil
}
