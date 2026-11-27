package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	legacyPHPConfigFile = "config/config.php"
	legacyPHPAPIKeyFile = "config/api_key.php"
	legacyPHPGuestFile  = "config/config.guest.php"
)

// AutoMigrate detects legacy PHP EasyImage config files in config/, converts
// them into Go JSON configs, and removes the old PHP files once persisted.
// It returns migrated=true when at least one PHP config file was migrated.
func AutoMigrate() (bool, error) {
	if HasInstalledOrCustomizedGoConfig() {
		return false, nil
	}

	hasConfig := fileExists(legacyPHPConfigFile)
	hasAPIKeys := fileExists(legacyPHPAPIKeyFile)
	hasGuest := fileExists(legacyPHPGuestFile)
	if !hasConfig && !hasAPIKeys && !hasGuest {
		return false, nil
	}

	var migratedFiles []string

	if hasConfig {
		phpConfig, err := parsePHPConfigFile(legacyPHPConfigFile)
		if err != nil {
			return false, fmt.Errorf("parse %s: %w", legacyPHPConfigFile, err)
		}
		if len(phpConfig) == 0 {
			return false, fmt.Errorf("parse %s: no configuration entries found", legacyPHPConfigFile)
		}

		goConfig := convertPHPToGoConfig(phpConfig)
		if err := Save(goConfig); err != nil {
			return false, fmt.Errorf("save config/config.json: %w", err)
		}
		if err := writeFileAtomic("config/install.lock", []byte("installed from PHP migration"), 0644); err != nil {
			return false, fmt.Errorf("write config/install.lock: %w", err)
		}
		migratedFiles = append(migratedFiles, legacyPHPConfigFile)
	}

	if hasAPIKeys {
		phpKeys, err := parsePHPConfigFile(legacyPHPAPIKeyFile)
		if err != nil {
			return false, fmt.Errorf("parse %s: %w", legacyPHPAPIKeyFile, err)
		}
		keys := convertPHPToAPIKeys(phpKeys)
		if err := SaveAPIKeys(keys); err != nil {
			return false, fmt.Errorf("save config/api_key.json: %w", err)
		}
		migratedFiles = append(migratedFiles, legacyPHPAPIKeyFile)
	}

	if hasGuest {
		if guests, err := parsePHPConfigFile(legacyPHPGuestFile); err == nil && len(guests) > 0 {
			log.Printf("[Migrate] 检测到 %d 个旧版 PHP 上传者账号（%s）；Go 版本已改用工作区账号管理，将清理旧 PHP 文件", len(guests), legacyPHPGuestFile)
		}
		migratedFiles = append(migratedFiles, legacyPHPGuestFile)
	}

	// Only remove PHP source files after all target JSON configs are safely persisted.
	for _, path := range migratedFiles {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("[Migrate] Warning: failed to remove migrated PHP file %s: %v", path, err)
		}
	}

	return true, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func parsePHPConfigFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePHPArray(string(data))
}

// parsePHPArray parses the top-level associative array from a PHP config file
// (supporting both Array(...) / array(...) across multiple lines and [...] syntax).
func parsePHPArray(src string) (map[string]any, error) {
	p := &phpArrayParser{src: src}
	if !p.seekRootArray() {
		return nil, fmt.Errorf("no PHP array found")
	}
	val, err := p.parseArray()
	if err != nil {
		return nil, err
	}
	m, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("root PHP array is not associative")
	}
	return m, nil
}

type phpArrayParser struct {
	src string
	pos int
}

