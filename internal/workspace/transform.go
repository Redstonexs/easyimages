package workspace

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"easyimage/internal/imageutil"
)

var (
	workspaceCwebpOnce sync.Once
	workspaceCwebpPath string
	workspaceCwebpErr  error
)

func lookupWorkspaceCwebp() (string, error) {
	workspaceCwebpOnce.Do(func() {
		workspaceCwebpPath, workspaceCwebpErr = exec.LookPath("cwebp")
	})
	return workspaceCwebpPath, workspaceCwebpErr
}

const (
	maxTransformDimension = 4096
	maxSourcePixels       = 20_000_000
	maxSourceBytes        = 32 << 20
)

var maxTransformWebPQuery = fmt.Sprintf("?format=webp&width=%d&height=%d", maxTransformDimension, maxTransformDimension)

// WebPConversionQuery builds a link within the renderer's input and output limits.
func WebPConversionQuery(asset Asset) string {
	// GIF conversion decodes one frame. Keep the original animation for quick copies.
	switch asset.ContentType {
	case "image/jpeg", "image/png", "image/bmp":
	default:
		return ""
	}
	if asset.SizeBytes > maxSourceBytes || asset.Width <= 0 || asset.Height <= 0 || asset.Width > maxSourcePixels/asset.Height {
		return ""
	}
	if _, err := lookupWorkspaceCwebp(); err != nil {
		return ""
	}
	if max(asset.Width, asset.Height) > maxTransformDimension {
		return maxTransformWebPQuery
	}
	return "?format=webp"
}

type TransformOptions struct {
	Width   int     `json:"width,omitempty"`
	Height  int     `json:"height,omitempty"`
	Scale   float64 `json:"scale,omitempty"`
	Fit     string  `json:"fit,omitempty"`
	Format  string  `json:"format,omitempty"`
	Quality int     `json:"quality,omitempty"`
}

func TransformFromQuery(query url.Values) (TransformOptions, bool, error) {
	var opts TransformOptions
	requested := false
	for _, key := range []string{"width", "height", "scale", "fit", "format", "quality"} {
		values, exists := query[key]
		if !exists {
			continue
		}
		requested = true
		if len(values) != 1 || values[0] == "" {
			return opts, true, ValidationError(key + " must have exactly one value")
		}
		value := values[0]
		var err error
		switch key {
		case "width", "height", "quality":
			number, parseErr := strconv.Atoi(value)
			if parseErr != nil || number <= 0 {
				return opts, true, ValidationError(key + " must be a positive integer")
			}
			switch key {
			case "width":
				opts.Width = number
			case "height":
				opts.Height = number
			case "quality":
				opts.Quality = number
			}
		case "scale":
			opts.Scale, err = strconv.ParseFloat(value, 64)
			if err != nil || opts.Scale <= 0 {
				return opts, true, ValidationError("scale must be a positive number")
			}
		case "fit":
			opts.Fit = value
		case "format":
			opts.Format = value
		}
	}
	return opts, requested, nil
}

func (opts TransformOptions) Normalize(contentType string) (TransformOptions, error) {
	if opts == (TransformOptions{}) {
		return opts, ValidationError("at least one transformation option is required")
	}
	if opts.Width < 0 || opts.Height < 0 || opts.Width > maxTransformDimension || opts.Height > maxTransformDimension {
		return opts, ValidationError("width and height must be between 1 and 4096 when present")
	}
	if math.IsNaN(opts.Scale) || (opts.Scale != 0 && (opts.Scale < 0.01 || opts.Scale > 4)) {
		return opts, ValidationError("scale must be between 0.01 and 4")
	}
	hasDimensions := opts.Width != 0 || opts.Height != 0
	if opts.Scale != 0 && hasDimensions {
		return opts, ValidationError("scale cannot be combined with width or height")
	}
	if opts.Fit != "" && !hasDimensions {
		return opts, ValidationError("fit requires width or height")
	}
	if hasDimensions && opts.Fit == "" {
		opts.Fit = "scale-down"
	}
	switch opts.Fit {
	case "", "scale-down", "contain":
	case "cover":
		if opts.Width == 0 || opts.Height == 0 {
			return opts, ValidationError("fit=cover requires both width and height")
		}
	default:
		return opts, ValidationError("fit must be scale-down, contain, or cover")
	}
	opts.Format = strings.ToLower(opts.Format)
	if opts.Format == "" {
		switch contentType {
		case "image/jpeg", "image/webp":
			opts.Format = strings.TrimPrefix(contentType, "image/")
		default:
			opts.Format = "png"
		}
	}
	if opts.Format == "jpg" {
		opts.Format = "jpeg"
	}
	switch opts.Format {
	case "jpeg", "png", "webp":
	default:
		return opts, ValidationError("format must be jpeg, png, or webp")
	}
	if opts.Quality < 0 || opts.Quality > 100 {
		return opts, ValidationError("quality must be between 1 and 100")
	}
	if opts.Format == "png" {
		opts.Quality = 0
	} else if opts.Quality == 0 {
		opts.Quality = 85
	}
	return opts, nil
}

