package imageutil

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"testing"
)

func TestDecodeOrientedJPEG(t *testing.T) {
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 96, 48)), nil); err != nil {
		t.Fatal(err)
	}

	// A little-endian EXIF directory with orientation 6 (90° clockwise).
	var exif bytes.Buffer
	exif.WriteString("Exif\x00\x00II")
	for _, value := range []any{uint16(42), uint32(8), uint16(1), uint16(0x0112), uint16(3), uint32(1), uint16(6), uint16(0), uint32(0)} {
		if err := binary.Write(&exif, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	imageData := append([]byte{0xff, 0xd8, 0xff, 0xe1}, byte((exif.Len()+2)>>8), byte(exif.Len()+2))
	imageData = append(imageData, exif.Bytes()...)
	imageData = append(imageData, jpegData.Bytes()[2:]...)
	oriented, err := DecodeOrientedJPEG(bytes.NewReader(imageData))
	if err != nil {
		t.Fatal(err)
	}
	if got := oriented.Bounds().Size(); got != image.Pt(48, 96) {
		t.Fatalf("rotated image size = %v, want (48,96)", got)
	}
}
