package media_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"testing"

	"github.com/douglascamata/busylib-go/media"
)

func TestConvertForStorage(t *testing.T) {
	ctx := context.Background()
	input := pngBytes(t, image.NewRGBA(image.Rect(0, 0, 100, 50)))
	for _, ext := range []string{".jpg", ".JPEG", ".png", ".bmp", ".tif", ".tiff", ".webp"} {
		asset, err := media.ConvertForStorage(ctx, "icon"+ext, input)
		if err != nil {
			t.Fatal(err)
		}
		if asset.Name != "icon.png" || decodePNG(t, asset.Data).Bounds().Size() != image.Pt(72, 16) {
			t.Fatalf("image %s: %+v", ext, asset)
		}
		if _, err := media.ConvertForStorage(ctx, "broken"+ext, []byte("bad image")); err == nil {
			t.Fatalf("invalid image %s passed through", ext)
		}
	}
	for _, ext := range []string{".gif", ".MOV", ".mp4", ".mkv", ".avi", ".webm"} {
		if _, err := media.ConvertForStorage(ctx, "clip"+ext, input); !errors.Is(err, media.ErrUnsupported) {
			t.Fatalf("%s: %v", ext, err)
		}
	}
	for _, name := range []string{"notes.txt", "file", "audio.raw", "audio.PCM", "audio.snd"} {
		asset, err := media.ConvertForStorage(ctx, name, input)
		if err != nil || asset.Name != name || !bytes.Equal(asset.Data, input) {
			t.Fatalf("passthrough %s: %v", name, err)
		}
	}
}

func TestConvertForStorageAudio(t *testing.T) {
	requireFFmpeg(t)
	for _, ext := range []string{".mp3", ".ogg", ".aac", ".m4a", ".flac", ".WAV"} {
		asset, err := media.ConvertForStorage(context.Background(), "tone"+ext, stereoWAV())
		if err != nil {
			t.Fatal(err)
		}
		if asset.Name != "tone.wav" || len(asset.Data) != 8820 {
			t.Fatalf("audio %s: name %s, %d bytes", ext, asset.Name, len(asset.Data))
		}
	}
}

func TestConvertAudioRaw(t *testing.T) {
	// Raw input bypasses FFmpeg only when the audio converter is called directly.
	t.Setenv("PATH", t.TempDir())
	input := []byte{0, 1, 2, 3}
	for _, name := range []string{"sounds/tone.raw", "sounds/tone.PCM"} {
		asset, err := media.ConvertAudio(context.Background(), name, input)
		if err != nil || asset.Name != "sounds/tone.wav" || !bytes.Equal(asset.Data, input) {
			t.Fatalf("raw conversion %s: %v", name, err)
		}
	}
}