type Rendered struct {
	File        *os.File
	SizeBytes   int64
	ContentType string
	Width       int
	Height      int
	Options     TransformOptions
	release     func()
}

func (r *Rendered) Remove() {
	_ = r.File.Close()
	_ = os.Remove(r.File.Name())
	r.release()
}

func (s *Store) Render(ctx context.Context, source *os.File, contentType string, opts TransformOptions) (*Rendered, error) {
	opts, err := opts.Normalize(contentType)
	if err != nil {
		return nil, err
	}
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp":
	default:
		return nil, ErrCannotTransform
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case s.renderers <- struct{}{}:
	default:
		return nil, ErrRenderBusy
	}
	success := false
	release := func() { <-s.renderers }
	defer func() {
		if !success {
			release()
		}
	}()

	info, err := source.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxSourceBytes {
		return nil, ErrCannotTransform
	}
	input, _, err := image.DecodeConfig(source)
	if err != nil || input.Width <= 0 || input.Height <= 0 || input.Width > maxSourcePixels/input.Height {
		return nil, ErrCannotTransform
	}
	if _, err := source.Seek(0, 0); err != nil {
		return nil, err
	}
	var img image.Image
	if contentType == "image/jpeg" {
		img, err = imageutil.DecodeOrientedJPEG(source)
	} else {
		img, _, err = image.Decode(source)
	}
	if err != nil {
		return nil, ErrCannotTransform
	}
	width, height := targetDimensions(img.Bounds(), opts)
	if width > maxTransformDimension || height > maxTransformDimension {
		return nil, ValidationError("output dimensions cannot exceed 4096 pixels per side")
	}
	if opts.Fit == "cover" {
		img = imageutil.CropToRatio(img, width, height)
	}
	if img.Bounds().Dx() != width || img.Bounds().Dy() != height {
		img = imageutil.Resize(img, width, height)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	out, err := s.stage()
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			_ = out.Close()
			_ = os.Remove(out.Name())
		}
	}()
	if err := s.encode(ctx, out, img, opts); err != nil {
		return nil, err
	}
	info, err = out.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() == 0 {
		return nil, fmt.Errorf("image encoder returned an empty result")
	}
	success = true
	return &Rendered{
		File: out, SizeBytes: info.Size(), ContentType: "image/" + opts.Format,
		Width: width, Height: height, Options: opts, release: release,
	}, nil
}

func targetDimensions(bounds image.Rectangle, opts TransformOptions) (int, int) {
	width, height := bounds.Dx(), bounds.Dy()
	if opts.Fit == "cover" {
		return opts.Width, opts.Height
	}
	scale := opts.Scale
	if scale == 0 {
		scale = 1
		if opts.Width != 0 {
			scale = float64(opts.Width) / float64(width)
		}
		if opts.Height != 0 {
			heightScale := float64(opts.Height) / float64(height)
			if opts.Width == 0 || heightScale < scale {
				scale = heightScale
			}
		}
		if opts.Fit == "scale-down" {
			scale = min(1, scale)
		}
	}
	return max(1, int(math.Round(float64(width)*scale))), max(1, int(math.Round(float64(height)*scale)))
}

func (s *Store) encode(ctx context.Context, out *os.File, img image.Image, opts TransformOptions) error {
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	switch opts.Format {
	case "png":
		return encoder.Encode(out, img)
	case "jpeg":
		if alpha, ok := img.(interface{ Opaque() bool }); !ok || !alpha.Opaque() {
			flat := image.NewRGBA(img.Bounds())
			draw.Draw(flat, flat.Bounds(), image.White, image.Point{}, draw.Src)
			draw.Draw(flat, flat.Bounds(), img, img.Bounds().Min, draw.Over)
			img = flat
		}
		return jpeg.Encode(out, img, &jpeg.Options{Quality: opts.Quality})
	case "webp":
		cwebp, err := lookupWorkspaceCwebp()
		if err != nil {
			return ErrWebPUnavailable
		}
		input, err := s.stage()
		if err != nil {
			return err
		}
		defer os.Remove(input.Name())
		err = encoder.Encode(input, img)
		if closeErr := input.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, cwebp, "-quiet", "-q", strconv.Itoa(opts.Quality), input.Name(), "-o", "-")
		cmd.Stdout = out
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("encode WebP: %w", err)
		}
		return nil
	default:
		return ValidationError("unsupported output format")
	}
}
