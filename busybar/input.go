package busybar

import (
	"context"
	"net/http"
	"net/url"
)

// InputSend simulates a button press, wheel step or switch move.
func (c *Client) InputSend(ctx context.Context, key InputKey) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/input", query: url.Values{"key": {string(key)}}}, nil)
}
