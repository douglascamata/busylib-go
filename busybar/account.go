package busybar

import (
	"context"
	"net/http"
)

type AccountConnection string

const (
	AccountConnected    AccountConnection = "connected"
	AccountDisconnected AccountConnection = "disconnected"
	AccountError        AccountConnection = "error"
)

type AccountInfo struct {
	Linked bool   `json:"linked"`
	ID     string `json:"id"`
	Email  string `json:"email"`
	UserID string `json:"user_id"`
}

type AccountLink struct {
	Code      string `json:"code"`
	ExpiresAt int64  `json:"expires_at"`
}

type AccountStatus struct {
	Status AccountConnection `json:"status"`
}

type ClientCertType string

const (
	ClientCertDefault ClientCertType = "default"
	ClientCertCustom  ClientCertType = "custom"
	ClientCertNone    ClientCertType = "none"
)

// AccountBackend is the MQTT backend configuration.
type AccountBackend struct {
	ServerURL        string         `json:"server_url"`
	ClientCertType   ClientCertType `json:"client_cert_type"`
	IgnoreServerCert bool           `json:"ignore_server_cert"`
}

// AccountInfoGet returns the linked BUSY account.
func (c *Client) AccountInfoGet(ctx context.Context) (*AccountInfo, error) {
	var out AccountInfo
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/account/info"}, &out)
}

// AccountStateGet returns the cloud connection state.
func (c *Client) AccountStateGet(ctx context.Context) (*AccountStatus, error) {
	var out AccountStatus
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/account/status"}, &out)
}

// AccountBackendGet returns the MQTT backend configuration.
func (c *Client) AccountBackendGet(ctx context.Context) (*AccountBackend, error) {
	var out AccountBackend
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/account/backend"}, &out)
}

// AccountBackendSet replaces the MQTT backend configuration.
func (c *Client) AccountBackendSet(ctx context.Context, params AccountBackend) error {
	req, err := jsonRequest(http.MethodPut, "/account/backend", params)
	if err != nil {
		return err
	}
	return c.do(ctx, req, nil)
}

// AccountLink starts linking the device to an account and returns the code to enter.
func (c *Client) AccountLink(ctx context.Context) (*AccountLink, error) {
	var out AccountLink
	return &out, c.do(ctx, request{method: http.MethodPost, path: "/account/link"}, &out)
}

// AccountUnlink removes the device from its account.
func (c *Client) AccountUnlink(ctx context.Context) error {
	return c.do(ctx, request{method: http.MethodDelete, path: "/account"}, nil)
}
