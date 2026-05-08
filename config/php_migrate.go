package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// PHPConfigParser PHP配置解析器
type PHPConfigParser struct {
	filePath string
}

// NewPHPConfigParser 创建PHP配置解析器
func NewPHPConfigParser(filePath string) *PHPConfigParser {
	return &PHPConfigParser{filePath: filePath}
}

// Parse 解析PHP配置文件
func (p *PHPConfigParser) Parse() (map[string]interface{}, error) {
	file, err := os.Open(p.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	config := make(map[string]interface{})
	scanner := bufio.NewScanner(file)

	// 匹配PHP数组格式: 'key'=>'value' 或 'key'=>value 或 'key'=>数字
	keyValueRe := regexp.MustCompile(`'([^']+)'\s*=>\s*(?:'([^']*)'|(\d+(?:\.\d+)?)|Array\s*\()`)
	// 匹配数组结束
	arrayEndRe := regexp.MustCompile(`^\s*\)\s*;`)
	// 匹配数组开始
	arrayStartRe := regexp.MustCompile(`'([^']+)'\s*=>\s*Array\s*\(`)

	var currentKey string
	inArray := false
	arrayConfig := make(map[string]interface{})

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过PHP标签和空行
		if line == "" || strings.HasPrefix(line, "<?php") || strings.HasPrefix(line, "$config") ||
			strings.HasPrefix(line, "$guestConfig") || strings.HasPrefix(line, "$tokenList") {
			continue
		}

		// 检查数组结束
		if arrayEndRe.MatchString(line) {
			if inArray {
				config[currentKey] = arrayConfig
				arrayConfig = make(map[string]interface{})
				inArray = false
			}
			continue
		}

		// 检查数组开始
		if matches := arrayStartRe.FindStringSubmatch(line); len(matches) > 0 {
			currentKey = matches[1]
			inArray = true
			continue
		}

		// 匹配键值对
		matches := keyValueRe.FindStringSubmatch(line)
		if len(matches) > 0 {
			key := matches[1]
			var value interface{}

			if matches[2] != "" {
				// 字符串值
				value = matches[2]
			} else if matches[3] != "" {
				// 数字值
				if strings.Contains(matches[3], ".") {
					value = matches[3] // 保持为字符串
				} else {
					value = matches[3] // 保持为字符串，后续转换
				}
			}

			if inArray {
				arrayConfig[key] = value
			} else {
				config[key] = value
			}
		}
	}

	return config, nil
}

// AutoMigrate 自动迁移PHP配置到Go配置
func AutoMigrate() error {
	// 检查是否已有Go配置
	if _, err := os.Stat("config/config.json"); err == nil {
		// 已有Go配置，跳过迁移
		return nil
	}

	// 检查是否存在PHP配置
	phpConfigFile := "config/config.php"
	if _, err := os.Stat(phpConfigFile); os.IsNotExist(err) {
		// 无PHP配置，跳过迁移
		return nil
	}

	fmt.Println("检测到PHP版本配置，开始自动迁移...")

	// 解析PHP配置
	parser := NewPHPConfigParser(phpConfigFile)
	phpConfig, err := parser.Parse()
	if err != nil {
		return fmt.Errorf("failed to parse PHP config: %w", err)
	}

	// 转换为Go配置
	goConfig := convertPHPToGoConfig(phpConfig)

	// 保存Go配置
	if err := Save(goConfig); err != nil {
		return fmt.Errorf("failed to save Go config: %w", err)
	}

	// 迁移上传者配置
	if err := migrateGuestConfig(); err != nil {
		fmt.Printf("Warning: failed to migrate guest config: %v\n", err)
	}

	// 迁移API密钥配置
	if err := migrateAPIKeys(); err != nil {
		fmt.Printf("Warning: failed to migrate API keys: %v\n", err)
	}

	// 创建安装锁文件
	os.WriteFile("config/install.lock", []byte("installed from PHP migration"), 0644)

	fmt.Println("自动迁移完成！")
	fmt.Printf("迁移的配置项: %d\n", len(phpConfig))

	return nil
}

