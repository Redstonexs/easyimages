package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	Version = "3.0.0"
)

type Config struct {
	Title               string   `json:"title"`
	Keywords            string   `json:"keywords"`
	Description         string   `json:"description"`
	Tips                string   `json:"tips"`
	NoticeStatus        int      `json:"notice_status"`
	Notice              string   `json:"notice"`
	Domain              string   `json:"domain"`
	ImageURL            string   `json:"imgurl"`
	User                string   `json:"user"`
	Password            string   `json:"password"`
	FtpStatus           int      `json:"ftp_status"`
	FtpHost             string   `json:"ftp_host"`
	FtpPort             int      `json:"ftp_port"`
	FtpUser             string   `json:"ftp_user"`
	FtpPass             string   `json:"ftp_pass"`
	FtpMode             int      `json:"ftp_mode"`
	FtpPasv             int      `json:"ftp_pasv"`
	FtpSSL              int      `json:"ftp_ssl"`
	FtpTime             int      `json:"ftp_time"`
	FtpCompleteDelLocal int      `json:"ftp_complete_del_local"`
	FtpDellocSync       int      `json:"ftp_delloc_sync"`
	Captcha             int      `json:"captcha"`
	MustLogin           int      `json:"mustLogin"`
	APIStatus           int      `json:"apiStatus"`
	Path                string   `json:"path"`
	StoragePath         string   `json:"storage_path"`
	Mime                string   `json:"mime"`
	ImgName             string   `json:"imgName"`
	MaxSize             int64    `json:"maxSize"`
	MaxUploadFiles      int      `json:"maxUploadFiles"`
	Watermark           int      `json:"watermark"`
	WaterText           string   `json:"waterText"`
	WaterPosition       int      `json:"waterPosition"`
	TextColor           string   `json:"textColor"`
	TextSize            int      `json:"textSize"`
	TextFont            string   `json:"textFont"`
	WaterImg            string   `json:"waterImg"`
	Extensions          string   `json:"extensions"`
	Compress            int      `json:"compress"`
	CompressRatio       int      `json:"compress_ratio"`
	Thumbnail           int      `json:"thumbnail"`
	ThumbnailW          int      `json:"thumbnail_w"`
	ThumbnailH          int      `json:"thumbnail_h"`
	ImgConvert          string   `json:"imgConvert"`
	MaxWidth            int      `json:"maxWidth"`
	MaxHeight           int      `json:"maxHeight"`
	MinWidth            int      `json:"minWidth"`
	MinHeight           int      `json:"minHeight"`
	ImgRatio            int      `json:"imgRatio"`
	ImageX              int      `json:"image_x"`
	ImageY              int      `json:"image_y"`
	ImgRatioQuality     int      `json:"imgRatio_quality"`
	ImgRatioCrop        int      `json:"imgRatio_crop"`
	ImgRatioPreserve    int      `json:"imgRatio_preserve_headers"`
	StaticCDN           int      `json:"static_cdn"`
	Theme               string   `json:"theme"`
	StaticCDNURL        string   `json:"static_cdn_url"`
	TinyPngKey          string   `json:"TinyPng_key"`
	CheckImg            int      `json:"checkImg"`
	CheckImgValue       int      `json:"checkImg_value"`
	ModeratecontentKey  string   `json:"moderatecontent_key"`
	NsfwjsURL           string   `json:"nsfwjs_url"`
	ShowSwitch          int      `json:"showSwitch"`
	History             int      `json:"history"`
	ShowSort            int      `json:"showSort"`
	ListNumber          int      `json:"listNumber"`
	ListDate            int      `json:"listDate"`
	Customize           string   `json:"customize"`
	CheckEnv            int      `json:"checkEnv"`
	Allowed             int      `json:"allowed"`
	UploadLogs          int      `json:"upload_logs"`
	CacheFreq           int      `json:"cache_freq"`
	UploadFirstShow     int      `json:"upload_first_show"`
	DarkMode            int      `json:"dark-mode"`
	ShowAdminInc        int      `json:"show_admin_inc"`
	ShowUserHashDel     int      `json:"show_user_hash_del"`
	ShowExifInfo        int      `json:"show_exif_info"`
	InfoRandPic         int      `json:"info_rand_pic"`
	ChartOn             int      `json:"chart_on"`
	CheckIP             int      `json:"check_ip"`
	CheckIPModel        int      `json:"check_ip_model"`
	CheckIPList         string   `json:"check_ip_list"`
	Md5Black            int      `json:"md5_black"`
	Md5Blacklist        string   `json:"md5_blacklist"`
	AutoDelete          int      `json:"auto_delete"`
	Timezone            string   `json:"timezone"`
	IPUploadCounts      int      `json:"ip_upload_counts"`
	Public              int      `json:"public"`
	PublicList          []string `json:"public_list"`
	Language            int      `json:"language"`
	LoginBg             string   `json:"login_bg"`
	Report              string   `json:"report"`
	ImageRecycle        int      `json:"image_recycl"`
	Tinyfilemanager     int      `json:"tinyfilemanager"`
	FileManage          int      `json:"file_manage"`
	DelDir              string   `json:"delDir"`
	Hide                int      `json:"hide"`
	HideKey             string   `json:"hide_key"`
	HidePath            int      `json:"hide_path"`
	AdminPathStatus     int      `json:"admin_path_status"`
	GuestPathStatus     int      `json:"guest_path_status"`
	TokenPathStatus     int      `json:"token_path_status"`
	TokenSuffixID       int      `json:"token_suffix_ID"`
	AdminPath           string   `json:"admin_path"`
	Chunks              int      `json:"chunks"`
	WebpConvert         int      `json:"webp_convert"`
	WebpQuality         int      `json:"webp_quality"`
	NProgressDefault    string   `json:"NProgress_default"`
	NProgressProgress   string   `json:"NProgress_Progress"`
	Footer              string   `json:"footer"`
	AdTop               int      `json:"ad_top"`
	AdTopInfo           string   `json:"ad_top_info"`
	AdBot               int      `json:"ad_bot"`
	AdBotInfo           string   `json:"ad_bot_info"`
	SetNotice           string   `json:"set_notice"`
	Terms               string   `json:"terms"`
	Update              string   `json:"update"`
	Port                int      `json:"port"`

	// 运行时配置
	mu sync.RWMutex `json:"-"`
}

