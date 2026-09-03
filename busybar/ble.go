package busybar

import (
	"context"
	"net/http"
)

// BleEnable turns Bluetooth on.
func (c *Client) BleEnable(ctx context.Context) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/ble/enable"}, nil)
}

// BleDisable turns Bluetooth off.
func (c *Client) BleDisable(ctx context.Context) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/ble/disable"}, nil)
}

// BleUnpair forgets the paired device.
func (c *Client) BleUnpair(ctx context.Context) error {
	return c.do(ctx, request{method: http.MethodDelete, path: "/ble/pairing"}, nil)
}

// BleStatusGet returns the Bluetooth service state.
func (c *Client) BleStatusGet(ctx context.Context) (*BleStatus, error) {
	var out BleStatus
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/ble/status"}, &out)
}
