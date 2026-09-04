// Package media prepares images and audio for upload to BUSY Bar.
// Conversion is explicit: pass the returned asset to busybar.AssetsUpload or
// busybar.StorageWrite, then reference its name when drawing or playing it.
// Audio conversion requires an ffmpeg executable on PATH.
package media

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
)

// ErrUnsupported indicates a video or animation conversion that is not implemented.
var ErrUnsupported = errors.New("media: conversion is not supported for this file type")

// Asset is a converted file. Name preserves the input path with a new extension.
type Asset struct {
	Name string
	Data []byte
}

// ConvertForStorage follows busylib-py's extension-based converter selection.
// Images use default ImageOptions. GIF and video files are rejected, including
// static GIFs. Unknown extensions (including .raw and .pcm) pass through unchanged.
// Use ConvertImage for custom image options, or ConvertAudio to rename raw PCM.
func ConvertForStorage(ctx context.Context, name string, data []byte) (Asset, error) {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".bmp", ".tif", ".tiff", ".webp":
		return ConvertImage(name, data, ImageOptions{})
	case ".mp3", ".ogg", ".aac", ".m4a", ".flac", ".wav":
		return ConvertAudio(ctx, name, data)
	case ".gif", ".mov", ".mp4", ".mkv", ".avi", ".webm":
		return Asset{}, fmt.Errorf("media: convert %q: %w", name, ErrUnsupported)
	default:
		return Asset{Name: name, Data: data}, nil
	}
}
