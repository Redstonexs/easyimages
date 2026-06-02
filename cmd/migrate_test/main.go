package main

import (
	"easyimage/config"
	"fmt"
	"os"
)

func main() {
	fmt.Println("========================================")
	fmt.Println("EasyImage 迁移测试工具")
	fmt.Println("========================================")

	// 检测PHP配置
	fmt.Println("\n1. 检测PHP配置文件...")
	phpFiles := []string{
		"config/config.php",
		"config/config.guest.php",
		"config/api_key.php",
	}

	phpFound := false
	for _, file := range phpFiles {
		if _, err := os.Stat(file); err == nil {
			fmt.Printf("   ✓ 找到: %s\n", file)
			phpFound = true
		} else {
			fmt.Printf("   ✗ 未找到: %s\n", file)
		}
	}

	if !phpFound {
		fmt.Println("\n未检测到PHP配置文件，无需迁移")
		return
	}
	migratable := config.HasMigratablePHPConfig()
	if migratable {
		fmt.Println("   ✓ 检测到可迁移的PHP配置")
	} else {
		fmt.Println("   ⚠ 仅检测到仓库默认PHP配置模板，不会自动迁移")
	}

	// 检测Go配置
	fmt.Println("\n2. 检测Go配置文件...")
	goFiles := []string{
		"config/config.json",
		"config/config.guest.json",
		"config/api_key.json",
	}

	goFound := false
	for _, file := range goFiles {
		if _, err := os.Stat(file); err == nil {
			fmt.Printf("   ✓ 找到: %s\n", file)
			goFound = true
		} else {
			fmt.Printf("   ✗ 未找到: %s\n", file)
		}
	}

	// 解析PHP配置
	fmt.Println("\n3. 解析PHP配置...")
	parser := config.NewPHPConfigParser("config/config.php")
	phpConfig, err := parser.Parse()
	if err != nil {
		fmt.Printf("   ✗ 解析失败: %v\n", err)
		return
	}

	fmt.Printf("   ✓ 解析成功，配置项数量: %d\n", len(phpConfig))

	// 显示关键配置
	fmt.Println("\n4. 关键配置项:")
	keyConfigs := []string{"title", "domain", "imgurl", "user", "path", "maxSize"}
	for _, key := range keyConfigs {
		if val, ok := phpConfig[key]; ok {
			fmt.Printf("   %s: %v\n", key, val)
		}
	}

	// 检查是否需要迁移
	if goFound {
		fmt.Println("\n5. 迁移状态:")
		fmt.Println("   ✓ Go配置已存在，无需迁移")
		fmt.Println("   如需重新迁移，请先删除 config/*.json 文件")
	} else if !migratable {
		fmt.Println("\n5. 迁移状态:")
		fmt.Println("   ✓ 当前PHP配置是仓库默认模板，无需迁移")
		fmt.Println("   全新安装请运行 ./easyimage 并访问 /install/")
	} else {
		fmt.Println("\n5. 迁移状态:")
		fmt.Println("   ⚠ Go配置不存在，需要迁移")
		fmt.Println("   运行 ./easyimage 将自动执行迁移")
	}

	// 显示目录结构
	fmt.Println("\n6. 目录结构检查:")
	dirs := []string{"i", "i/cache", "i/suspic", "i/recycle", "admin/logs/upload", "admin/logs/ipcounts"}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err == nil {
			fmt.Printf("   ✓ %s\n", dir)
		} else {
			fmt.Printf("   ✗ %s (将在迁移时创建)\n", dir)
		}
	}

	// 显示配置对照
	fmt.Println("\n7. 配置对照预览:")
	goConfig := config.NewPHPConfigParser("config/config.php")
	if goConfig != nil {
		// 转换为Go配置
		goConfigStruct := convertPHPToGoConfigPreview(phpConfig)
		printConfigPreview(goConfigStruct)
	}

	fmt.Println("\n========================================")
	fmt.Println("测试完成")
	fmt.Println("========================================")
}

func convertPHPToGoConfigPreview(phpConfig map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// 映射关键配置
	configMap := map[string]string{
		"title":              "title",
		"domain":             "domain",
		"imgurl":             "imgurl",
		"user":               "user",
		"password":           "password",
		"path":               "path",
		"storage_path":       "storage_path",
		"maxSize":            "maxSize",
		"maxUploadFiles":     "maxUploadFiles",
		"extensions":         "extensions",
		"compress":           "compress",
		"compress_ratio":     "compress_ratio",
		"thumbnail":          "thumbnail",
		"thumbnail_w":        "thumbnail_w",
		"thumbnail_h":        "thumbnail_h",
		"watermark":          "watermark",
		"mustLogin":          "mustLogin",
		"apiStatus":          "apiStatus",
		"check_ip":           "check_ip",
		"upload_logs":        "upload_logs",
		"hide":               "hide",
		"hide_path":          "hide_path",
		"admin_path_status":  "admin_path_status",
		"guest_path_status":  "guest_path_status",
		"image_recycl":       "image_recycl",
		"show_user_hash_del": "show_user_hash_del",
		"showSwitch":         "showSwitch",
		"history":            "history",
		"showSort":           "showSort",
		"listNumber":         "listNumber",
		"listDate":           "listDate",
		"timezone":           "timezone",
	}

	for phpKey, goKey := range configMap {
		if val, ok := phpConfig[phpKey]; ok {
			result[goKey] = val
		}
	}

	return result
}

func printConfigPreview(config map[string]interface{}) {
	// 按类别分组显示
	categories := map[string][]string{
		"站点配置": {"title", "domain", "imgurl", "path", "storage_path", "timezone"},
		"用户配置": {"user", "password", "mustLogin"},
		"上传配置": {"maxSize", "maxUploadFiles", "extensions", "compress", "compress_ratio"},
		"图片处理": {"thumbnail", "thumbnail_w", "thumbnail_h", "watermark"},
		"安全配置": {"check_ip", "upload_logs", "hide", "hide_path", "admin_path_status", "guest_path_status"},
		"显示配置": {"showSwitch", "history", "showSort", "listNumber", "listDate", "show_user_hash_del", "image_recycl"},
	}

	for category, keys := range categories {
		fmt.Printf("\n   %s:\n", category)
		for _, key := range keys {
			if val, ok := config[key]; ok {
				// 隐藏密码
				if key == "password" {
					fmt.Printf("     %s: [已隐藏]\n", key)
				} else {
					fmt.Printf("     %s: %v\n", key, val)
				}
			}
		}
	}
}
