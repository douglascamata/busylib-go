package busybar

import (
	"context"
	"net/http"
	"net/url"
)

type InputKey string

const (
	KeyUp       InputKey = "up"
	KeyDown     InputKey = "down"
	KeyOK       InputKey = "ok"
	KeyBack     InputKey = "back"
	KeyStart    InputKey = "start"
	KeyBusy     InputKey = "busy"
	KeyCustom   InputKey = "custom"
	KeyOff      InputKey = "off"
	KeyApps     InputKey = "apps"
	KeySettings InputKey = "settings"
)

// InputSend simulates a button press, wheel step or switch move.
func (c *Client) InputSend(ctx context.Context, key InputKey) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/input", query: url.Values{"key": {string(key)}}}, nil)
}
