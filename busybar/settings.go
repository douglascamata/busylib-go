package busybar

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
)

type HTTPAccessMode string

const (
	HTTPAccessDisabled HTTPAccessMode = "disabled"
	HTTPAccessEnabled  HTTPAccessMode = "enabled"
	HTTPAccessKey      HTTPAccessMode = "key"
)

type HTTPAccessInfo struct {
	Mode     HTTPAccessMode `json:"mode"`
	KeyValid bool           `json:"key_valid"`
}

type HTTPAccessParams struct {
	Mode HTTPAccessMode
	// Key is a 4 to 10 digit access password. Required for HTTPAccessKey.
	Key string
}

type NameInfo struct {
	Name string `json:"name"`
}

var accessKeyRe = regexp.MustCompile(`^\d{4,10}$`)

// SettingsAccessGet returns the HTTP access mode.
func (c *Client) SettingsAccessGet(ctx context.Context) (*HTTPAccessInfo, error) {
	var out HTTPAccessInfo
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/access"}, &out)
}

// SettingsAccessSet changes the HTTP access mode and password.
func (c *Client) SettingsAccessSet(ctx context.Context, params HTTPAccessParams) error {
	if params.Key != "" && !accessKeyRe.MatchString(params.Key) {
		return errors.New("busybar: access key must be 4 to 10 digits")
	}
	return c.do(ctx, request{
		method: http.MethodPost,
		path:   "/access",
		query:  url.Values{"mode": {string(params.Mode)}, "key": {params.Key}},
	}, nil)
}

// SettingsNameGet returns the device name.
func (c *Client) SettingsNameGet(ctx context.Context) (*NameInfo, error) {
	var out NameInfo
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/name"}, &out)
}

// SettingsNameSet renames the device.
func (c *Client) SettingsNameSet(ctx context.Context, name string) error {
	req, err := jsonRequest(http.MethodPost, "/name", NameInfo{Name: name})
	if err != nil {
		return err
	}
	return c.do(ctx, req, nil)
}
