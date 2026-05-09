package service

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"easyimage/config"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
)

// 并发限制：限制同时进行的图片后处理 goroutine 数量，避免在大量上传时耗尽 CPU/内存。
// 值为 CPU 核心数，CPU 密集型工作的最优并发度。
var processSem = make(chan struct{}, runtime.NumCPU())

// ProcessUpload 处理上传文件
func ProcessUpload(c *gin.Context, fileHeader *multipart.FileHeader, cfg *config.Config, from string) map[string]interface{} {
	// 检查文件大小
	if fileHeader.Size > cfg.MaxSize {
		return map[string]interface{}{
			"result":  "failed",
			"code":    400,
			"message": fmt.Sprintf("文件大小超过限制: %d bytes", cfg.MaxSize),
		}
	}

	// 检查文件扩展名
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(fileHeader.Filename), "."))
	if !isAllowedExtension(ext, cfg.Extensions) {
		return map[string]interface{}{
			"result":  "failed",
			"code":    400,
			"message": "不支持的文件格式: " + ext,
		}
	}

	// 生成文件名
	fileName := GenerateFileName(strings.TrimSuffix(fileHeader.Filename, filepath.Ext(fileHeader.Filename)), cfg.ImgName)
	newFileName := fileName + "." + ext

	// 生成存储路径
	now := time.Now()
	storagePath := cfg.StoragePath
	storagePath = strings.Replace(storagePath, "Y", fmt.Sprintf("%04d", now.Year()), 1)
	storagePath = strings.Replace(storagePath, "m", fmt.Sprintf("%02d", now.Month()), 1)
	storagePath = strings.Replace(storagePath, "d", fmt.Sprintf("%02d", now.Day()), 1)

	// 完整的存储目录
	uploadDir := filepath.Join(".", cfg.Path, storagePath)

	// 创建目录
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return map[string]interface{}{
			"result":  "failed",
			"code":    500,
			"message": "创建目录失败: " + err.Error(),
		}
	}

	// 完整的文件路径
	filePath := filepath.Join(uploadDir, newFileName)

	// 保存文件
	if err := c.SaveUploadedFile(fileHeader, filePath); err != nil {
		return map[string]interface{}{
			"result":  "failed",
			"code":    500,
			"message": "保存文件失败: " + err.Error(),
		}
	}

	// SVG 安全检查：防止 XSS 攻击
	if ext == "svg" {
		if !CheckSVGSecurity(filePath) {
			os.Remove(filePath)
			return map[string]interface{}{
				"result":  "failed",
				"code":    400,
				"message": "SVG文件包含不安全内容，已拒绝上传",
			}
		}
	}

	// 生成访问URL
	relativePath := cfg.Path + storagePath + newFileName
	imageURL := cfg.Domain + relativePath

	// 生成缩略图URL
	thumbURL := cfg.Domain + "/app/thumb?img=" + relativePath

	// 生成删除链接
	delURL := ""
	if cfg.ShowUserHashDel == 1 {
		delHash := EncryptHash(relativePath, cfg.Password)
		delURL = cfg.Domain + "/app/del_hash?hash=" + delHash
	}

	// 如果启用了隐藏路径
	if cfg.HidePath == 1 {
		imageURL = strings.Replace(imageURL, cfg.Path, "/", 1)
	}

	// 如果启用了源图保护
	if cfg.Hide == 1 {
		hideKey := EncryptHideKey(relativePath, cfg.HideKey)
		imageURL = cfg.Domain + "/app/hide?key=" + hideKey
	}

	// 异步处理图片后处理（压缩、水印、格式转换、WebP转换）
	go ProcessImageAfterUpload(filePath, cfg)

	// 生成WebP URL（如果配置了WebP转换）
	// WebP 文件存储在 cfg.Path + "webp/" 下，镜像原始目录结构
	webpURL := ""
	if cfg.WebpConvert == 1 {
		webpRelativePath := cfg.Path + "webp/" + storagePath + fileName + ".webp"
		webpURL = cfg.Domain + webpRelativePath
		if cfg.HidePath == 1 {
			webpURL = strings.Replace(webpURL, cfg.Path, "/", 1)
		}
	}

	return map[string]interface{}{
		"result":   "success",
		"code":     200,
		"url":      imageURL,
		"srcName":  strings.TrimSuffix(fileHeader.Filename, filepath.Ext(fileHeader.Filename)),
		"thumb":    thumbURL,
		"del":      delURL,
		"webp_url": webpURL,
	}
}

