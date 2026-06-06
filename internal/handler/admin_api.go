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

const (
	siteIconDir     = "config/site-icon"
	maxSiteIconSize = 512 * 1024
)

var allowedSiteIconExts = map[string]string{
	".ico": "image/x-icon",
	".png": "image/png",
	".svg": "image/svg+xml",
}

type adminConfigPayload struct {
	Title                string                      `json:"title"`
	SiteIcon             string                      `json:"site_icon"`
	Domain               string                      `json:"domain"`
	ImageURL             string                      `json:"imgurl"`
	MaxSize              int64                       `json:"maxSize"`
	Extensions           string                      `json:"extensions"`
	MustLogin            int                         `json:"mustLogin"`
	CompressRatio        int                         `json:"compress_ratio"`
	Thumbnail            int                         `json:"thumbnail"`
	ThumbnailW           int                         `json:"thumbnail_w"`
	ThumbnailH           int                         `json:"thumbnail_h"`
	WebpConvert          int                         `json:"webp_convert"`
	WebpQuality          int                         `json:"webp_quality"`
	Watermark            int                         `json:"watermark"`
	WaterText            string                      `json:"waterText"`
	WaterPosition        int                         `json:"waterPosition"`
	TextColor            string                      `json:"textColor"`
	TextSize             int                         `json:"textSize"`
	TextFont             string                      `json:"textFont"`
	WaterImg             string                      `json:"waterImg"`
	Captcha              int                         `json:"captcha"`
	CaptchaType          int                         `json:"captcha_type"`
	TurnstileSiteKey     string                      `json:"turnstile_site_key"`
	RecaptchaSiteKey     string                      `json:"recaptcha_site_key"`
	HotlinkProtect       int                         `json:"hotlink_protect"`
	HotlinkDomains       string                      `json:"hotlink_domains"`
	Mime                 string                      `json:"mime"`
	StoragePath          string                      `json:"storage_path"`
	TimeFormat           string                      `json:"time_format"`
	AutoDelete           int                         `json:"auto_delete"`
	DefaultStorageSource string                      `json:"default_storage_source"`
	StorageSources       []adminStorageSourcePayload `json:"storage_sources"`
	TurnstileSecretSet   bool                        `json:"turnstile_secret_set"`
	RecaptchaSecretSet   bool                        `json:"recaptcha_secret_set"`
}

type adminStorageSourcePayload struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Type              string `json:"type"`
	Enabled           bool   `json:"enabled"`
	PublicBaseURL     string `json:"public_base_url"`
	S3Endpoint        string `json:"s3_endpoint"`
	S3Region          string `json:"s3_region"`
	S3Bucket          string `json:"s3_bucket"`
	S3Prefix          string `json:"s3_prefix"`
	S3AccessKeyID     string `json:"s3_access_key_id"`
	S3AccessKeySecret string `json:"s3_access_key_secret,omitempty"`
	S3ForcePathStyle  bool   `json:"s3_force_path_style"`
	S3SecretSet       bool   `json:"s3_secret_set"`
}

type adminOverviewPayload struct {
	Version    string             `json:"version"`
	TotalFiles int                `json:"total_files"`
	UsedSpace  int64              `json:"used_space"`
	UsedHuman  string             `json:"used_human"`
	Config     adminConfigPayload `json:"config"`
}

type adminChartPayload struct {
	Version     string         `json:"version"`
	TotalFiles  int            `json:"total_files"`
	UsedSpace   int64          `json:"used_space"`
	UsedHuman   string         `json:"used_human"`
	DailyStats  []gin.H        `json:"daily_stats"`
	FormatStats map[string]int `json:"format_stats"`
}

