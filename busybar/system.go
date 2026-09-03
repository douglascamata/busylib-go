package busybar

import (
	"context"
	"net/http"
	"net/url"
)

// SystemVersionGet returns the API version. This is the only call that does
// not send the X-API-Sem-Ver header.
func (c *Client) SystemVersionGet(ctx context.Context) (*VersionInfo, error) {
	var out VersionInfo
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/version"}, &out)
}

// SystemStatusGet returns all status groups.
func (c *Client) SystemStatusGet(ctx context.Context) (*Status, error) {
	var out Status
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/status"}, &out)
}

// SystemInfoGet returns the system status group.
func (c *Client) SystemInfoGet(ctx context.Context) (*StatusSystem, error) {
	var out StatusSystem
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/status/system"}, &out)
}

// SystemStatusPowerGet returns the power status group.
func (c *Client) SystemStatusPowerGet(ctx context.Context) (*StatusPower, error) {
	var out StatusPower
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/status/power"}, &out)
}

// SystemStatusDeviceGet returns the device status group.
func (c *Client) SystemStatusDeviceGet(ctx context.Context) (*StatusDevice, error) {
	var out StatusDevice
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/status/device"}, &out)
}

// SystemStatusFirmwareGet returns the firmware status group.
func (c *Client) SystemStatusFirmwareGet(ctx context.Context) (*StatusFirmware, error) {
	var out StatusFirmware
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/status/firmware"}, &out)
}

// SystemTransportGet reports whether the request arrived over USB or Wi-Fi.
func (c *Client) SystemTransportGet(ctx context.Context) (*NetworkInterfaceInfo, error) {
	var out NetworkInterfaceInfo
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/transport"}, &out)
}

// SystemLogDump writes the device log to storage. filename may be empty.
func (c *Client) SystemLogDump(ctx context.Context, filename string) (*LogDumpResponse, error) {
	q := url.Values{}
	if filename != "" {
		q.Set("filename", filename)
	}
	var out LogDumpResponse
	return &out, c.do(ctx, request{method: http.MethodPost, path: "/log_dump", query: q}, &out)
}
