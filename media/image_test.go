package media_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"testing"

	"github.com/douglascamata/busylib-go/frame"
	"github.com/douglascamata/busylib-go/media"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

func TestConvertImageDimensions(t *testing.T) {
	data, err := os.ReadFile("testdata/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name                   string
		Width, Height, Display int
		NoScale, NoCrop        bool
		WantWidth, WantHeight  int
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			src := image.NewNRGBA(image.Rect(0, 0, tc.Width, tc.Height))
			opts := media.ImageOptions{Display: frame.Display(tc.Display), NoScale: tc.NoScale, NoCrop: tc.NoCrop}
			asset, err := media.ConvertImage("icons/photo.PNG", pngBytes(t, src), opts)
			if err != nil {
				t.Fatal(err)
			}
			if asset.Name != "icons/photo.png" {
				t.Fatalf("name = %q", asset.Name)
			}
			got := decodePNG(t, asset.Data)
			want := image.Pt(tc.WantWidth, tc.WantHeight)
			if got.Bounds().Size() != want {
				t.Fatalf("size = %v, want Python size %v", got.Bounds().Size(), want)
			}
		})
	}
}

func TestConvertImageCenterCrop(t *testing.T) {
	// The center is green. Cropping from either edge would leave red or blue.
	for _, horizontal := range []bool{false, true} {
		w, h := 72, 48
		if horizontal {
			w, h = 216, 16
		}
		src := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := range h {
			for x := range w {
				band := y / 16
				if horizontal {
					band = x / 72
				}
				src.SetNRGBA(x, y, []color.NRGBA{{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}}[band])
			}
		}
		asset, err := media.ConvertImage("bands.png", pngBytes(t, src), media.ImageOptions{})
		if err != nil {
			t.Fatal(err)
		}
		got := decodePNG(t, asset.Data)
		for y := range 16 {
			for x := range 72 {
				if pixel := color.NRGBAModel.Convert(got.At(x, y)); pixel != (color.NRGBA{G: 255, A: 255}) {
					t.Fatalf("horizontal=%v pixel %d,%d = %v", horizontal, x, y, pixel)
				}
			}
		}
	}
}

func TestConvertImagePreservesTransparency(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 144, 32))
	want := color.NRGBA{R: 200, G: 80, B: 40, A: 128}
	for y := range 32 {
		for x := range 144 {
			src.SetNRGBA(x, y, want)
		}
	}
	for _, opts := range []media.ImageOptions{{}, {NoScale: true}, {NoCrop: true}, {NoScale: true, NoCrop: true}} {
		asset, err := media.ConvertImage("alpha", pngBytes(t, src), opts)
		if err != nil {
			t.Fatal(err)
		}
		got := color.NRGBAModel.Convert(decodePNG(t, asset.Data).At(20, 10)).(color.NRGBA)
		// Scaling can round unpremultiplied color values by one.
		if abs(int(got.R)-int(want.R)) > 1 || abs(int(got.G)-int(want.G)) > 1 || abs(int(got.B)-int(want.B)) > 1 || got.A != want.A {
			t.Fatalf("options %+v: color = %v, want %v", opts, got, want)
		}
	}
}

func TestConvertImageDecoders(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 10, 5))
	for y := range 5 {
		for x := range 10 {
			src.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	for _, format := range []string{"jpeg", "bmp", "tiff"} {
		t.Run(format, func(t *testing.T) {
			var input bytes.Buffer
			var err error
			switch format {
			case "jpeg":
				err = jpeg.Encode(&input, src, nil)
			case "bmp":
				err = bmp.Encode(&input, src)
			case "tiff":
				err = tiff.Encode(&input, src, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			// Decode the content, regardless of the supplied extension.
			asset, err := media.ConvertImage("icon.bin", input.Bytes(), media.ImageOptions{})
			if err != nil {
				t.Fatal(err)
			}
			got := decodePNG(t, asset.Data)
			if got.Bounds().Size() != src.Bounds().Size() {
				t.Fatal(got.Bounds())
			}
			r, g, b, a := got.At(5, 2).RGBA()
			if r < 65000 || g > 256 || b > 256 || a != 65535 {
				t.Fatalf("pixel = %d,%d,%d,%d", r, g, b, a)
			}
		})
	}
}

func TestConvertImageErrors(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("invalid")} {
		if _, err := media.ConvertImage("bad.png", data, media.ImageOptions{}); err == nil {
			t.Fatal("expected decode error")
		}
	}
	valid := pngBytes(t, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	if _, err := media.ConvertImage("bad.png", valid, media.ImageOptions{Display: 99}); err == nil {
		t.Fatal("expected display error")
	}
}

func TestConvertImageAnimations(t *testing.T) {
	for _, name := range []string{"animated.png", "animated.webp"} {
		data, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := media.ConvertImage(name, data, media.ImageOptions{}); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	var data bytes.Buffer
	if err := gif.EncodeAll(&data, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{1, 1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := media.ConvertImage("animated.gif", data.Bytes(), media.ImageOptions{}); !errors.Is(err, media.ErrUnsupported) {
		t.Fatalf("GIF error = %v", err)
	}
	data.Reset()
	if err := gif.Encode(&data, frame, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := media.ConvertImage("still.gif", data.Bytes(), media.ImageOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestConvertImagePythonPixels(t *testing.T) {
	data, err := os.ReadFile("testdata/lanczos-input.png")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := os.ReadFile("testdata/lanczos-output.png")
	if err != nil {
		t.Fatal(err)
	}
	asset, err := media.ConvertImage("image.png", data, media.ImageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, want := decodePNG(t, asset.Data), decodePNG(t, reference)
	var maxDelta, totalDelta int
	for y := range 16 {
		for x := range 72 {
			a := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA)
			b := color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
			delta := max(abs(int(a.R)-int(b.R)), abs(int(a.G)-int(b.G)), abs(int(a.B)-int(b.B)), abs(int(a.A)-int(b.A)))

			maxDelta = max(maxDelta, delta)
			totalDelta += delta
		}
	}
	// Alpha premultiplication and Lanczos ringing differ between Go and Pillow
	// at the sharp alpha edge. Keep the average within two channel levels.
	if maxDelta > 8 || float64(totalDelta)/(72*16) > 2 {
		t.Fatalf("maximum channel difference from Pillow = %d", maxDelta)
	}
}

func TestConvertImageWebP(t *testing.T) {
	data, err := os.ReadFile("testdata/still.webp")
	if err != nil {
		t.Fatal(err)
	}
	asset, err := media.ConvertImage("still.webp", data, media.ImageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, asset.Data)
	if img.Bounds().Size() != image.Pt(8, 8) {
		t.Fatal(img.Bounds())
	}
	if got := color.NRGBAModel.Convert(img.At(4, 4)); got != (color.NRGBA{R: 255, A: 255}) {
		t.Fatal(got)
	}
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