// isAllowedExtension 检查扩展名是否允许
// 使用逗号包裹的字符串搜索，避免每次调用都 Split 分配新切片。
func isAllowedExtension(ext, allowedExtensions string) bool {
	if allowedExtensions == "" {
		return false
	}
	// ",jpg,jpeg,png," 中查找 ",ext,"
	return strings.Contains(","+allowedExtensions+",", ","+ext+",")
}

// AddWatermark 添加水印
func AddWatermark(imgPath string, cfg *config.Config) error {
	if cfg.Watermark == 0 {
		return nil
	}

	// 打开图片
	img, err := imaging.Open(imgPath)
	if err != nil {
		return err
	}

	switch cfg.Watermark {
	case 1: // 文字水印
		// 创建水印图片
		watermarkImg := createTextWatermark(cfg.WaterText, cfg.TextSize, color.RGBA{255, 0, 0, 128})

		// 计算水印位置
		bounds := img.Bounds()
		wBounds := watermarkImg.Bounds()
		x, y := calculatePosition(bounds, wBounds, cfg.WaterPosition)

		// 合并图片
		rgba := imaging.Clone(img)
		draw.Draw(rgba, wBounds.Add(image.Pt(x, y)), watermarkImg, image.Point{}, draw.Over)

		// 保存
		return imaging.Save(rgba, imgPath)

	case 2: // 图片水印
		// 打开水印图片
		watermarkPath := filepath.Join(".", cfg.WaterImg)
		watermark, err := imaging.Open(watermarkPath)
		if err != nil {
			return err
		}

		// 计算水印位置
		bounds := img.Bounds()
		wBounds := watermark.Bounds()
		x, y := calculatePosition(bounds, wBounds, cfg.WaterPosition)

		// 合并图片
		rgba := imaging.Clone(img)
		draw.Draw(rgba, wBounds.Add(image.Pt(x, y)), watermark, image.Point{}, draw.Over)

		// 保存
		return imaging.Save(rgba, imgPath)
	}

	return nil
}

// createTextWatermark 创建文字水印图片
func createTextWatermark(text string, fontSize int, clr color.RGBA) image.Image {
	// 计算文字宽度（近似值）
	charWidth := fontSize * 6 / 10
	textWidth := len(text) * charWidth + 20
	textHeight := fontSize + 20

	// 创建透明背景
	img := image.NewRGBA(image.Rect(0, 0, textWidth, textHeight))

	// 绘制文字（简单实现，使用像素点绘制）
	drawText(img, text, 10, fontSize+5, clr)

	return img
}

// drawText 绘制文字（简化实现）
func drawText(img *image.RGBA, text string, x, y int, clr color.RGBA) {
	for i, ch := range text {
		drawChar(img, ch, x+i*12, y, clr)
	}
}

// drawChar 绘制单个字符（简化实现）
func drawChar(img *image.RGBA, ch rune, x, y int, clr color.RGBA) {
	for dy := 0; dy < 10; dy++ {
		for dx := 0; dx < 8; dx++ {
			img.Set(x+dx, y-dy, clr)
		}
	}
}

// calculatePosition 计算水印位置
func calculatePosition(dst, src image.Rectangle, position int) (int, int) {
	dw := dst.Dx()
	dh := dst.Dy()
	sw := src.Dx()
	sh := src.Dy()

	switch position {
	case 1: // 左上
		return 0, 0
	case 2: // 上中
		return (dw - sw) / 2, 0
	case 3: // 右上
		return dw - sw, 0
	case 4: // 左中
		return 0, (dh - sh) / 2
	case 5: // 居中
		return (dw - sw) / 2, (dh - sh) / 2
	case 6: // 右中
		return dw - sw, (dh - sh) / 2
	case 7: // 左下
		return 0, dh - sh
	case 8: // 下中
		return (dw - sw) / 2, dh - sh
	case 9: // 右下
		return dw - sw, dh - sh
	default:
		return dw - sw, dh - sh
	}
}

