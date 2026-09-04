package busybar

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/douglascamata/busylib-go/frame"
)

type DisplayBrightnessInfo struct {
	// Value is "auto" or "0".."100".
	Value string `json:"value"`
}

// Brightness is "auto" or a level from BrightnessLevel.
type Brightness string

const BrightnessAuto Brightness = "auto"

// DisplayDrawParams is a draw request. Priority is 1-100; 0 means the default of 50.
type DisplayDrawParams struct {
	ApplicationName string `json:"application_name"`
	Priority        int    `json:"priority"`
	// LEDNotificationColor blinks the status LED, in #RRGGBBAA.
	LEDNotificationColor string    `json:"led_notification_color,omitempty"`
	Elements             []Element `json:"elements"`
}

// DisplayDraw shows elements on the display.
func (c *Client) DisplayDraw(ctx context.Context, params DisplayDrawParams) error {
	if params.Priority == 0 {
		params.Priority = 50
	}
	req, err := jsonRequest(http.MethodPost, "/display/draw", params)
	if err != nil {
		return err
	}
	return c.do(ctx, req, nil)
}

// DisplayClear removes the elements of one application, or of every
// application when applicationName is empty.
func (c *Client) DisplayClear(ctx context.Context, applicationName string) error {
	q := url.Values{}
	if applicationName != "" {
		q.Set("application_name", applicationName)
	}
	return c.do(ctx, request{method: http.MethodDelete, path: "/display/draw", query: q}, nil)
}

// DisplayBrightnessGet returns the brightness setting.
func (c *Client) DisplayBrightnessGet(ctx context.Context) (*DisplayBrightnessInfo, error) {
	var out DisplayBrightnessInfo
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/display/brightness"}, &out)
}

// BrightnessLevel converts a 0-100 level to a Brightness value.
func BrightnessLevel(level int) Brightness {
	return Brightness(strconv.Itoa(level))
}

// DisplayBrightnessSet sets the brightness to BrightnessAuto or a BrightnessLevel.
func (c *Client) DisplayBrightnessSet(ctx context.Context, value Brightness) error {
	if value != BrightnessAuto {
		if n, err := strconv.Atoi(string(value)); err != nil || n < 0 || n > 100 {
			return errors.New("busybar: brightness must be between 0 and 100 or auto")
		}
	}
	return c.do(ctx, request{
		method: http.MethodPost,
		path:   "/display/brightness",
		query:  url.Values{"value": {string(value)}},
	}, nil)
}

// FrameFormat selects how DisplayScreenFrameGet returns pixels.
type FrameFormat int

const (
	// FrameRaw returns the bytes as the device sends them: BGR for the front
	// display, 4-bit grayscale for the back display.
	FrameRaw FrameFormat = iota
	// FrameRGBA converts the frame to 4 bytes per pixel.
	FrameRGBA
)

// DisplayScreenFrameGet captures the current frame of a display.
func (c *Client) DisplayScreenFrameGet(ctx context.Context, display frame.Display, format FrameFormat) ([]byte, error) {
	var body []byte
	err := c.do(ctx, request{
		method: http.MethodGet,
		path:   "/screen",
		query:  url.Values{"display": {strconv.Itoa(int(display))}},
	}, &body)
	if err != nil {
		return nil, err
	}
	// The device answers with base64 text; fall back to the raw body otherwise.
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
	if err != nil {
		raw = body
	}
	if format == FrameRaw {
		return raw, nil
	}
	w, h := frame.Dimensions(display)
	if display == frame.Back {
		return frame.L4ToRGBA(raw, w, h), nil
	}
	return frame.BGRToRGBA(raw, w, h), nil
}
