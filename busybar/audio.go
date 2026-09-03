package busybar

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

// AudioPlay plays an uploaded or stock sound.
func (c *Client) AudioPlay(ctx context.Context, params AudioPlayParams) error {
	req, err := jsonRequest(http.MethodPost, "/audio/play", params)
	if err != nil {
		return err
	}
	return c.do(ctx, req, nil)
}

// AudioStop stops playback.
func (c *Client) AudioStop(ctx context.Context) error {
	return c.do(ctx, request{method: http.MethodDelete, path: "/audio/play"}, nil)
}

// AudioVolumeGet returns the current volume.
func (c *Client) AudioVolumeGet(ctx context.Context) (*AudioVolumeInfo, error) {
	var out AudioVolumeInfo
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/audio/volume"}, &out)
}

// AudioVolumeSet sets the volume (0-100).
func (c *Client) AudioVolumeSet(ctx context.Context, params AudioVolumeParams) error {
	if params.Volume < 0 || params.Volume > 100 {
		return errors.New("busybar: volume must be between 0 and 100")
	}
	q := url.Values{"volume": {strconv.Itoa(params.Volume)}}
	if params.Silent {
		q.Set("silent", "1")
	}
	return c.do(ctx, request{method: http.MethodPost, path: "/audio/volume", query: q}, nil)
}
