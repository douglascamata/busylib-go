package busybar

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
)

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
