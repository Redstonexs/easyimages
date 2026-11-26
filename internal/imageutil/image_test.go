package imageutil

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/draw"
)

func TestPrefilterMatchesSerialScale(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1400, 816))
	for y := range src.Bounds().Dy() {
		for x := range src.Bounds().Dx() {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	bounds := image.Rect(0, 0, 700, 408)
	want := image.NewRGBA(bounds)
	draw.ApproxBiLinear.Scale(want, bounds, src, src.Bounds(), draw.Src, nil)
	got := image.NewRGBA(bounds)
	prefilter(got, src)
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("parallel prefilter changed pixel values")
	}
}

func TestHalfYCbCr420HandlesCroppedSourceStride(t *testing.T) {
	parent := image.NewYCbCr(image.Rect(0, 0, 16, 16), image.YCbCrSubsampleRatio420)
	for y := range 16 {
		for x := range 16 {
			parent.Y[parent.YOffset(x, y)] = byte(4*x + 8*y)
		}
	}
	for y := range 8 {
		for x := range 8 {
			offset := parent.COffset(2*x, 2*y)
			parent.Cb[offset], parent.Cr[offset] = byte(70+4*x), byte(128+4*y)
		}
	}
	src := parent.SubImage(image.Rect(4, 4, 12, 12)).(*image.YCbCr)
	got := halfYCbCr420(src, 4, 4)
	if first := got.YCbCrAt(0, 0); first != (color.YCbCr{Y: 54, Cb: 80, Cr: 138}) {
		t.Fatalf("first reduced pixel = %+v", first)
	}
	if last := got.YCbCrAt(3, 3); last != (color.YCbCr{Y: 126, Cb: 88, Cr: 146}) {
		t.Fatalf("last reduced pixel = %+v", last)
	}
}
