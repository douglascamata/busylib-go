package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// ConvertAudio decodes audio using ffmpeg and returns a .wav
// asset: headerless signed 16-bit little-endian PCM, mono, at 44100 Hz.
// Supported inputs depend on the installed FFmpeg build (typically WAV, MP3,
// OGG, AAC, M4A and FLAC). Like busylib-py, the .wav name does not imply a WAV
// header. Inputs named .raw or .pcm are returned unchanged with a .wav name;
// callers must supply PCM in the device format. No normalization is applied.
// Other inputs require the ffmpeg executable installed and available on PATH.
// The context cancels the conversion. Input bytes are stored in a temporary
// file during conversion so formats such as M4A can seek within the source.
func ConvertAudio(ctx context.Context, name string, data []byte) (Asset, error) {
	if err := ctx.Err(); err != nil {
		return Asset{}, err
	}
	ext := path.Ext(name)
	outputName := strings.TrimSuffix(name, ext) + ".wav"
	if strings.EqualFold(ext, ".raw") || strings.EqualFold(ext, ".pcm") {
		return Asset{Name: outputName, Data: data}, nil
	}
	src, err := os.CreateTemp("", "busylib-audio-*"+strings.ToLower(ext))
	if err != nil {
		return Asset{}, fmt.Errorf("media: create audio source: %w", err)
	}
	defer func() { _ = os.Remove(src.Name()) }()
	_, writeErr := src.Write(data)
	closeErr := src.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return Asset{}, fmt.Errorf("media: write audio source: %w", err)
	}

	stream := ffmpeg.Input(src.Name()).Output("pipe:1", ffmpeg.KwArgs{
		"ar": 44100, "ac": 1,
		"format": "s16le", "acodec": "pcm_s16le",
	}).GlobalArgs("-nostdin", "-hide_banner", "-loglevel", "error")
	// Compile/Run log globally; GetArgs lets us avoid changing ffmpeg-go's
	// global logging state and gives each call its own cancellation context.
	cmd := exec.CommandContext(ctx, "ffmpeg", stream.GetArgs()...)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Asset{}, fmt.Errorf("media: convert audio %q: %w", name, ctx.Err())
		}
		if errors.Is(err, exec.ErrNotFound) {
			return Asset{}, fmt.Errorf("media: audio conversion requires ffmpeg installed on PATH: %w", err)
		}
		return Asset{}, fmt.Errorf("media: convert audio %q: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return Asset{Name: outputName, Data: out.Bytes()}, nil
}
