// Package frame converts BUSY Bar display frames between the wire formats the
// device uses and plain RGBA bytes.
package frame

import (
	"bytes"
	"compress/zlib"
	"image"
	"io"
)

// Display identifies one of the two screens on the BUSY Bar.
type Display int

const (
	Front Display = 0
	Back  Display = 1
)

// Dimensions returns the pixel width and height of a display.
func Dimensions(d Display) (width, height int) {
	if d == Back {
		return 160, 80
	}
	return 72, 16
}

// L4ToRGBA expands 4-bit grayscale (two pixels per byte, low nibble first) to RGBA.
func L4ToRGBA(data []byte, width, height int) []byte {
	pixels := width * height
	rgba := make([]byte, pixels*4)
	idx := 0
	for _, b := range data {
		for _, gray := range [2]byte{(b & 0x0f) * 17, (b >> 4) * 17} {
			if idx >= pixels {
				return rgba
			}
			putGray(rgba, idx, gray)
			idx++
		}
	}
	return rgba
}

// L8ToRGBA expands 8-bit grayscale to RGBA.
func L8ToRGBA(data []byte, width, height int) []byte {
	pixels := width * height
	rgba := make([]byte, pixels*4)
	for i := 0; i < len(data) && i < pixels; i++ {
		putGray(rgba, i, data[i])
	}
	return rgba
}

func putGray(rgba []byte, idx int, gray byte) {
	o := idx * 4
	rgba[o], rgba[o+1], rgba[o+2], rgba[o+3] = gray, gray, gray, 255
}

// BGRToRGBA converts 24-bit pixels stored as B,G,R (the device's RGB888 order)
// to RGBA. Pixels missing from data stay transparent black.
func BGRToRGBA(data []byte, width, height int) []byte {
	pixels := width * height
	rgba := make([]byte, pixels*4)
	for i := 0; i < pixels; i++ {
		src, dst := i*3, i*4
		if src+2 >= len(data) {
			break
		}
		rgba[dst], rgba[dst+1], rgba[dst+2], rgba[dst+3] = data[src+2], data[src+1], data[src], 255
	}
	return rgba
}

// DecompressRLE expands the device's run-length encoding. Each opcode byte holds
// a block count in its low 7 bits. With the high bit set, that many literal
// blocks follow. Without it, one block follows and is repeated that many times.
func DecompressRLE(src []byte, blockSize int) []byte {
	var out []byte
	for i := 0; i < len(src); {
		opcode := src[i]
		i++
		count := int(opcode & 0x7f)
		if count == 0 {
			continue
		}
		if opcode&0x80 != 0 {
			end := min(i+count*blockSize, len(src))
			out = append(out, src[i:end]...)
			i = end
			continue
		}
		end := min(i+blockSize, len(src))
		block := src[i:end]
		i = end
		for range count {
			out = append(out, block...)
		}
	}
	return out
}

// Inflate decompresses zlib-wrapped deflate data, the format the device sends
// for the DEFLATE frame encodings.
func Inflate(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}

// ToImage wraps RGBA bytes in an image.RGBA without copying.
func ToImage(rgba []byte, width, height int) *image.RGBA {
	return &image.RGBA{Pix: rgba, Stride: width * 4, Rect: image.Rect(0, 0, width, height)}
}