// CompressImage 压缩图片
func CompressImage(imgPath string, quality int) error {
	// 打开图片
	file, err := os.Open(imgPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// 解码图片
	img, _, err := image.Decode(file)
	if err != nil {
		return err
	}

	// 获取文件扩展名
	ext := strings.ToLower(filepath.Ext(imgPath))

	// 创建临时文件
	tmpPath := imgPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// 根据格式压缩
	switch ext {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(out, img, &jpeg.Options{Quality: quality})
	case ".png":
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		err = encoder.Encode(out, img)
	default:
		// 其他格式不压缩
		return nil
	}

	if err != nil {
		os.Remove(tmpPath)
		return err
	}

	// 替换原文件
	out.Close()
	return os.Rename(tmpPath, imgPath)
}

// ResizeImage 调整图片大小
func ResizeImage(imgPath string, width, height int) error {
	img, err := imaging.Open(imgPath)
	if err != nil {
		return err
	}

	// 调整大小
	resized := imaging.Resize(img, width, height, imaging.Lanczos)

	// 保存
	return imaging.Save(resized, imgPath)
}

// CropImage 裁剪图片
func CropImage(imgPath string, width, height int) error {
	img, err := imaging.Open(imgPath)
	if err != nil {
		return err
	}

	// 居中裁剪
	cropped := imaging.CropCenter(img, width, height)

	return imaging.Save(cropped, imgPath)
}

// ConvertImage 转换图片格式
func ConvertImage(imgPath, format string) (string, error) {
	// 新文件路径
	newPath := strings.TrimSuffix(imgPath, filepath.Ext(imgPath)) + "." + format

	if format == "webp" {
		// WebP 需要使用 cwebp CLI 编码（imaging 库不支持 WebP 编码）
		cmd := exec.Command("cwebp", "-q", "90", imgPath, "-o", newPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("webp convert failed: %v (output: %s)", err, string(output))
		}
		return newPath, nil
	}

	img, err := imaging.Open(imgPath)
	if err != nil {
		return "", err
	}

	// 保存为新格式
	switch format {
	case "jpg", "jpeg":
		err = imaging.Save(img, newPath, imaging.JPEGQuality(90))
	case "png":
		err = imaging.Save(img, newPath, imaging.PNGCompressionLevel(png.BestCompression))
	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}

	if err != nil {
		return "", err
	}

	return newPath, nil
}

// IsGifAnimated 检查GIF是否动态（流式搜索，不将整个文件读入内存）
func IsGifAnimated(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	// 读取文件头
	header := make([]byte, 6)
	if _, err := io.ReadFull(file, header); err != nil {
		return false
	}

	// 检查GIF魔数
	if string(header[:3]) != "GIF" {
		return false
	}

	// 流式搜索 NETSCAPE2.0 标记
	// 使用滑动窗口读取，保留前一块的尾部以处理跨块匹配
	const chunkSize = 32 * 1024 // 32KB 块
	buf := make([]byte, chunkSize)
	prev := make([]byte, 0, 12) // 保留上一块末尾最多11字节（NETSCAPE2.0长度）
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			// 拼接上一块尾部 + 当前块
			search := append(prev, buf[:n]...)
			for i := 0; i < len(search)-12; i++ {
				if search[i] == 0x21 && search[i+1] == 0xff {
					if string(search[i+2:i+13]) == "NETSCAPE2.0" {
						return true
					}
				}
			}
			// 保留末尾最多11字节作为下一块的前缀
			if len(search) > 11 {
				prev = search[len(search)-11:]
			} else {
				prev = search
			}
		}
		if readErr != nil {
			break
		}
	}

	return false
}

// IsAnimatedWebP 检查WebP是否动态（流式搜索 ANMF 标记，不将整个文件读入内存）
func IsAnimatedWebP(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	// 流式搜索 "ANMF" 字节序列
	const chunkSize = 32 * 1024
	buf := make([]byte, chunkSize)
	prev := make([]byte, 0, 3) // 保留上一块末尾最多3字节（ANMF长度-1）
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			search := append(prev, buf[:n]...)
			if bytes.Contains(search, []byte("ANMF")) {
				return true
			}
			if len(search) > 3 {
				prev = search[len(search)-3:]
			} else {
				prev = search
			}
		}
		if readErr != nil {
			break
		}
	}
	return false
}

// GenerateImageHash 生成图片哈希（流式计算，不将整个文件读入内存）
func GenerateImageHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// ProcessImageAfterUpload 上传后处理图片（通过 processSem 限制并发数）
func ProcessImageAfterUpload(filePath string, cfg *config.Config) {
	processSem <- struct{}{}        // 获取令牌，阻塞 if 已达上限
	defer func() { <-processSem }() // 归还令牌

	// 压缩
	if cfg.Compress == 1 {
		CompressImage(filePath, cfg.CompressRatio)
	}

	// 水印
	if cfg.Watermark > 0 {
		AddWatermark(filePath, cfg)
	}

	// 格式转换
	if cfg.ImgConvert != "" {
		ConvertImage(filePath, cfg.ImgConvert)
	}

	// WebP转换
	if cfg.WebpConvert == 1 {
		ConvertToWebP(filePath, cfg)
	}
}