type adminFileEntry struct {
	Name         string `json:"name"`
	OriginalName string `json:"original_name,omitempty"`
	Path         string `json:"path"`
	URL          string `json:"url"`
	ThumbURL     string `json:"thumb_url"`
	WebPURL      string `json:"webp_url,omitempty"`
	Ext          string `json:"ext,omitempty"`
	Size         int64  `json:"size,omitempty"`
	SizeHuman    string `json:"size_human,omitempty"`
	ModifiedAt   string `json:"modified_at,omitempty"`
}

type adminURLListData struct {
	Path       string           `json:"path"`
	Query      string           `json:"q"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	Total      int              `json:"total"`
	TotalPages int              `json:"total_pages"`
	Files      []adminFileEntry `json:"files"`
}

type adminFilerData struct {
	RootPath   string           `json:"root_path"`
	Path       string           `json:"path"`
	ParentPath string           `json:"parent_path"`
	Dirs       []string         `json:"dirs"`
	Files      []adminFileEntry `json:"files"`
}

func AdminOverviewAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats := service.CollectDirStats("." + cfg.Path)
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, adminOverviewPayload{
			Version:    config.Version,
			TotalFiles: stats.TotalFiles,
			UsedSpace:  stats.TotalSize,
			UsedHuman:  service.FormatSize(stats.TotalSize),
			Config:     adminConfigFromConfig(cfg),
		})
	}
}

func AdminConfigAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, adminConfigFromConfig(cfg))
	}
}

func AdminConfigSaveAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req adminConfigPayload
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"result": "error", "msg": "参数错误"})
			return
		}

		applyAdminConfigPayload(cfg, req)
		if err := config.Save(cfg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"result": "error", "msg": "保存失败"})
			return
		}

		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"result": "success", "msg": "保存成功", "config": adminConfigFromConfig(cfg)})
	}
}

func AdminSiteIconUploadAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := c.FormFile("icon")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"result": "error", "message": "请选择图标文件"})
			return
		}
		if file.Size <= 0 || file.Size > maxSiteIconSize {
			c.JSON(http.StatusBadRequest, gin.H{"result": "error", "message": "图标大小需小于 512KB"})
			return
		}

		ext := strings.ToLower(filepath.Ext(file.Filename))
		contentType, ok := allowedSiteIconExts[ext]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"result": "error", "message": "仅支持 ICO、PNG 或 SVG 图标"})
			return
		}

		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"result": "error", "message": "读取图标失败"})
			return
		}
		defer src.Close()

		content, err := io.ReadAll(io.LimitReader(src, maxSiteIconSize+1))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"result": "error", "message": "读取图标失败"})
			return
		}
		if len(content) == 0 || len(content) > maxSiteIconSize {
			c.JSON(http.StatusBadRequest, gin.H{"result": "error", "message": "图标大小需小于 512KB"})
			return
		}
		if ext == ".svg" && !service.IsSafeSVGContent(content) {
			c.JSON(http.StatusBadRequest, gin.H{"result": "error", "message": "SVG 图标包含不安全内容"})
			return
		}

		if err := os.MkdirAll(siteIconDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"result": "error", "message": "创建图标目录失败"})
			return
		}
		for existingExt := range allowedSiteIconExts {
			_ = os.Remove(filepath.Join(siteIconDir, "favicon"+existingExt))
		}

		if err := os.WriteFile(filepath.Join(siteIconDir, "favicon"+ext), content, 0644); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"result": "error", "message": "保存图标失败"})
			return
		}

		cfg.SiteIcon = fmt.Sprintf("/favicon.ico?v=%d", time.Now().Unix())
		if err := config.Save(cfg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"result": "error", "message": "保存配置失败"})
			return
		}

		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"result": "success", "message": "图标已更新", "site_icon": cfg.SiteIcon, "content_type": contentType})
	}
}

func SiteIcon(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, ext := range []string{".ico", ".png", ".svg"} {
			path := filepath.Join(siteIconDir, "favicon"+ext)
			if _, err := os.Stat(path); err == nil {
				c.Header("Cache-Control", "public, max-age=3600")
				c.Header("Content-Type", allowedSiteIconExts[ext])
				c.File(path)
				return
			}
		}

		c.Header("Cache-Control", "public, max-age=86400")
		c.File("./public/images/image_icon_153794.png")
	}
}

func AdminBatchWebPAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		BatchWebP(cfg)(c)
	}
}

func AdminChartAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		payload := adminChartPayloadFromConfig(cfg)
		c.Header("Cache-Control", "private, max-age=30")
		c.JSON(http.StatusOK, payload)
	}
}

func AdminHistoryAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, err := adminHistoryPayload(c, cfg)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.Header("Cache-Control", "private, max-age=15, stale-while-revalidate=120")
		c.JSON(http.StatusOK, payload)
	}
}

func AdminHistoryDeleteAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			URL  string `json:"url"`
			Mode string `json:"mode"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.URL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
			return
		}
		mode := req.Mode
		if mode == "" {
			mode = "delete"
		}

		switch mode {
		case "delete":
			if err := service.DeleteFile(req.URL); err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "删除失败", "type": "danger", "icon": "exclamation-sign"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功", "type": "success", "icon": "ok-sign"})
		case "recycle":
			if err := service.MoveToRecycle(req.URL, cfg); err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "回收失败", "type": "danger", "icon": "exclamation-sign"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "回收成功", "type": "success", "icon": "ok-sign"})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "无效操作"})
		}
	}
}

func AdminURLListAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, err := adminURLListPayload(c, cfg)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.Header("Cache-Control", "private, max-age=30, stale-while-revalidate=120")
		c.JSON(http.StatusOK, payload)
	}
}

func AdminFilerAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, err := adminFilerPayload(c, cfg)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, payload)
	}
}

func AdminDeleteAPI(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		AdminDelete(cfg)(c)
	}
}

func adminConfigFromConfig(cfg *config.Config) adminConfigPayload {
	return adminConfigPayload{
		Title:                cfg.Title,
		SiteIcon:             cfg.SiteIcon,
		Domain:               cfg.Domain,
		ImageURL:             cfg.ImageURL,
		MaxSize:              cfg.MaxSize,
		Extensions:           cfg.Extensions,
		MustLogin:            cfg.MustLogin,
		CompressRatio:        cfg.CompressRatio,
		Thumbnail:            cfg.Thumbnail,
		ThumbnailW:           cfg.ThumbnailW,
		ThumbnailH:           cfg.ThumbnailH,
		WebpConvert:          cfg.WebpConvert,
		WebpQuality:          cfg.WebpQuality,
		Watermark:            cfg.Watermark,
		WaterText:            cfg.WaterText,
		WaterPosition:        cfg.WaterPosition,
		TextColor:            cfg.TextColor,
		TextSize:             cfg.TextSize,
		TextFont:             cfg.TextFont,
		WaterImg:             cfg.WaterImg,
		Captcha:              cfg.Captcha,
		CaptchaType:          cfg.CaptchaType,
		TurnstileSiteKey:     cfg.TurnstileSiteKey,
		RecaptchaSiteKey:     cfg.RecaptchaSiteKey,
		HotlinkProtect:       cfg.HotlinkProtect,
		HotlinkDomains:       cfg.HotlinkDomains,
		Mime:                 cfg.Mime,
		StoragePath:          cfg.StoragePath,
		TimeFormat:           cfg.TimeFormat,
		AutoDelete:           cfg.AutoDelete,
		DefaultStorageSource: cfg.DefaultStorageSource,
		StorageSources:       adminStorageSourcesFromConfig(cfg),
		TurnstileSecretSet:   cfg.TurnstileSecretKey != "",
		RecaptchaSecretSet:   cfg.RecaptchaSecretKey != "",
	}
}

