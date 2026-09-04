package busybar

import (
	"context"
	"net/http"
	"net/url"
)

type VersionInfo struct {
	APISemver string `json:"api_semver"`
}

type TransportType string

const (
	TransportUSB  TransportType = "usb"
	TransportWifi TransportType = "wifi"
)

type NetworkInterfaceInfo struct {
	Type TransportType `json:"type"`
}

type PowerState string

const (
	PowerDischarging PowerState = "discharging"
	PowerCharging    PowerState = "charging"
	PowerCharged     PowerState = "charged"
)

type StatusPower struct {
	State         PowerState `json:"state"`
	BatteryCharge int        `json:"battery_charge"`
	// The device reports these in mV and mA. The emulator reports decimals.
	BatteryVoltage float64 `json:"battery_voltage"`
	BatteryCurrent float64 `json:"battery_current"`
	USBVoltage     float64 `json:"usb_voltage"`
}

type StatusDevice struct {
	SerialNumber     string `json:"serial_number"`
	USBMac           string `json:"usb_mac"`
	WifiMac          string `json:"wifi_mac,omitempty"`
	BleMac           string `json:"ble_mac,omitempty"`
	OTPValid         bool   `json:"otp_valid"`
	OTPModel         string `json:"otp_model,omitempty"`
	OTPTimestamp     int64  `json:"otp_timestamp,omitempty"`
	FirmwareSecurity string `json:"firmware_security"`
}

type StatusFirmware struct {
	Version         string `json:"version"`
	Target          any    `json:"target"`
	Branch          string `json:"branch"`
	BuildDate       string `json:"build_date"`
	CommitHash      string `json:"commit_hash"`
	IntercomVersion string `json:"intercom_version"`
	NWPVersion      string `json:"nwp_version,omitempty"`
	MatterVersion   string `json:"matter_version,omitempty"`
}

type StatusSystem struct {
	APISemver         string `json:"api_semver"`
	Uptime            string `json:"uptime"`
	BootTime          int64  `json:"boot_time"`
	AutoUpdateEnabled bool   `json:"auto_update_enabled"`
}

type Status struct {
	Device   *StatusDevice   `json:"device,omitempty"`
	Firmware *StatusFirmware `json:"firmware,omitempty"`
	System   *StatusSystem   `json:"system,omitempty"`
	Power    *StatusPower    `json:"power,omitempty"`
}

type LogDumpResponse struct {
	Result string `json:"result"`
	Path   string `json:"path"`
}

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