// ConvertToWebP 将图片转换为WebP格式并存储到独立文件夹
// 使用 cwebp CLI 工具进行编码（Go 生态无纯 Go WebP 编码器，golang.org/x/image/webp 仅支持解码）。
// 如果 cwebp 不可用，静默跳过（日志提示）。
func ConvertToWebP(imgPath string, cfg *config.Config) error {
	// 跳过已经是webp的文件
	ext := strings.ToLower(filepath.Ext(imgPath))
	if ext == ".webp" {
		return nil
	}

	// 跳过动态图片（gif）
	if ext == ".gif" && IsGifAnimated(imgPath) {
		return nil
	}

	// 生成webp存储路径
	// 原始路径如: ./i/2026/05/08/xxx.jpg
	// webp路径如: ./i/webp/2026/05/08/xxx.webp
	relPath, err := filepath.Rel(".", imgPath)
	if err != nil {
		return err
	}

	webpDir := filepath.Join(".", cfg.Path, "webp")
	webpPath := filepath.Join(webpDir, strings.TrimSuffix(relPath, filepath.Ext(relPath))+".webp")

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(webpPath), 0755); err != nil {
		return err
	}

	quality := cfg.WebpQuality
	if quality == 0 {
		quality = 80
	}

	// 使用 cwebp 命令行工具转换（Go 标准库和 x/image 均无 WebP 编码器）
	// cwebp -q <quality> input -o output
	cmd := exec.Command("cwebp", "-q", fmt.Sprintf("%d", quality), imgPath, "-o", webpPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// cwebp 未安装或转换失败，记录日志但不中断流程
		fmt.Printf("[WebP] 转换失败 %s: %v (output: %s)\n", imgPath, err, string(output))
		return err
	}

	return nil
}

// GetWebPURL 获取图片的WebP版本URL
// WebP 文件存储在 cfg.Path/webp/ 下，镜像原始目录结构。
// 例如: /i/2026/05/08/xxx.jpg → /i/webp/2026/05/08/xxx.webp
func GetWebPURL(originalPath string, cfg *config.Config) string {
	ext := strings.ToLower(filepath.Ext(originalPath))
	if ext == ".webp" {
		return cfg.Domain + originalPath
	}

	// WebP 存储在 cfg.Path + "webp/" + 原始相对路径（替换扩展名）
	// originalPath 如: /i/2026/05/08/xxx.jpg
	// 期望 webpPath: /i/webp/2026/05/08/xxx.webp
	relToRoot := strings.TrimPrefix(originalPath, cfg.Path)
	webpRelPath := "webp/" + strings.TrimSuffix(relToRoot, filepath.Ext(relToRoot)) + ".webp"
	webpURLPath := cfg.Path + webpRelPath
	webpFsPath := filepath.Join(".", webpURLPath)

	// 检查webp文件是否存在
	if _, err := os.Stat(webpFsPath); err == nil {
		return cfg.Domain + webpURLPath
	}

	return ""
}

// GetAllowedExtensions 获取允许的扩展名
func GetAllowedExtensions(cfg *config.Config) []string {
	if cfg.Extensions == "" {
		return []string{"jpg", "jpeg", "png", "gif", "bmp", "webp", "ico", "jfif", "tif", "tga", "svg"}
	}
	return strings.Split(cfg.Extensions, ",")
}

// IsAllowedExtension 检查扩展名是否允许
func IsAllowedExtension(filename string, cfg *config.Config) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	allowed := GetAllowedExtensions(cfg)
	for _, a := range allowed {
		if a == ext {
			return true
		}
	}
	return false
}

// CheckSVGSecurity 检查SVG安全性，防止XSS攻击
// 直接在字节切片上搜索，避免将整个内容转为 string 产生额外拷贝。
func CheckSVGSecurity(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	lower := bytes.ToLower(content)

	// 检查已知的 XSS 向量
	dangerousPatterns := [][]byte{
		[]byte("<script"),
		[]byte("javascript:"),
		[]byte("vbscript:"),
		[]byte("data:"),
		[]byte("<iframe"),
		[]byte("<embed"),
		[]byte("<object"),
		[]byte("<foreignobject"),
		[]byte("onload"),
		[]byte("onerror"),
		[]byte("onclick"),
		[]byte("onmouseover"),
		[]byte("onfocus"),
		[]byte("onblur"),
		[]byte("onanimationend"),
		[]byte("onbegin"),
		[]byte("<use"),
		[]byte("xlink:href"),
		[]byte("<set"),
		[]byte("<animate"),
	}

	for _, pattern := range dangerousPatterns {
		if bytes.Contains(lower, pattern) {
			return false
		}
	}

	return true
}

func init() {
	// 导入crypto/sha256
}