func (p *phpArrayParser) skipTrivia() {
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			p.pos++
			continue
		}
		if ch == '/' && p.pos+1 < len(p.src) {
			if p.src[p.pos+1] == '/' {
				p.pos += 2
				for p.pos < len(p.src) && p.src[p.pos] != '\n' {
					p.pos++
				}
				continue
			}
			if p.src[p.pos+1] == '*' {
				p.pos += 2
				if end := strings.Index(p.src[p.pos:], "*/"); end >= 0 {
					p.pos += end + 2
				} else {
					p.pos = len(p.src)
				}
				continue
			}
		}
		if ch == '#' {
			p.pos++
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

func (p *phpArrayParser) seekRootArray() bool {
	for p.pos < len(p.src) {
		p.skipTrivia()
		if p.pos >= len(p.src) {
			return false
		}
		if p.isArrayStart() {
			return true
		}
		ch := p.src[p.pos]
		if ch == '\'' || ch == '"' {
			if _, err := p.parseString(); err != nil {
				return false
			}
			continue
		}
		p.pos++
	}
	return false
}

func (p *phpArrayParser) isArrayStart() bool {
	if p.pos >= len(p.src) {
		return false
	}
	if p.src[p.pos] == '[' {
		return true
	}
	if len(p.src)-p.pos >= 5 && strings.EqualFold(p.src[p.pos:p.pos+5], "array") {
		// Ensure "array" is not part of a longer identifier (e.g. "myarray" or "$array").
		if p.pos > 0 && isIdentChar(p.src[p.pos-1]) {
			return false
		}
		next := p.pos + 5
		if next < len(p.src) && isIdentChar(p.src[next]) {
			return false
		}
		saved := p.pos
		p.pos = next
		p.skipTrivia()
		hasParen := p.pos < len(p.src) && p.src[p.pos] == '('
		p.pos = saved
		return hasParen
	}
	return false
}

func (p *phpArrayParser) parseArray() (any, error) {
	p.skipTrivia()
	var closeCh byte
	if p.pos < len(p.src) && p.src[p.pos] == '[' {
		closeCh = ']'
		p.pos++
	} else if len(p.src)-p.pos >= 5 && strings.EqualFold(p.src[p.pos:p.pos+5], "array") {
		p.pos += 5
		p.skipTrivia()
		if p.pos >= len(p.src) || p.src[p.pos] != '(' {
			return nil, fmt.Errorf("expected '(' after array at offset %d", p.pos)
		}
		closeCh = ')'
		p.pos++
	} else {
		return nil, fmt.Errorf("expected array at offset %d", p.pos)
	}

	assoc := make(map[string]any)
	var list []any
	isAssoc := false

	for {
		p.skipTrivia()
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("unterminated PHP array")
		}
		if p.src[p.pos] == closeCh {
			p.pos++
			if isAssoc || len(list) == 0 {
				return assoc, nil
			}
			return list, nil
		}

		first, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		p.skipTrivia()

		if p.pos+1 < len(p.src) && p.src[p.pos] == '=' && p.src[p.pos+1] == '>' {
			p.pos += 2
			val, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			if !isAssoc && len(list) > 0 {
				for i, item := range list {
					assoc[strconv.Itoa(i)] = item
				}
				list = nil
			}
			isAssoc = true
			assoc[fmt.Sprint(first)] = val
		} else if isAssoc {
			assoc[strconv.Itoa(len(assoc))] = first
		} else {
			list = append(list, first)
		}

		p.skipTrivia()
		if p.pos < len(p.src) && p.src[p.pos] == ',' {
			p.pos++
			continue
		}
		if p.pos < len(p.src) && p.src[p.pos] == closeCh {
			p.pos++
			if isAssoc || len(list) == 0 {
				return assoc, nil
			}
			return list, nil
		}
		return nil, fmt.Errorf("expected ',' or '%c' at offset %d", closeCh, p.pos)
	}
}

func (p *phpArrayParser) parseValue() (any, error) {
	p.skipTrivia()
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("unexpected EOF in PHP array")
	}
	if p.isArrayStart() {
		return p.parseArray()
	}
	ch := p.src[p.pos]
	if ch == '\'' || ch == '"' {
		return p.parseString()
	}
	start := p.pos
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == ',' || c == ')' || c == ']' || c == '=' || c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			break
		}
		p.pos++
	}
	token := p.src[start:p.pos]
	if token == "" {
		return nil, fmt.Errorf("unexpected character %q at offset %d", ch, p.pos)
	}
	switch strings.ToLower(token) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	default:
		return token, nil
	}
}

func (p *phpArrayParser) parseString() (string, error) {
	quote := p.src[p.pos]
	p.pos++
	var b strings.Builder
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == '\\' && p.pos+1 < len(p.src) {
			next := p.src[p.pos+1]
			if quote == '\'' {
				if next == '\'' || next == '\\' {
					b.WriteByte(next)
					p.pos += 2
					continue
				}
			} else {
				switch next {
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				case '"', '\\', '$':
					b.WriteByte(next)
				default:
					b.WriteByte('\\')
					b.WriteByte(next)
				}
				p.pos += 2
				continue
			}
		}
		if ch == quote {
			p.pos++
			return b.String(), nil
		}
		b.WriteByte(ch)
		p.pos++
	}
	return "", fmt.Errorf("unterminated PHP string")
}

func isIdentChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '_' || ch == '$'
}

func phpString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func phpInt(m map[string]any, key string) (int, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(n))
		return parsed, err == nil
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

func phpInt64(m map[string]any, key string) (int64, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	if s, ok := v.(string); ok {
		parsed, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		return parsed, err == nil
	}
	return 0, false
}

func setStringField(m map[string]any, key string, dst *string) {
	if v, ok := phpString(m, key); ok {
		*dst = v
	}
}

func setIntField(m map[string]any, key string, dst *int) {
	if v, ok := phpInt(m, key); ok {
		*dst = v
	}
}