func applyAdminConfigPayload(cfg *config.Config, req adminConfigPayload) {
	if req.Title != "" {
		cfg.Title = req.Title
	}
	if req.SiteIcon != "" {
		cfg.SiteIcon = req.SiteIcon
	}
	if req.Domain != "" {
		cfg.Domain = req.Domain
		cfg.ImageURL = req.Domain
	}
	if req.ImageURL != "" {
		cfg.ImageURL = req.ImageURL
	}
	if req.MaxSize > 0 {
		cfg.MaxSize = req.MaxSize
	}
	if req.Extensions != "" {
		cfg.Extensions = req.Extensions
	}
	if req.CompressRatio > 0 {
		cfg.CompressRatio = req.CompressRatio
	}
	if req.ThumbnailW > 0 {
		cfg.ThumbnailW = req.ThumbnailW
	}
	if req.ThumbnailH > 0 {
		cfg.ThumbnailH = req.ThumbnailH
	}
	if req.WebpQuality > 0 {
		cfg.WebpQuality = req.WebpQuality
	}
	if req.WaterText != "" {
		cfg.WaterText = req.WaterText
	}
	if req.TextColor != "" {
		cfg.TextColor = req.TextColor
	}
	if req.TextSize > 0 {
		cfg.TextSize = req.TextSize
	}
	if req.TextFont != "" {
		cfg.TextFont = req.TextFont
	}
	if req.WaterImg != "" {
		cfg.WaterImg = req.WaterImg
	}
	if req.TurnstileSiteKey != "" {
		cfg.TurnstileSiteKey = req.TurnstileSiteKey
	}
	if req.RecaptchaSiteKey != "" {
		cfg.RecaptchaSiteKey = req.RecaptchaSiteKey
	}
	if req.Mime != "" {
		cfg.Mime = req.Mime
	}
	if req.StoragePath != "" {
		cfg.StoragePath = req.StoragePath
	}
	if req.TimeFormat != "" {
		cfg.TimeFormat = req.TimeFormat
	}
	if req.DefaultStorageSource != "" {
		cfg.DefaultStorageSource = req.DefaultStorageSource
	}
	if len(req.StorageSources) > 0 {
		cfg.StorageSources = mergeAdminStorageSources(cfg.StorageSources, req.StorageSources)
	}

	cfg.MustLogin = req.MustLogin
	cfg.Thumbnail = req.Thumbnail
	cfg.WebpConvert = req.WebpConvert
	cfg.Watermark = req.Watermark
	cfg.WaterPosition = req.WaterPosition
	cfg.Captcha = req.Captcha
	cfg.CaptchaType = req.CaptchaType
	cfg.HotlinkProtect = req.HotlinkProtect
	cfg.HotlinkDomains = req.HotlinkDomains
	cfg.AutoDelete = req.AutoDelete
}

func adminStorageSourcesFromConfig(cfg *config.Config) []adminStorageSourcePayload {
	sources := make([]adminStorageSourcePayload, 0, len(cfg.StorageSources))
	for _, source := range cfg.StorageSources {
		sources = append(sources, adminStorageSourcePayload{
			ID: source.ID, Name: source.Name, Type: source.Type, Enabled: source.Enabled,
			PublicBaseURL: source.PublicBaseURL, S3Endpoint: source.S3Endpoint, S3Region: source.S3Region,
			S3Bucket: source.S3Bucket, S3Prefix: source.S3Prefix, S3AccessKeyID: source.S3AccessKeyID,
			S3ForcePathStyle: source.S3ForcePathStyle, S3SecretSet: source.S3AccessKeySecret != "",
		})
	}
	return sources
}

