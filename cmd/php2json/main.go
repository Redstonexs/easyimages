package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// PHP配置转Go配置工具
func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: php2json <input.php> <output.json>")
		fmt.Println("Example: php2json config/config.php config/config.json")
		os.Exit(1)
	}

	inputFile := os.Args[1]
	outputFile := os.Args[2]

	// 读取PHP配置
	config, err := parsePHPConfig(inputFile)
	if err != nil {
		fmt.Printf("Error parsing PHP config: %v\n", err)
		os.Exit(1)
	}

	// 转换为JSON
	jsonData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}

	// 写入文件
	if err := os.WriteFile(outputFile, jsonData, 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully converted %s to %s\n", inputFile, outputFile)
}

// parsePHPConfig 解析PHP配置文件
func parsePHPConfig(filename string) (map[string]interface{}, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := make(map[string]interface{})
	scanner := bufio.NewScanner(file)

	// 匹配PHP数组格式: 'key'=>'value' 或 'key'=>value
	re := regexp.MustCompile(`'([^']+)'\s*=>\s*(?:'([^']*)'|(\d+)|Array\s*\()`)
	// 匹配数组结束
	arrayEndRe := regexp.MustCompile(`^\s*\)\s*;`)

	var currentKey string
	inArray := false
	arrayConfig := make(map[string]interface{})

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过PHP标签和空行
		if line == "" || strings.HasPrefix(line, "<?php") || strings.HasPrefix(line, "$config") {
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

		// 匹配键值对
		matches := re.FindStringSubmatch(line)
		if len(matches) > 0 {
			key := matches[1]
			value := matches[2]
			if value == "" && matches[3] != "" {
				value = matches[3]
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
