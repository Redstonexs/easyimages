package handler

import (
	"easyimage/config"
	"easyimage/internal/service"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type adminConfigPayload struct {
	Title              string `json:"title"`
	Domain             string `json:"domain"`
	ImageURL           string `json:"imgurl"`
	MaxSize            int64  `json:"maxSize"`
	Extensions         string `json:"extensions"`
	MustLogin          int    `json:"mustLogin"`
	CompressRatio      int    `json:"compress_ratio"`
	Thumbnail          int    `json:"thumbnail"`
	ThumbnailW         int    `json:"thumbnail_w"`
	ThumbnailH         int    `json:"thumbnail_h"`
	WebpConvert        int    `json:"webp_convert"`
	WebpQuality        int    `json:"webp_quality"`
	Watermark          int    `json:"watermark"`
	WaterText          string `json:"waterText"`
	WaterPosition      int    `json:"waterPosition"`
	TextColor          string `json:"textColor"`
	TextSize           int    `json:"textSize"`
	TextFont           string `json:"textFont"`
	WaterImg           string `json:"waterImg"`
	Captcha            int    `json:"captcha"`
	CaptchaType        int    `json:"captcha_type"`
	TurnstileSiteKey   string `json:"turnstile_site_key"`
	RecaptchaSiteKey   string `json:"recaptcha_site_key"`
	HotlinkProtect     int    `json:"hotlink_protect"`
	HotlinkDomains     string `json:"hotlink_domains"`
	Mime               string `json:"mime"`
	StoragePath        string `json:"storage_path"`
	TimeFormat         string `json:"time_format"`
	AutoDelete         int    `json:"auto_delete"`
	TurnstileSecretSet bool   `json:"turnstile_secret_set"`
	RecaptchaSecretSet bool   `json:"recaptcha_secret_set"`
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
}

type adminURLListData struct {
	Path       string           `json:"path"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	Total      int              `json:"total"`
	TotalPages int              `json:"total_pages"`
	Files      []adminFileEntry `json:"files"`
}

type adminFilerData struct {
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
		Title:              cfg.Title,
		Domain:             cfg.Domain,
		ImageURL:           cfg.ImageURL,
		MaxSize:            cfg.MaxSize,
		Extensions:         cfg.Extensions,
		MustLogin:          cfg.MustLogin,
		CompressRatio:      cfg.CompressRatio,
		Thumbnail:          cfg.Thumbnail,
		ThumbnailW:         cfg.ThumbnailW,
		ThumbnailH:         cfg.ThumbnailH,
		WebpConvert:        cfg.WebpConvert,
		WebpQuality:        cfg.WebpQuality,
		Watermark:          cfg.Watermark,
		WaterText:          cfg.WaterText,
		WaterPosition:      cfg.WaterPosition,
		TextColor:          cfg.TextColor,
		TextSize:           cfg.TextSize,
		TextFont:           cfg.TextFont,
		WaterImg:           cfg.WaterImg,
		Captcha:            cfg.Captcha,
		CaptchaType:        cfg.CaptchaType,
		TurnstileSiteKey:   cfg.TurnstileSiteKey,
		RecaptchaSiteKey:   cfg.RecaptchaSiteKey,
		HotlinkProtect:     cfg.HotlinkProtect,
		HotlinkDomains:     cfg.HotlinkDomains,
		Mime:               cfg.Mime,
		StoragePath:        cfg.StoragePath,
		TimeFormat:         cfg.TimeFormat,
		AutoDelete:         cfg.AutoDelete,
		TurnstileSecretSet: cfg.TurnstileSecretKey != "",
		RecaptchaSecretSet: cfg.RecaptchaSecretKey != "",
	}
}

func applyAdminConfigPayload(cfg *config.Config, req adminConfigPayload) {
	if req.Title != "" {
		cfg.Title = req.Title
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

	rawFiles := service.GetFileListRecursive("." + reqPath)
	allFiles := make([]string, 0, len(rawFiles))
	for _, name := range rawFiles {
		if !strings.HasPrefix(name, "webp/") && service.IsImageFile(name) {
			allFiles = append(allFiles, name)
		}
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
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
		Files:      adminFileEntries(reqPath, allFiles[start:end], cfg),
	}, nil
}

func adminFilerPayload(c *gin.Context, cfg *config.Config) (adminFilerData, error) {
	reqPath, err := adminRequestPath(c, cfg)
	if err != nil {
		return adminFilerData{}, err
	}

	dirs := service.GetDirList("." + reqPath)
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
		files = append(files, adminFileEntry{
			Name:         name,
			OriginalName: metadata.OriginalName,
			Path:         relativePath,
			URL:          cfg.Domain + relativePath,
			ThumbURL:     "/app/thumb?img=" + relativePath,
			WebPURL:      service.GetWebPURL(relativePath, cfg),
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
