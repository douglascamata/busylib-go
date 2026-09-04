package media

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	"image/png"
	"path"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/douglascamata/busylib-go/frame"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// ImageOptions controls Python-compatible image conversion. The zero value
// selects the front display and enables both scaling and center cropping.
type ImageOptions struct {
	Display frame.Display
	NoScale bool
	NoCrop  bool
}

// ConvertImage decodes an image and returns a PNG asset. Built-in input formats
// are PNG, JPEG, BMP, TIFF, WebP and static GIF. Animated images are rejected.
// Like busylib-py, scaling runs only if either source dimension exceeds the
// display, using the larger width/height ratio and Lanczos filtering. Cropping
// runs only if both resulting dimensions are at least the display size.
// EXIF orientation and color-profile processing are not applied.
func ConvertImage(name string, data []byte, opts ImageOptions) (Asset, error) {
	src, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Asset{}, fmt.Errorf("media: decode image %q: %w", name, err)
	}
	if format == "gif" {
		animation, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return Asset{}, fmt.Errorf("media: decode GIF %q: %w", name, err)
		}
		if len(animation.Image) > 1 {
			return Asset{}, fmt.Errorf("media: animated image %q: %w", name, ErrUnsupported)
		}
	}
	if format == "png" && animatedPNG(data) {
		return Asset{}, fmt.Errorf("media: animated image %q: %w", name, ErrUnsupported)
	}
	if opts.Display != frame.Front && opts.Display != frame.Back {
		return Asset{}, fmt.Errorf("media: unknown display %d", opts.Display)
	}
	w, h := frame.Dimensions(opts.Display)
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if !opts.NoScale && (sw > w || sh > h) {
		scale := max(float64(w)/float64(sw), float64(h)/float64(sh))
		nw, nh := max(1, int(float64(sw)*scale)), max(1, int(float64(sh)*scale))
		filter := imaging.Lanczos
		// Pillow uses nearest-neighbor for palette and 1-bit images even when
		// its caller requests Lanczos.
		if _, paletted := src.(*image.Paletted); paletted {
			filter = imaging.NearestNeighbor
		}
		src = imaging.Resize(src, nw, nh, filter)
	}
	if !opts.NoCrop && w <= src.Bounds().Dx() && h <= src.Bounds().Dy() {
		left, top := (src.Bounds().Dx()-w)/2, (src.Bounds().Dy()-h)/2
		src = imaging.Crop(src, image.Rect(left, top, left+w, top+h).Add(src.Bounds().Min))
	}

	var out bytes.Buffer
	if err := png.Encode(&out, src); err != nil {
		return Asset{}, fmt.Errorf("media: encode image %q: %w", name, err)
	}
	return Asset{Name: strings.TrimSuffix(name, path.Ext(name)) + ".png", Data: out.Bytes()}, nil
}

// image/png ignores APNG animation chunks. Inspect them after PNG validation so
// an animation is not silently converted to its default still frame.
func animatedPNG(data []byte) bool {
	var frames uint32
	var frameControl bool
	for pos := 8; pos+12 <= len(data); {
		n := int(binary.BigEndian.Uint32(data[pos:]))
		if n > len(data)-pos-12 {
			return false
		}
		switch string(data[pos+4 : pos+8]) {
		case "acTL":
			if n == 8 {
				frames = binary.BigEndian.Uint32(data[pos+8:])
			}
		case "fcTL":
			frameControl = true
		case "IDAT":
			return frames > 1 || (frames == 1 && !frameControl)
		}
		pos += n + 12
	}
	return false
}