// convertPHPToGoConfig 将PHP配置转换为Go配置
func convertPHPToGoConfig(phpConfig map[string]interface{}) *Config {
	cfg := getDefaultConfig()

	// 映射PHP配置到Go配置
	if v, ok := phpConfig["title"].(string); ok {
		cfg.Title = v
	}
	if v, ok := phpConfig["keywords"].(string); ok {
		cfg.Keywords = v
	}
	if v, ok := phpConfig["description"].(string); ok {
		cfg.Description = v
	}
	if v, ok := phpConfig["domain"].(string); ok {
		cfg.Domain = v
	}
	if v, ok := phpConfig["imgurl"].(string); ok {
		cfg.ImageURL = v
	}
	if v, ok := phpConfig["user"].(string); ok {
		cfg.User = v
	}
	if v, ok := phpConfig["password"].(string); ok {
		cfg.Password = v
	}
	if v, ok := phpConfig["path"].(string); ok {
		cfg.Path = v
	}
	if v, ok := phpConfig["storage_path"].(string); ok {
		cfg.StoragePath = v
	}
	if v, ok := phpConfig["mime"].(string); ok {
		cfg.Mime = v
	}
	if v, ok := phpConfig["imgName"].(string); ok {
		cfg.ImgName = v
	}
	if v, ok := phpConfig["maxSize"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.MaxSize = int64(n)
		}
	}
	if v, ok := phpConfig["maxUploadFiles"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.MaxUploadFiles = n
		}
	}
	if v, ok := phpConfig["extensions"].(string); ok {
		cfg.Extensions = v
	}
	if v, ok := phpConfig["compress"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.Compress = n
		}
	}
	if v, ok := phpConfig["compress_ratio"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.CompressRatio = n
		}
	}
	if v, ok := phpConfig["thumbnail"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.Thumbnail = n
		}
	}
	if v, ok := phpConfig["thumbnail_w"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.ThumbnailW = n
		}
	}
	if v, ok := phpConfig["thumbnail_h"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.ThumbnailH = n
		}
	}
	if v, ok := phpConfig["watermark"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.Watermark = n
		}
	}
	if v, ok := phpConfig["waterText"].(string); ok {
		cfg.WaterText = v
	}
	if v, ok := phpConfig["waterPosition"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.WaterPosition = n
		}
	}
	if v, ok := phpConfig["textColor"].(string); ok {
		cfg.TextColor = v
	}
	if v, ok := phpConfig["textSize"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.TextSize = n
		}
	}
	if v, ok := phpConfig["textFont"].(string); ok {
		cfg.TextFont = v
	}
	if v, ok := phpConfig["waterImg"].(string); ok {
		cfg.WaterImg = v
	}
	if v, ok := phpConfig["imgConvert"].(string); ok {
		cfg.ImgConvert = v
	}
	if v, ok := phpConfig["maxWidth"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.MaxWidth = n
		}
	}
	if v, ok := phpConfig["maxHeight"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.MaxHeight = n
		}
	}
	if v, ok := phpConfig["minWidth"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.MinWidth = n
		}
	}
	if v, ok := phpConfig["minHeight"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.MinHeight = n
		}
	}
	if v, ok := phpConfig["mustLogin"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.MustLogin = n
		}
	}
	if v, ok := phpConfig["apiStatus"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.APIStatus = n
		}
	}
	if v, ok := phpConfig["captcha"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.Captcha = n
		}
	}
	if v, ok := phpConfig["check_ip"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.CheckIP = n
		}
	}
	if v, ok := phpConfig["check_ip_model"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.CheckIPModel = n
		}
	}
	if v, ok := phpConfig["check_ip_list"].(string); ok {
		cfg.CheckIPList = v
	}
	if v, ok := phpConfig["md5_black"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.Md5Black = n
		}
	}
	if v, ok := phpConfig["md5_blacklist"].(string); ok {
		cfg.Md5Blacklist = v
	}
	if v, ok := phpConfig["upload_logs"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.UploadLogs = n
		}
	}
	if v, ok := phpConfig["hide"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.Hide = n
		}
	}
	if v, ok := phpConfig["hide_key"].(string); ok {
		cfg.HideKey = v
	}
	if v, ok := phpConfig["hide_path"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.HidePath = n
		}
	}
	if v, ok := phpConfig["admin_path_status"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.AdminPathStatus = n
		}
	}
	if v, ok := phpConfig["admin_path"].(string); ok {
		cfg.AdminPath = v
	}
	if v, ok := phpConfig["guest_path_status"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.GuestPathStatus = n
		}
	}
	if v, ok := phpConfig["token_path_status"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.TokenPathStatus = n
		}
	}
	if v, ok := phpConfig["token_suffix_ID"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.TokenSuffixID = n
		}
	}
	if v, ok := phpConfig["image_recycl"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.ImageRecycle = n
		}
	}
	if v, ok := phpConfig["show_user_hash_del"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.ShowUserHashDel = n
		}
	}
	if v, ok := phpConfig["showSwitch"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.ShowSwitch = n
		}
	}
	if v, ok := phpConfig["history"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.History = n
		}
	}
	if v, ok := phpConfig["showSort"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.ShowSort = n
		}
	}
	if v, ok := phpConfig["listNumber"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.ListNumber = n
		}
	}
	if v, ok := phpConfig["listDate"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.ListDate = n
		}
	}
	if v, ok := phpConfig["timezone"].(string); ok {
		cfg.Timezone = v
	}
	if v, ok := phpConfig["ip_upload_counts"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.IPUploadCounts = n
		}
	}
	if v, ok := phpConfig["theme"].(string); ok {
		cfg.Theme = v
	}
	if v, ok := phpConfig["dark-mode"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.DarkMode = n
		}
	}
	if v, ok := phpConfig["show_admin_inc"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.ShowAdminInc = n
		}
	}
	if v, ok := phpConfig["show_exif_info"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.ShowExifInfo = n
		}
	}
	if v, ok := phpConfig["allowed"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.Allowed = n
		}
	}
	if v, ok := phpConfig["chunks"].(string); ok {
		if n, err := parseInt(v); err == nil {
			cfg.Chunks = n
		}
	}

	// 设置端口
	cfg.Port = 8080
	cfg.Update = time.Now().Format("2006-01-02 15:04:05")

	return cfg
}