func convertPHPToGoConfig(phpConfig map[string]any) *Config {
	cfg := getDefaultConfig()

	setStringField(phpConfig, "title", &cfg.Title)
	setStringField(phpConfig, "keywords", &cfg.Keywords)
	setStringField(phpConfig, "description", &cfg.Description)
	setStringField(phpConfig, "domain", &cfg.Domain)
	setStringField(phpConfig, "imgurl", &cfg.ImageURL)
	setStringField(phpConfig, "user", &cfg.User)
	setStringField(phpConfig, "password", &cfg.Password)
	setStringField(phpConfig, "path", &cfg.Path)
	setStringField(phpConfig, "storage_path", &cfg.StoragePath)
	setStringField(phpConfig, "mime", &cfg.Mime)
	setStringField(phpConfig, "imgName", &cfg.ImgName)
	setStringField(phpConfig, "extensions", &cfg.Extensions)
	setStringField(phpConfig, "waterText", &cfg.WaterText)
	setStringField(phpConfig, "textColor", &cfg.TextColor)
	setStringField(phpConfig, "textFont", &cfg.TextFont)
	setStringField(phpConfig, "waterImg", &cfg.WaterImg)
	setStringField(phpConfig, "imgConvert", &cfg.ImgConvert)
	setStringField(phpConfig, "check_ip_list", &cfg.CheckIPList)
	setStringField(phpConfig, "md5_blacklist", &cfg.Md5Blacklist)
	setStringField(phpConfig, "hide_key", &cfg.HideKey)
	setStringField(phpConfig, "admin_path", &cfg.AdminPath)
	setStringField(phpConfig, "timezone", &cfg.Timezone)
	setStringField(phpConfig, "theme", &cfg.Theme)

	if v, ok := phpInt64(phpConfig, "maxSize"); ok {
		cfg.MaxSize = v
	}
	setIntField(phpConfig, "maxUploadFiles", &cfg.MaxUploadFiles)
	setIntField(phpConfig, "compress", &cfg.Compress)
	setIntField(phpConfig, "compress_ratio", &cfg.CompressRatio)
	setIntField(phpConfig, "thumbnail", &cfg.Thumbnail)
	setIntField(phpConfig, "thumbnail_w", &cfg.ThumbnailW)
	setIntField(phpConfig, "thumbnail_h", &cfg.ThumbnailH)
	setIntField(phpConfig, "watermark", &cfg.Watermark)
	setIntField(phpConfig, "waterPosition", &cfg.WaterPosition)
	setIntField(phpConfig, "textSize", &cfg.TextSize)
	setIntField(phpConfig, "maxWidth", &cfg.MaxWidth)
	setIntField(phpConfig, "maxHeight", &cfg.MaxHeight)
	setIntField(phpConfig, "minWidth", &cfg.MinWidth)
	setIntField(phpConfig, "minHeight", &cfg.MinHeight)
	setIntField(phpConfig, "mustLogin", &cfg.MustLogin)
	setIntField(phpConfig, "apiStatus", &cfg.APIStatus)
	setIntField(phpConfig, "check_ip", &cfg.CheckIP)
	setIntField(phpConfig, "check_ip_model", &cfg.CheckIPModel)
	setIntField(phpConfig, "md5_black", &cfg.Md5Black)
	setIntField(phpConfig, "upload_logs", &cfg.UploadLogs)
	setIntField(phpConfig, "hide", &cfg.Hide)
	setIntField(phpConfig, "hide_path", &cfg.HidePath)
	setIntField(phpConfig, "admin_path_status", &cfg.AdminPathStatus)
	setIntField(phpConfig, "guest_path_status", &cfg.GuestPathStatus)
	setIntField(phpConfig, "token_path_status", &cfg.TokenPathStatus)
	setIntField(phpConfig, "token_suffix_ID", &cfg.TokenSuffixID)
	setIntField(phpConfig, "image_recycl", &cfg.ImageRecycle)
	setIntField(phpConfig, "show_user_hash_del", &cfg.ShowUserHashDel)
	setIntField(phpConfig, "showSwitch", &cfg.ShowSwitch)
	setIntField(phpConfig, "history", &cfg.History)
	setIntField(phpConfig, "showSort", &cfg.ShowSort)
	setIntField(phpConfig, "listNumber", &cfg.ListNumber)
	setIntField(phpConfig, "listDate", &cfg.ListDate)
	setIntField(phpConfig, "auto_delete", &cfg.AutoDelete)
	setIntField(phpConfig, "ip_upload_counts", &cfg.IPUploadCounts)
	setIntField(phpConfig, "dark-mode", &cfg.DarkMode)
	setIntField(phpConfig, "show_admin_inc", &cfg.ShowAdminInc)
	setIntField(phpConfig, "show_exif_info", &cfg.ShowExifInfo)
	setIntField(phpConfig, "allowed", &cfg.Allowed)
	setIntField(phpConfig, "chunks", &cfg.Chunks)

	// PHP 版本的验证码机制与 Go 版本不同，迁移时统一关闭，用户可在后台重新启用。
	if _, ok := phpConfig["captcha"]; ok {
		cfg.Captcha = 0
	}

	cfg.Port = 8080
	cfg.Update = time.Now().Format("2006-01-02 15:04:05")
	setDefaults(cfg)
	return cfg
}

func convertPHPToAPIKeys(phpKeys map[string]any) map[string]*APIKey {
	result := make(map[string]*APIKey, len(phpKeys))
	for token, raw := range phpKeys {
		arr, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		ak := &APIKey{}
		setIntField(arr, "id", &ak.ID)
		if v, ok := phpInt64(arr, "expired"); ok {
			ak.Expired = v
		}
		if v, ok := phpInt64(arr, "add_time"); ok {
			ak.AddTime = v
		}
		result[token] = ak
	}
	return result
}
