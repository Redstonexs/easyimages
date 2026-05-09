package main

import (
	"easyimage/config"
	"easyimage/internal/handler"
	"easyimage/internal/middleware"
	"easyimage/internal/service"
	"fmt"
	"html/template"
	"log"
	"os"
	"strings"

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

	// 创建必要目录
	dirs := []string{
		cfg.Path,
		cfg.Path + "/cache",
		cfg.Path + "/suspic",
		cfg.Path + "/recycle",
		"admin/logs/upload",
		"admin/logs/ipcounts",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("Warning: failed to create directory %s: %v", dir, err)
		}
	}

	// 设置Gin模式
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 注册自定义模板函数
	r.SetFuncMap(template.FuncMap{
		"format_size": service.FormatSize,
	})

	// 加载HTML模板
	r.LoadHTMLGlob("templates/*")

	// 静态文件
	r.Static("/public", "./public")
	// 图片静态文件服务
	// cfg.Path 通常是 "/i/"，需要注册 "/i" 路由来匹配请求
	imgRoutePath := strings.TrimRight(cfg.Path, "/")
	r.Static(imgRoutePath, "."+cfg.Path)

	// 如果配置了隐藏路径，还需要处理不带 /i/ 前缀的请求
	if cfg.HidePath == 1 {
		r.StaticFile("/favicon.ico", "./public/images/favicon.ico")
	}

	// 页面路由
	r.GET("/", handler.Index(cfg))
	r.GET("/app/list.php", handler.List(cfg))
	r.GET("/app/info.php", handler.Info(cfg))
	r.GET("/app/down.php", handler.Download(cfg))
	r.GET("/app/thumb.php", handler.Thumbnail(cfg))
	r.GET("/app/hide.php", handler.HideImage(cfg))
	r.GET("/app/del.php", handler.DeleteByHash(cfg))

	// 登录API
	r.POST("/api/login", handler.AdminLoginAPI(cfg))

	// 上传路由
	r.POST("/app/upload.php", middleware.CheckLogin(cfg), handler.Upload(cfg))

	// API路由
	r.POST("/api/index.php", handler.APIUpload(cfg))
	r.OPTIONS("/api/index.php", func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Status(200)
	})

	// 管理后台
	admin := r.Group("/admin")
	{
		admin.GET("/index.php", handler.AdminIndex(cfg))
		admin.POST("/index.php", handler.AdminLogin(cfg))
		admin.GET("/manager.php", middleware.RequireAdmin(cfg), handler.Manager(cfg))
		admin.POST("/manager.php", middleware.RequireAdmin(cfg), handler.ManagerAction(cfg))
		admin.GET("/chart.php", middleware.RequireAdmin(cfg), handler.Chart(cfg))
		admin.GET("/filer.php", middleware.RequireAdmin(cfg), handler.Filer(cfg))
		admin.POST("/del.php", middleware.RequireAdmin(cfg), handler.AdminDelete(cfg))
	}

	// 删除操作
	r.POST("/app/del.php", middleware.CheckLogin(cfg), handler.DeleteAction(cfg))

	// 安装页面
	r.GET("/install/", handler.Install(cfg))
	r.POST("/install/", handler.InstallAction(cfg))

	// 启动服务器
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("EasyImage starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// checkAndMigrate 检测并执行自动迁移
func checkAndMigrate() error {
	// 检查是否已有Go配置（非默认配置）
	if hasValidGoConfig() {
		log.Println("Valid Go config exists, skipping migration")
		return nil
	}

	// 检查是否存在PHP配置
	if !config.HasPHPConfig() {
		log.Println("No PHP config found, skipping migration")
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

// hasValidGoConfig 检查是否存在有效的Go配置（非默认配置）
func hasValidGoConfig() bool {
	// 检查配置文件是否存在
	if _, err := os.Stat("config/config.json"); os.IsNotExist(err) {
		return false
	}

	// 加载配置并检查是否已自定义
	cfg, err := config.Load()
	if err != nil {
		return false
	}

	// 检查是否还是默认配置（域名和密码未修改）
	defaultDomain := "http://127.0.0.1:8080"
	defaultPassword := "7676aaafb027c825bd9abab78b234070e702752f625b752e55e55b48e607e358"

	// 如果域名或密码已修改，说明是有效配置
	if cfg.Domain != defaultDomain || cfg.Password != defaultPassword {
		return true
	}

	// 如果有安装锁文件，说明已完成安装
	if _, err := os.Stat("config/install.lock"); err == nil {
		return true
	}

	return false
}
