package imageutil

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"io"
)

// DecodeOrientedJPEG reads only JPEG headers for the EXIF orientation. EXIF
// offsets and entries are validated against their APP1 segment before use.
func DecodeOrientedJPEG(r io.ReadSeeker) (image.Image, error) {
	orientation := jpegOrientation(bufio.NewReader(io.LimitReader(r, 1<<20)))
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	img, err := jpeg.Decode(r)
	if err != nil || orientation <= 1 {
		return img, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	size := image.Pt(w, h)
	if orientation >= 5 {
		size = image.Pt(h, w)
	}
	dst := image.NewNRGBA(image.Rectangle{Max: size})
	switch src := img.(type) {
	case *image.YCbCr:
		for y := range size.Y {
			dstRow := dst.Pix[y*dst.Stride:]
			for x := range size.X {
				sx, sy := orientedSource(x, y, w, h, orientation)
				px, py := bounds.Min.X+sx, bounds.Min.Y+sy
				yi := src.YOffset(px, py)
				ci := src.COffset(px, py)
				r8, g8, b8 := color.YCbCrToRGB(src.Y[yi], src.Cb[ci], src.Cr[ci])
				off := x * 4
				dstRow[off] = r8
				dstRow[off+1] = g8
				dstRow[off+2] = b8
				dstRow[off+3] = 255
			}
		}
	case *image.RGBA:
		for y := range size.Y {
			dstRow := dst.Pix[y*dst.Stride:]
			for x := range size.X {
				sx, sy := orientedSource(x, y, w, h, orientation)
				so := src.PixOffset(bounds.Min.X+sx, bounds.Min.Y+sy)
				do := x * 4
				copy(dstRow[do:do+4], src.Pix[so:so+4])
			}
		}
	case *image.NRGBA:
		for y := range size.Y {
			dstRow := dst.Pix[y*dst.Stride:]
			for x := range size.X {
				sx, sy := orientedSource(x, y, w, h, orientation)
				so := src.PixOffset(bounds.Min.X+sx, bounds.Min.Y+sy)
				do := x * 4
				copy(dstRow[do:do+4], src.Pix[so:so+4])
			}
		}
	default:
		for y := range size.Y {
			for x := range size.X {
				sx, sy := orientedSource(x, y, w, h, orientation)
				dst.Set(x, y, img.At(bounds.Min.X+sx, bounds.Min.Y+sy))
			}
		}
	}
	return dst, nil
}

func orientedSource(x, y, w, h int, orientation uint16) (int, int) {
	switch orientation {
	case 2:
		return w - 1 - x, y
	case 3:
		return w - 1 - x, h - 1 - y
	case 4:
		return x, h - 1 - y
	case 5:
		return y, x
	case 6:
		return y, h - 1 - x
	case 7:
		return w - 1 - y, h - 1 - x
	case 8:
		return w - 1 - y, x
	default:
		return x, y
	}
}

func jpegOrientation(r *bufio.Reader) uint16 {
	var pair [2]byte
	if _, err := io.ReadFull(r, pair[:]); err != nil || pair != [2]byte{0xff, 0xd8} {
		return 1
	}
	for {
		if prefix, err := r.ReadByte(); err != nil || prefix != 0xff {
			return 1
		}
		marker, err := r.ReadByte()
		for err == nil && marker == 0xff {
			marker, err = r.ReadByte()
		}
		if err != nil || marker == 0xda || marker == 0xd9 {
			return 1
		}
		if _, err := io.ReadFull(r, pair[:]); err != nil {
			return 1
		}
		size := int64(binary.BigEndian.Uint16(pair[:])) - 2
		if size < 0 {
			return 1
		}
		if marker != 0xe1 {
			if _, err := io.CopyN(io.Discard, r, size); err != nil {
				return 1
			}
			continue
		}
		segment := make([]byte, size)
		if _, err := io.ReadFull(r, segment); err != nil {
			return 1
		}
		if value := exifOrientation(segment); value >= 1 && value <= 8 {
			return value
		}
	}
}

func exifOrientation(segment []byte) uint16 {
	data, ok := bytes.CutPrefix(segment, []byte("Exif\x00\x00"))
	if !ok || len(data) < 8 {
		return 0
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0
	}
	offset := order.Uint32(data[4:8])
	if order.Uint16(data[2:4]) != 42 || offset < 8 || offset > uint32(len(data)-2) {
		return 0
	}
	count := int(order.Uint16(data[offset:]))
	entries := data[offset+2:]
	if count > len(entries)/12 {
		return 0
	}
	for i := range count {
		entry := entries[i*12 : i*12+12]
		if order.Uint16(entry[:2]) == 0x0112 && order.Uint16(entry[2:4]) == 3 && order.Uint32(entry[4:8]) == 1 {
			return order.Uint16(entry[8:10])
		}
	}
	return 0
}
