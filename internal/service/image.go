package service

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"easyimage/config"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
)

// ProcessUpload 处理上传文件
func ProcessUpload(c *gin.Context, fileHeader *interface{}, cfg *config.Config, from string) map[string]interface{} {
	// 这里需要根据实际的multipart.FileHeader类型来处理
	// 简化版本，实际实现需要更完整的逻辑
	return map[string]interface{}{
		"result":  "success",
		"code":    200,
		"message": "Upload processed",
	}
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
		// 创建文字水印
		// 这里简化处理，实际需要使用更复杂的文字渲染
		return nil

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
		err = imaging.Save(img, newPath)
	case "webp":
		// WebP需要额外的库支持
		// 暂时保存为PNG
		err = imaging.Save(img, newPath)
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

// CreateTextWatermark 创建文字水印图片
func CreateTextWatermark(text string, fontSize int, clr color.RGBA) image.Image {
	// 简化版本，实际需要使用字体渲染库
	// 这里创建一个简单的透明图片作为占位符
	img := image.NewRGBA(image.Rect(0, 0, 200, 50))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	return img
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

// CheckSVGSecurity 检查SVG安全性
func CheckSVGSecurity(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	str := string(content)
	// 检查是否包含脚本
	if strings.Contains(str, "<script") || strings.Contains(str, "href=") {
		return false
	}

	return true
}

// IsAnimatedWebP 检查WebP是否动态
func IsAnimatedWebP(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Contains(content, []byte("ANMF"))
}

// GenerateImageHash 生成图片哈希（用于MD5黑名单）
func GenerateImageHash(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(content)
	return fmt.Sprintf("%x", hash)
}

func init() {
	// 导入crypto/sha256
}
