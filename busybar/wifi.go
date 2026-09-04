package busybar

import (
	"context"
	"net/http"
)

type WifiSecurityMethod string

const (
	WifiOpen        WifiSecurityMethod = "Open"
	WifiWPA         WifiSecurityMethod = "WPA"
	WifiWPA2        WifiSecurityMethod = "WPA2"
	WifiWEP         WifiSecurityMethod = "WEP"
	WifiWPAWPA2     WifiSecurityMethod = "WPA/WPA2"
	WifiWPA3        WifiSecurityMethod = "WPA3"
	WifiWPA2WPA3    WifiSecurityMethod = "WPA2/WPA3"
	WifiUnsupported WifiSecurityMethod = "Unsupported"
)

type WifiIPMethod string

const (
	WifiDHCP   WifiIPMethod = "dhcp"
	WifiStatic WifiIPMethod = "static"
)

type WifiIPType string

const (
	WifiIPv4 WifiIPType = "ipv4"
	WifiIPv6 WifiIPType = "ipv6"
)

type WifiState string

const (
	WifiUnknown       WifiState = "unknown"
	WifiDisconnected  WifiState = "disconnected"
	WifiConnected     WifiState = "connected"
	WifiConnecting    WifiState = "connecting"
	WifiDisconnecting WifiState = "disconnecting"
	WifiReconnecting  WifiState = "reconnecting"
)

type WifiIPConfig struct {
	IPMethod WifiIPMethod `json:"ip_method"`
	Address  string       `json:"address,omitempty"`
	Mask     string       `json:"mask,omitempty"`
	Gateway  string       `json:"gateway,omitempty"`
}

type WifiConnectParams struct {
	SSID     string             `json:"ssid"`
	Password string             `json:"password,omitempty"`
	Security WifiSecurityMethod `json:"security"`
	IPConfig WifiIPConfig       `json:"ip_config"`
}

type WifiStatusIPConfig struct {
	IPMethod WifiIPMethod `json:"ip_method"`
	IPType   WifiIPType   `json:"ip_type"`
	Address  string       `json:"address"`
}

// WifiStatus reports the Wi-Fi state. Only State is always present; the other
// fields are set while connected.
type WifiStatus struct {
	State    WifiState           `json:"state"`
	SSID     string              `json:"ssid,omitempty"`
	BSSID    string              `json:"bssid,omitempty"`
	Channel  int                 `json:"channel,omitempty"`
	RSSI     int                 `json:"rssi,omitempty"`
	Security WifiSecurityMethod  `json:"security,omitempty"`
	IPConfig *WifiStatusIPConfig `json:"ip_config,omitempty"`
}

type WifiNetwork struct {
	SSID     string             `json:"ssid"`
	Security WifiSecurityMethod `json:"security"`
	RSSI     int                `json:"rssi"`
}

type WifiNetworks struct {
	Count    int           `json:"count"`
	Networks []WifiNetwork `json:"networks"`
}

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
