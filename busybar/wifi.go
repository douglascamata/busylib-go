package busybar

import (
	"context"
	"net/http"
)

// WifiStatusGet returns the Wi-Fi connection state.
func (c *Client) WifiStatusGet(ctx context.Context) (*WifiStatus, error) {
	var out WifiStatus
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/wifi/status"}, &out)
}

// WifiConnect joins a network.
func (c *Client) WifiConnect(ctx context.Context, params WifiConnectParams) error {
	req, err := jsonRequest(http.MethodPost, "/wifi/connect", params)
	if err != nil {
		return err
	}
	return c.do(ctx, req, nil)
}

// WifiDisconnect leaves the current network.
func (c *Client) WifiDisconnect(ctx context.Context) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/wifi/disconnect"}, nil)
}

// WifiNetworksGet scans for networks.
func (c *Client) WifiNetworksGet(ctx context.Context) (*WifiNetworks, error) {
	var out WifiNetworks
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/wifi/networks"}, &out)
}
