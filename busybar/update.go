package busybar

import (
	"context"
	"net/http"
	"net/url"
)

// UpdateFromFile installs a firmware bundle uploaded by the caller.
func (c *Client) UpdateFromFile(ctx context.Context, file []byte) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/update", body: file, contentType: "application/octet-stream"}, nil)
}

// UpdateCheck asks the device to look for a new firmware version.
func (c *Client) UpdateCheck(ctx context.Context) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/update/check"}, nil)
}

// UpdateStatusGet returns the update check and install progress.
func (c *Client) UpdateStatusGet(ctx context.Context) (*UpdateStatus, error) {
	var out UpdateStatus
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/update/status"}, &out)
}

// UpdateChangelogGet returns the changelog of a firmware version.
func (c *Client) UpdateChangelogGet(ctx context.Context, version string) (*UpdateChangelog, error) {
	var out UpdateChangelog
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/update/changelog", query: url.Values{"version": {version}}}, &out)
}

// UpdateInstall downloads and installs a firmware version.
func (c *Client) UpdateInstall(ctx context.Context, version string) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/update/install", query: url.Values{"version": {version}}}, nil)
}

// UpdateAbort cancels a running download.
func (c *Client) UpdateAbort(ctx context.Context) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/update/abort_download"}, nil)
}

// UpdateAutoUpdateGet returns the automatic update settings.
func (c *Client) UpdateAutoUpdateGet(ctx context.Context) (*AutoUpdateSettings, error) {
	var out AutoUpdateSettings
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/update/autoupdate"}, &out)
}

// UpdateAutoUpdateSet changes the automatic update settings.
func (c *Client) UpdateAutoUpdateSet(ctx context.Context, params AutoUpdateSettings) error {
	req, err := jsonRequest(http.MethodPost, "/update/autoupdate", params)
	if err != nil {
		return err
	}
	return c.do(ctx, req, nil)
}
