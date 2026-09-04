package media_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/douglascamata/busylib-go/media"
)

func TestConvertAudioPCM(t *testing.T) {
	requireFFmpeg(t)
	// A 100 ms, 440 Hz tone at 48 kHz, stereo. Only the left channel has sound.
	asset, err := media.ConvertAudio(context.Background(), "sounds/tone.WAV", stereoWAV())
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != "sounds/tone.wav" {
		t.Fatalf("name = %q", asset.Name)
	}
	if len(asset.Data) != 4410*2 {
		t.Fatalf("got %d bytes, want 8820", len(asset.Data))
	}
	// Read the actual output as signed little-endian samples. Verify rate,
	// channel mixing, amplitude and header removal against the known waveform.
	var squareError float64
	for i := 100; i < 4300; i++ {
		got := float64(int16(binary.LittleEndian.Uint16(asset.Data[i*2:])))
		want := 6000 * math.Sin(2*math.Pi*440*float64(i)/44100)
		squareError += (got - want) * (got - want)
	}
	if rms := math.Sqrt(squareError / 4200); rms > 20 {
		t.Fatalf("waveform RMS error = %.1f", rms)
	}
}

func TestConvertAudioFormats(t *testing.T) {
	requireFFmpeg(t)
	src := filepath.Join(t.TempDir(), "tone.wav")
	if err := os.WriteFile(src, stereoWAV(), 0600); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"mp3", "ogg", "aac", "m4a", "flac"} {
		t.Run(format, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "tone."+format)
			// In particular, default M4A output has its metadata after the audio,
			// exercising the converter's seekable input file.
			if out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-i", src, file).CombinedOutput(); err != nil {
				t.Fatalf("make fixture: %v: %s", err, out)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			asset, err := media.ConvertAudio(context.Background(), "tone."+format, data)
			if err != nil {
				t.Fatal(err)
			}
			// Lossy codecs can add frame padding. Output must still be mono PCM
			// near 100 ms, with both positive and negative samples.
			if len(asset.Data) < 8000 || len(asset.Data) > 15000 || len(asset.Data)%2 != 0 || bytes.HasPrefix(asset.Data, []byte("RIFF")) {
				t.Fatalf("unexpected PCM length/header: %d", len(asset.Data))
			}
			var positive, negative bool
			for i := 0; i < len(asset.Data); i += 2 {
				v := int16(binary.LittleEndian.Uint16(asset.Data[i:]))
				positive = positive || v > 1000
				negative = negative || v < -1000
			}
			if !positive || !negative {
				t.Fatal("tone missing from output")
			}
		})
	}
}

func TestConvertAudioInvalidInput(t *testing.T) {
	requireFFmpeg(t)
	_, err := media.ConvertAudio(context.Background(), "bad.mp3", []byte("not audio"))
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || !strings.Contains(err.Error(), "bad.mp3") || !strings.Contains(err.Error(), "Invalid data") {
		t.Fatalf("error = %v", err)
	}
}

func TestConvertAudioMissingFFmpeg(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := media.ConvertAudio(context.Background(), "tone.wav", stereoWAV())
	if !errors.Is(err, exec.ErrNotFound) || !strings.Contains(err.Error(), "requires ffmpeg installed on PATH") {
		t.Fatalf("error = %v", err)
	}
}

func TestConvertAudioCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := media.ConvertAudio(ctx, "tone.wav", stereoWAV())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("install ffmpeg to run audio conversion tests")
	}
}

func stereoWAV() []byte {
	const frames = 4800
	data := make([]byte, 44+frames*4)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1) // PCM
	binary.LittleEndian.PutUint16(data[22:], 2) // stereo
	binary.LittleEndian.PutUint32(data[24:], 48000)
	binary.LittleEndian.PutUint32(data[28:], 48000*4)
	binary.LittleEndian.PutUint16(data[32:], 4)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], frames*4)
	for i := range frames {
		v := int16(12000 * math.Sin(2*math.Pi*440*float64(i)/48000))
		binary.LittleEndian.PutUint16(data[44+i*4:], uint16(v))
	}
	return data
}