type GuestConfig struct {
	Password string `json:"password"`
	Expired  int64  `json:"expired"`
	AddTime  int64  `json:"add_time"`
}

type APIKey struct {
	ID      int   `json:"id"`
	Expired int64 `json:"expired"`
	AddTime int64 `json:"add_time"`
}

var (
	currentConfig *Config
	guestConfig   map[string]*GuestConfig
	apiKeys       map[string]*APIKey
	configMu      sync.RWMutex
)

func Load() (*Config, error) {
	configMu.Lock()
	defer configMu.Unlock()

	// 如果已有配置，直接返回
	if currentConfig != nil {
		return currentConfig, nil
	}

	// 尝试加载JSON配置
	if _, err := os.Stat("config/config.json"); err == nil {
		data, err := os.ReadFile("config/config.json")
		if err != nil {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}
		// 设置默认值
		setDefaults(&cfg)
		currentConfig = &cfg
		return &cfg, nil
	}

	// 如果没有配置文件，返回默认配置（不自动保存）
	// 配置会在安装完成后保存
	cfg := getDefaultConfig()
	currentConfig = cfg
	return cfg, nil
}

func Save(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return os.WriteFile("config/config.json", data, 0644)
}

func Get() *Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return currentConfig
}

func LoadGuestConfig() (map[string]*GuestConfig, error) {
	configMu.Lock()
	defer configMu.Unlock()

	if guestConfig != nil {
		return guestConfig, nil
	}

	guestConfig = make(map[string]*GuestConfig)
	data, err := os.ReadFile("config/config.guest.json")
	if err != nil {
		if os.IsNotExist(err) {
			return guestConfig, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &guestConfig); err != nil {
		return nil, err
	}
	return guestConfig, nil
}

func SaveGuestConfig(gc map[string]*GuestConfig) error {
	data, err := json.MarshalIndent(gc, "", "  ")
	if err != nil {
		return err
	}
	configMu.Lock()
	guestConfig = gc
	configMu.Unlock()
	return os.WriteFile("config/config.guest.json", data, 0644)
}

func LoadAPIKeys() (map[string]*APIKey, error) {
	configMu.Lock()
	defer configMu.Unlock()

	if apiKeys != nil {
		return apiKeys, nil
	}

	apiKeys = make(map[string]*APIKey)
	data, err := os.ReadFile("config/api_key.json")
	if err != nil {
		if os.IsNotExist(err) {
			return apiKeys, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &apiKeys); err != nil {
		return nil, err
	}
	return apiKeys, nil
}

func SaveAPIKeys(ak map[string]*APIKey) error {
	data, err := json.MarshalIndent(ak, "", "  ")
	if err != nil {
		return err
	}
	configMu.Lock()
	apiKeys = ak
	configMu.Unlock()
	return os.WriteFile("config/api_key.json", data, 0644)
}

func setDefaults(cfg *Config) {
	if cfg.Path == "" {
		cfg.Path = "/i/"
	}
	if cfg.StoragePath == "" {
		cfg.StoragePath = "Y/m/d/"
	}
	if cfg.MaxSize == 0 {
		cfg.MaxSize = 10485760
	}
	if cfg.MaxUploadFiles == 0 {
		cfg.MaxUploadFiles = 30
	}
	if cfg.MaxWidth == 0 {
		cfg.MaxWidth = 10240
	}
	if cfg.MaxHeight == 0 {
		cfg.MaxHeight = 10240
	}
	if cfg.MinWidth == 0 {
		cfg.MinWidth = 5
	}
	if cfg.MinHeight == 0 {
		cfg.MinHeight = 5
	}
	if cfg.CompressRatio == 0 {
		cfg.CompressRatio = 80
	}
	if cfg.WebpQuality == 0 {
		cfg.WebpQuality = 80
	}
	if cfg.ThumbnailW == 0 {
		cfg.ThumbnailW = 258
	}
	if cfg.ThumbnailH == 0 {
		cfg.ThumbnailH = 258
	}
	if cfg.Extensions == "" {
		cfg.Extensions = "jpg,jpeg,png,gif,bmp,webp,ico,jfif,tif,tga,svg"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.Timezone == "" {
		cfg.Timezone = "Asia/Shanghai"
	}
	if cfg.ListNumber == 0 {
		cfg.ListNumber = 20
	}
	if cfg.ListDate == 0 {
		cfg.ListDate = 10
	}
	if cfg.Theme == "" {
		cfg.Theme = "default"
	}
}

func getDefaultConfig() *Config {
	cfg := &Config{
		Title:           "简单图床 - EasyImage",
		Keywords:        "简单图床,easyimage,无数据库图床",
		Description:     "简单图床EasyImage是一款支持多文件上传的无数据库图床",
		Domain:          "http://127.0.0.1:8080",
		ImageURL:        "http://127.0.0.1:8080",
		User:            "admin",
		Password:        "7676aaafb027c825bd9abab78b234070e702752f625b752e55e55b48e607e358",
		Path:            "/i/",
		StoragePath:     "Y/m/d/",
		Mime:            "image/*,video/*",
		ImgName:         "default",
		MaxSize:         10485760,
		MaxUploadFiles:  30,
		Extensions:      "jpg,jpeg,png,gif,bmp,webp,ico,jfif,tif,tga,svg",
		CompressRatio:   80,
		Thumbnail:       1,
		ThumbnailW:      258,
		ThumbnailH:      258,
		MaxWidth:        10240,
		MaxHeight:       10240,
		MinWidth:        5,
		MinHeight:       5,
		Theme:           "default",
		StaticCDNURL:    "https://fastly.jsdelivr.net/gh/icret/EasyImages2.0",
		ShowSwitch:      1,
		History:         1,
		ShowSort:        1,
		ListNumber:      20,
		ListDate:        10,
		Allowed:         1,
		DarkMode:        1,
		ShowAdminInc:    1,
		ShowUserHashDel: 1,
		ShowExifInfo:    1,
		InfoRandPic:     1,
		ChartOn:         1,
		ImageRecycle:    1,
		Timezone:        "Asia/Shanghai",
		LoginBg:         "../app/bing.php",
		HideKey:         "EasyImage2.0",
		AdminPath:       "u",
		Port:            8080,
		Update:          time.Now().Format("2006-01-02 15:04:05"),
		PublicList: []string{
			"time", "today", "yesterday", "total_space",
			"used_space", "free_space", "image_used", "file", "dir", "month",
		},
	}
	setDefaults(cfg)
	return cfg
}
