package busybar

import (
	"context"
	"net/http"
)

// SmartHomePairingGet returns the Matter pairing state.
func (c *Client) SmartHomePairingGet(ctx context.Context) (*SmartHomePairingInfo, error) {
	var out SmartHomePairingInfo
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/smart_home/pairing"}, &out)
}

// SmartHomePair opens a Matter pairing window and returns the codes to use.
func (c *Client) SmartHomePair(ctx context.Context) (*SmartHomePairingPayload, error) {
	var out SmartHomePairingPayload
	return &out, c.do(ctx, request{method: http.MethodPost, path: "/smart_home/pairing"}, &out)
}

// SmartHomeErase removes all Matter pairings.
func (c *Client) SmartHomeErase(ctx context.Context) error {
	return c.do(ctx, request{method: http.MethodDelete, path: "/smart_home/pairing"}, nil)
}

// SmartHomeSwitchStateGet returns the emulated switch state.
func (c *Client) SmartHomeSwitchStateGet(ctx context.Context) (*SmartHomeSwitchState, error) {
	var out SmartHomeSwitchState
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/smart_home/switch"}, &out)
}

// SmartHomeSwitchStateSet sets the emulated switch state.
func (c *Client) SmartHomeSwitchStateSet(ctx context.Context, params SmartHomeSwitchState) error {
	req, err := jsonRequest(http.MethodPost, "/smart_home/switch", params)
	if err != nil {
		return err
	}
	return c.do(ctx, req, nil)
}
