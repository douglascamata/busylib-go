package busybar

import (
	"context"
	"net/http"
)

type SmartHomePairingState string

const (
	PairingNeverStarted          SmartHomePairingState = "never_started"
	PairingStarted               SmartHomePairingState = "started"
	PairingCompletedSuccessfully SmartHomePairingState = "completed_successfully"
	PairingFailed                SmartHomePairingState = "failed"
)

type SmartHomePairingStatus struct {
	Value     SmartHomePairingState `json:"value"`
	Timestamp int64                 `json:"timestamp,omitempty"`
}

type SmartHomePairingInfo struct {
	FabricCount         int                     `json:"fabric_count"`
	LatestPairingStatus *SmartHomePairingStatus `json:"latest_pairing_status,omitempty"`
}

type SmartHomePairingPayload struct {
	AvailableUntil string `json:"available_until"`
	QRCode         string `json:"qr_code"`
	ManualCode     string `json:"manual_code"`
}

type SwitchStartup string

const (
	SwitchStartupOff    SwitchStartup = "off"
	SwitchStartupOn     SwitchStartup = "on"
	SwitchStartupToggle SwitchStartup = "toggle"
	SwitchStartupLast   SwitchStartup = "last"
)

type SmartHomeSwitchState struct {
	State bool `json:"state"`
	// Startup is only meaningful when setting the state; the device never returns it.
	Startup SwitchStartup `json:"startup,omitempty"`
}

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
