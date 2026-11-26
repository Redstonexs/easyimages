package imageutil

import (
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	"golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

func Open(name string) (image.Image, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

type EncodeOptions struct {
	JPEGQuality    int
	PNGCompression png.CompressionLevel
}

func Save(img image.Image, name string, options EncodeOptions) error {
	var encode func(io.Writer) error
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		quality := options.JPEGQuality
		if quality == 0 {
			quality = 95
		}
		encode = func(w io.Writer) error {
			return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
		}
	case ".png":
		encoder := png.Encoder{CompressionLevel: options.PNGCompression}
		encode = func(w io.Writer) error { return encoder.Encode(w, img) }
	case ".gif":
		encode = func(w io.Writer) error { return gif.Encode(w, img, nil) }
	case ".tif", ".tiff":
		encode = func(w io.Writer) error {
			return tiff.Encode(w, img, &tiff.Options{Compression: tiff.Deflate, Predictor: true})
		}
	case ".bmp":
		encode = func(w io.Writer) error { return bmp.Encode(w, img) }
	default:
		return fmt.Errorf("unsupported image extension %q", filepath.Ext(name))
	}
	file, err := os.Create(name)
	if err != nil {
		return err
	}
	err = encode(file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func Clone(img image.Image) *image.NRGBA {
	dst := image.NewNRGBA(img.Bounds().Sub(img.Bounds().Min))
	draw.Draw(dst, dst.Bounds(), img, img.Bounds().Min, draw.Src)
	return dst
}

func Resize(img image.Image, width, height int) *image.RGBA {
	if width < 0 || height < 0 || (width == 0 && height == 0) || img.Bounds().Empty() {
		return image.NewRGBA(image.Rectangle{})
	}
	bounds := img.Bounds()
	if width == 0 {
		width = max(1, int(math.Round(float64(bounds.Dx())*float64(height)/float64(bounds.Dy()))))
	}
	if height == 0 {
		height = max(1, int(math.Round(float64(bounds.Dy())*float64(width)/float64(bounds.Dx()))))
	}
	// Halving the source averages nearby pixels before the final filter and
	// bounds the amount of work in CatmullRom for large downscales.
	for bounds.Dx()/2 > width || bounds.Dy()/2 > height {
		w, h := bounds.Dx(), bounds.Dy()
		if w/2 > width {
			w /= 2
		}
		if h/2 > height {
			h /= 2
		}
		if ycbcr, ok := img.(*image.YCbCr); ok && ycbcr.SubsampleRatio == image.YCbCrSubsampleRatio420 &&
			bounds.Min.X%2 == 0 && bounds.Min.Y%2 == 0 && w%2 == 0 && h%2 == 0 &&
			bounds.Dx() == w*2 && bounds.Dy() == h*2 {
			img = halfYCbCr420(ycbcr, w, h)
		} else {
			smaller := image.NewRGBA(image.Rect(0, 0, w, h))
			prefilter(smaller, img)
			img = smaller
		}
		bounds = img.Bounds()
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)
	return dst
}

// Resample aligned 4:2:0 images at each plane's native resolution before
// converting to RGB. The caller verifies exact halving and chroma alignment.
func halfYCbCr420(src *image.YCbCr, width, height int) *image.YCbCr {
	dst := image.NewYCbCr(image.Rect(0, 0, width, height), image.YCbCrSubsampleRatio420)
	halfPlane(dst.Y, src.Y, dst.YStride, src.YStride, width, height)
	halfPlane(dst.Cb, src.Cb, dst.CStride, src.CStride, width/2, height/2)
	halfPlane(dst.Cr, src.Cr, dst.CStride, src.CStride, width/2, height/2)
	return dst
}

func halfPlane(dst, src []byte, dstStride, srcStride, width, height int) {
	for y := range height {
		top := src[2*y*srcStride:][:2*width]
		bottom := src[(2*y+1)*srcStride:][:2*width]
		row := dst[y*dstStride:][:width]
		for x := range width {
			i := x * 2
			sum := uint16(top[i]) + uint16(top[i+1]) + uint16(bottom[i]) + uint16(bottom[i+1])
			row[x] = byte((sum + 2) / 4)
		}
	}
}

func prefilter(dst *image.RGBA, src image.Image) {
	bounds := dst.Bounds()
	workers := min(4, runtime.GOMAXPROCS(0), bounds.Dy())
	if workers <= 1 || bounds.Dx()*bounds.Dy() < 256*1024 {
		draw.ApproxBiLinear.Scale(dst, bounds, src, src.Bounds(), draw.Src, nil)
		return
	}
	var wg sync.WaitGroup
	for i := range workers {
		stripe := bounds
		stripe.Min.Y += bounds.Dy() * i / workers
		stripe.Max.Y = bounds.Min.Y + bounds.Dy()*(i+1)/workers
		wg.Go(func() {
			draw.ApproxBiLinear.Scale(dst.SubImage(stripe).(*image.RGBA), bounds, src, src.Bounds(), draw.Src, nil)
		})
	}
	wg.Wait()
}

func CropCenter(img image.Image, width, height int) *image.NRGBA {
	bounds := img.Bounds()
	width, height = min(width, bounds.Dx()), min(height, bounds.Dy())
	if width <= 0 || height <= 0 {
		return image.NewNRGBA(image.Rectangle{})
	}
	src := bounds.Min.Add(image.Pt((bounds.Dx()-width)/2, (bounds.Dy()-height)/2))
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), img, src, draw.Src)
	return dst
}

func CropToRatio(img image.Image, width, height int) image.Image {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if width <= 0 || height <= 0 || w <= 0 || h <= 0 {
		return img
	}
	if float64(w)*float64(height) > float64(h)*float64(width) {
		w = max(1, int(math.Round(float64(h)*float64(width)/float64(height))))
	} else {
		h = max(1, int(math.Round(float64(w)*float64(height)/float64(width))))
	}
	if w == img.Bounds().Dx() && h == img.Bounds().Dy() {
		return img
	}
	if source, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		bounds := img.Bounds()
		start := bounds.Min.Add(image.Pt((bounds.Dx()-w)/2, (bounds.Dy()-h)/2))
		return source.SubImage(image.Rectangle{Min: start, Max: start.Add(image.Pt(w, h))})
	}
	return CropCenter(img, w, h)
}