func mergeAdminStorageSources(existing []config.StorageSourceConfig, payloads []adminStorageSourcePayload) []config.StorageSourceConfig {
	secretByID := make(map[string]string, len(existing))
	for _, source := range existing {
		secretByID[source.ID] = source.S3AccessKeySecret
	}
	sources := make([]config.StorageSourceConfig, 0, len(payloads))
	for _, item := range payloads {
		if item.ID == "" || item.Type == "" {
			continue
		}
		secret := item.S3AccessKeySecret
		if secret == "" {
			secret = secretByID[item.ID]
		}
		sources = append(sources, config.StorageSourceConfig{
			ID: item.ID, Name: item.Name, Type: item.Type, Enabled: item.Enabled, PublicBaseURL: item.PublicBaseURL,
			S3Endpoint: item.S3Endpoint, S3Region: item.S3Region, S3Bucket: item.S3Bucket, S3Prefix: item.S3Prefix,
			S3AccessKeyID: item.S3AccessKeyID, S3AccessKeySecret: secret, S3ForcePathStyle: item.S3ForcePathStyle,
		})
	}
	return sources
}

func adminChartPayloadFromConfig(cfg *config.Config) adminChartPayload {
	fsPath := "." + cfg.Path
	stats := service.CollectDirStats(fsPath)
	dailyStats := make([]gin.H, 0, 30)
	for i := 0; i < 30; i++ {
		date := time.Now().AddDate(0, 0, -i)
		datePath := date.Format("2006/01/02/")
		count := service.GetFileCount(fsPath + datePath + "*.*")
		dailyStats = append(dailyStats, gin.H{"date": date.Format("01-02"), "count": count})
	}
	slices.Reverse(dailyStats)

	return adminChartPayload{
		Version:     config.Version,
		TotalFiles:  stats.TotalFiles,
		UsedSpace:   stats.TotalSize,
		UsedHuman:   service.FormatSize(stats.TotalSize),
		DailyStats:  dailyStats,
		FormatStats: stats.ByExt,
	}
}

func adminHistoryPayload(c *gin.Context, cfg *config.Config) (imageListPayloadData, error) {
	return imageListPayload(c, cfg)
}

func adminURLListPayload(c *gin.Context, cfg *config.Config) (adminURLListData, error) {
	reqPath, err := adminRequestPath(c, cfg)
	if err != nil {
		return adminURLListData{}, err
	}
	page := parseBoundedInt(c.DefaultQuery("page", "1"), 1, 1, 100000)
	pageSize := parseBoundedInt(c.DefaultQuery("page_size", "50"), 50, 1, 200)
	query := strings.TrimSpace(c.Query("q"))
	if query != "" && !isSafeListQuery(query) {
		return adminURLListData{}, fmt.Errorf("invalid search parameter")
	}

	rawFiles := service.GetFileListRecursive("." + reqPath)
	allFiles := make([]adminFileEntry, 0, len(rawFiles))
	seen := make(map[string]bool, len(rawFiles))
	for _, name := range rawFiles {
		if strings.HasPrefix(name, "webp/") || !service.IsImageFile(name) {
			continue
		}
		relativePath := reqPath + name
		if query != "" {
			metadata, _ := service.GetImageMetadata(relativePath)
			if !adminFileMatchesQuery(name, relativePath, metadata, query) {
				continue
			}
		}
		seen[relativePath] = true
		allFiles = append(allFiles, adminFileEntries(reqPath, []string{name}, cfg)...)
	}
	if reqPath == cfg.Path {
		allFiles = append(allFiles, adminURLListMetadataEntries(cfg, query, seen)...)
	}

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

	return adminURLListData{
		Path:       reqPath,
		Query:      query,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
		Files:      allFiles[start:end],
	}, nil
}