// migrateGuestConfig 迁移上传者配置
func migrateGuestConfig() error {
	phpFile := "config/config.guest.php"
	if _, err := os.Stat(phpFile); os.IsNotExist(err) {
		return nil
	}

	parser := NewPHPConfigParser(phpFile)
	phpConfig, err := parser.Parse()
	if err != nil {
		return err
	}

	guestConfig := make(map[string]*GuestConfig)
	for key, value := range phpConfig {
		if arr, ok := value.(map[string]interface{}); ok {
			gc := &GuestConfig{}
			if v, ok := arr["password"].(string); ok {
				gc.Password = v
			}
			if v, ok := arr["expired"].(string); ok {
				if n, err := parseInt(v); err == nil {
					gc.Expired = int64(n)
				}
			}
			if v, ok := arr["add_time"].(string); ok {
				if n, err := parseInt(v); err == nil {
					gc.AddTime = int64(n)
				}
			}
			guestConfig[key] = gc
		}
	}

	return SaveGuestConfig(guestConfig)
}

// migrateAPIKeys 迁移API密钥配置
func migrateAPIKeys() error {
	phpFile := "config/api_key.php"
	if _, err := os.Stat(phpFile); os.IsNotExist(err) {
		return nil
	}

	parser := NewPHPConfigParser(phpFile)
	phpConfig, err := parser.Parse()
	if err != nil {
		return err
	}

	apiKeys := make(map[string]*APIKey)
	for key, value := range phpConfig {
		if arr, ok := value.(map[string]interface{}); ok {
			ak := &APIKey{}
			if v, ok := arr["id"].(string); ok {
				if n, err := parseInt(v); err == nil {
					ak.ID = n
				}
			}
			if v, ok := arr["expired"].(string); ok {
				if n, err := parseInt(v); err == nil {
					ak.Expired = int64(n)
				}
			}
			if v, ok := arr["add_time"].(string); ok {
				if n, err := parseInt(v); err == nil {
					ak.AddTime = int64(n)
				}
			}
			apiKeys[key] = ak
		}
	}

	return SaveAPIKeys(apiKeys)
}

// parseInt 解析整数
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// HasPHPConfig 检测是否存在PHP配置
func HasPHPConfig() bool {
	phpFiles := []string{
		"config/config.php",
		"config/config.guest.php",
		"config/api_key.php",
	}

	for _, file := range phpFiles {
		if _, err := os.Stat(file); err == nil {
			return true
		}
	}

	return false
}

// MigratePHPData 迁移PHP数据目录
func MigratePHPData() error {
	// 确保图片目录存在
	dirs := []string{
		"i",
		"i/cache",
		"i/suspic",
		"i/recycle",
		"admin/logs/upload",
		"admin/logs/ipcounts",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// BackupPHPConfig 备份PHP配置文件
func BackupPHPConfig() error {
	backupDir := "config/php_backup"
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	phpFiles := []string{
		"config/config.php",
		"config/config.guest.php",
		"config/api_key.php",
	}

	for _, file := range phpFiles {
		if _, err := os.Stat(file); err == nil {
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			backupFile := filepath.Join(backupDir, filepath.Base(file))
			os.WriteFile(backupFile, data, 0644)
		}
	}

	return nil
}
