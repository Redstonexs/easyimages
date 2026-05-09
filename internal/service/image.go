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
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"easyimage/config"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
)

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

	// 异步处理图片后处理（压缩、水印、格式转换）
	go ProcessImageAfterUpload(filePath, cfg)

	return map[string]interface{}{
		"result":  "success",
		"code":    200,
		"url":     imageURL,
		"srcName": strings.TrimSuffix(fileHeader.Filename, filepath.Ext(fileHeader.Filename)),
		"thumb":   thumbURL,
		"del":     delURL,
	}
}

// isAllowedExtension 检查扩展名是否允许
func isAllowedExtension(ext, allowedExtensions string) bool {
	allowed := strings.Split(allowedExtensions, ",")
	for _, a := range allowed {
		if strings.TrimSpace(a) == ext {
			return true
		}
	}
	return false
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
	img, err := imaging.Open(imgPath)
	if err != nil {
		return "", err
	}

	// 新文件路径
	newPath := strings.TrimSuffix(imgPath, filepath.Ext(imgPath)) + "." + format

	// 保存为新格式
	switch format {
	case "jpg", "jpeg":
		err = imaging.Save(img, newPath, imaging.JPEGQuality(90))
	case "png":
		err = imaging.Save(img, newPath, imaging.PNGCompressionLevel(png.BestCompression))
	case "webp":
		err = imaging.Save(img, newPath, imaging.JPEGQuality(90))
	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}

	if err != nil {
		return "", err
	}

	return newPath, nil
}

// IsGifAnimated 检查GIF是否动态
func IsGifAnimated(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	// 读取文件头
	header := make([]byte, 6)
	file.Read(header)

	// 检查GIF魔数
	if string(header[:3]) != "GIF" {
		return false
	}

	// 读取更多内容检查是否有多个帧
	buf := new(bytes.Buffer)
	buf.ReadFrom(file)
	content := buf.Bytes()

	// 查找NETSCAPE2.0标记
	for i := 0; i < len(content)-11; i++ {
		if content[i] == 0x21 && content[i+1] == 0xff {
			if string(content[i+2:i+13]) == "NETSCAPE2.0" {
				return true
			}
		}
	}

	return false
}

// IsAnimatedWebP 检查WebP是否动态
func IsAnimatedWebP(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Contains(content, []byte("ANMF"))
}

// GenerateImageHash 生成图片哈希
func GenerateImageHash(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(content)
	return fmt.Sprintf("%x", hash)
}

// ProcessImageAfterUpload 上传后处理图片
func ProcessImageAfterUpload(filePath string, cfg *config.Config) {
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
func CheckSVGSecurity(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	str := strings.ToLower(string(content))

	// 检查已知的 XSS 向量
	dangerousPatterns := []string{
		"<script",      // 脚本标签
		"javascript:",   // JavaScript URI
		"vbscript:",     // VBScript URI
		"data:",         // Data URI（可嵌入任意内容）
		"<iframe",       // 内嵌框架
		"<embed",        // 嵌入对象
		"<object",       // 对象标签
		"<foreignobject",// SVG foreignObject
		"onload",        // 事件处理器
		"onerror",       // 错误事件
		"onclick",       // 点击事件
		"onmouseover",   // 鼠标悬停事件
		"onfocus",       // 焦点事件
		"onblur",        // 失焦事件
		"onanimationend",// 动画结束事件
		"onbegin",       // SVG 动画开始事件
		"<use",          // SVG use（可引用外部资源）
		"xlink:href",    // XLink 引用
		"<set",          // SVG set（可触发事件）
		"<animate",      // SVG animate
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(str, pattern) {
			return false
		}
	}

	return true
}

func init() {
	// 导入crypto/sha256
}