func adminURLListMetadataEntries(cfg *config.Config, query string, seen map[string]bool) []adminFileEntry {
	allFiles := make([]adminFileEntry, 0)
	for i := 0; i < cfg.ListDate; i++ {
		datePath := time.Now().AddDate(0, 0, -i).Format("2006/01/02/")
		for _, metadata := range service.ListImageMetadataForDate(datePath, cfg.ShowSort == 1) {
			if metadata.StorageType != "s3" || seen[metadata.Path] {
				continue
			}
			if query != "" && !adminFileMatchesQuery(metadata.StoredName, metadata.Path, metadata, query) {
				continue
			}
			name := metadata.StoredName
			if name == "" {
				name = filepath.Base(metadata.Path)
			}
			allFiles = append(allFiles, adminFileEntry{
				Name: name, OriginalName: metadata.OriginalName, Path: metadata.Path,
				URL: imageURLForMetadata(cfg, metadata.Path, metadata), ThumbURL: thumbURLForMetadata(metadata.Path, metadata),
				Ext: strings.TrimPrefix(strings.ToUpper(filepath.Ext(name)), "."), Size: metadata.Size, SizeHuman: service.FormatSize(metadata.Size), ModifiedAt: metadata.UploadedAt,
			})
		}
	}
	return allFiles
}

func adminFileMatchesQuery(name, relativePath string, metadata service.ImageMetadata, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	return service.MetadataMatchesQuery(metadata, name, query) || strings.Contains(strings.ToLower(relativePath), query)
}

func adminFilerPayload(c *gin.Context, cfg *config.Config) (adminFilerData, error) {
	reqPath, err := adminRequestPath(c, cfg)
	if err != nil {
		return adminFilerData{}, err
	}

	dirs := service.GetDirList("." + reqPath)
	if dirs == nil {
		dirs = []string{}
	}
	files := service.GetFileList("."+reqPath+"*.*", cfg.ShowSort)
	parentPath := ""
	if reqPath != cfg.Path {
		parentPath = filepath.Dir(strings.TrimSuffix(reqPath, "/"))
		if !strings.HasSuffix(parentPath, "/") {
			parentPath += "/"
		}
		if !strings.HasPrefix(parentPath, cfg.Path) {
			parentPath = cfg.Path
		}
	}

	return adminFilerData{
		RootPath:   cfg.Path,
		Path:       reqPath,
		ParentPath: parentPath,
		Dirs:       dirs,
		Files:      adminFileEntries(reqPath, files, cfg),
	}, nil
}

func adminRequestPath(c *gin.Context, cfg *config.Config) (string, error) {
	reqPath := c.DefaultQuery("path", cfg.Path)
	reqPath, err := service.ValidateURLPath(reqPath, cfg.Path)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(reqPath, "/") {
		reqPath += "/"
	}
	return reqPath, nil
}

func adminFileEntries(basePath string, names []string, cfg *config.Config) []adminFileEntry {
	files := make([]adminFileEntry, 0, len(names))
	for _, name := range names {
		if !service.IsImageFile(name) {
			continue
		}
		relativePath := basePath + name
		metadata, _ := service.GetImageMetadata(relativePath)
		ext := strings.TrimPrefix(strings.ToUpper(filepath.Ext(name)), ".")
		var size int64
		var sizeHuman, modifiedAt string
		if info, err := os.Stat("." + relativePath); err == nil {
			size = info.Size()
			sizeHuman = service.FormatSize(size)
			modifiedAt = info.ModTime().Format("2006-01-02 15:04")
		}
		files = append(files, adminFileEntry{
			Name:         name,
			OriginalName: metadata.OriginalName,
			Path:         relativePath,
			URL:          imageURLForMetadata(cfg, relativePath, metadata),
			ThumbURL:     thumbURLForMetadata(relativePath, metadata),
			WebPURL:      service.GetWebPURL(relativePath, cfg),
			Ext:          ext,
			Size:         size,
			SizeHuman:    sizeHuman,
			ModifiedAt:   modifiedAt,
		})
	}
	return files
}

func parseBoundedInt(value string, fallback, min, max int) int {
	n, err := strconv.Atoi(value)
	if err != nil || n < min || n > max {
		return fallback
	}
	return n
}

func adminShellData(c *gin.Context, cfg *config.Config, view string) gin.H {
	return gin.H{
		"config": cfg,
		"admin": gin.H{
			"view":    view,
			"version": config.Version,
			"title":   cfg.Title,
		},
	}
}
