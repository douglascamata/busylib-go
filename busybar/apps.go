package busybar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// AppInfo describes a JavaScript app installed on the bar or held in staging.
type AppInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	// APIVersion is the minimum HTTP API version required by the app.
	APIVersion  string `json:"api_version"`
	Author      string `json:"author"`
	Description string `json:"description"`
	IconPath    string `json:"icon_path"`
	IsDebug     bool   `json:"is_debug"`
}

type AppListResult struct {
	Apps []AppInfo `json:"apps"`
}

// AppStageResult holds a single-use installation key and the app it selects.
// Installed is present when installation would replace an existing app.
type AppStageResult struct {
	InstallKey uint32   `json:"install_key"`
	Staged     AppInfo  `json:"staged"`
	Installed  *AppInfo `json:"installed,omitempty"`
}

// AppSettingsDocument is the complete settings document for one app.
// Version must match the app's settings schema. Values preserves JSON numbers
// and nested objects without imposing an app-specific schema on callers.
type AppSettingsDocument struct {
	Version int                        `json:"version"`
	Values  map[string]json.RawMessage `json:"values"`
}

// AppsList lists installed JavaScript apps. App methods require a local
// connection (USB or Wi-Fi). They are supported by firmware 1.3.1, API 27.14.0.
func (c *Client) AppsList(ctx context.Context) (*AppListResult, error) {
	var out AppListResult
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/apps/list"}, &out)
}

// AppsStage uploads a TAR or TGZ package and returns the app it would install.
// The archive must contain an app directory, with appmeta/manifest.json inside.
// Use a longer context deadline if upload and unpacking need more time.
// Coordinate staging and installation per device: a later stage replaces the key.
func (c *Client) AppsStage(ctx context.Context, data []byte) (*AppStageResult, error) {
	var out AppStageResult
	return &out, c.do(ctx, request{
		method: http.MethodPost, path: "/apps/stage",
		body: data, contentType: "application/octet-stream",
	}, &out)
}

// AppsInstall installs the package selected by the single-use AppsStage key.
func (c *Client) AppsInstall(ctx context.Context, installKey uint32) error {
	return c.do(ctx, request{
		method: http.MethodPost, path: "/apps/install",
		query: url.Values{"install_key": {strconv.FormatUint(uint64(installKey), 10)}},
	}, nil)
}

// AppsRemove removes an app, stopping it if it is running. Its settings remain.
func (c *Client) AppsRemove(ctx context.Context, appID string) error {
	return c.do(ctx, request{
		method: http.MethodDelete, path: "/apps", query: url.Values{"app_id": {appID}},
	}, nil)
}

// AppsLaunch requests launch of an installed app. Script initialization can
// finish after the response. Incompatible API versions and higher-priority
// applications can prevent launch and return an HTTP error.
func (c *Client) AppsLaunch(ctx context.Context, appID string) error {
	return c.do(ctx, request{
		method: http.MethodPost, path: "/apps/launch", query: url.Values{"app_id": {appID}},
	}, nil)
}

// AppsQuit requests shutdown of the running app. No running app returns HTTP 409.
// Firmware 1.3.1 can return before shutdown finishes; immediate app removal can
// fail with HTTP 508. The client returns that server error without retrying.
func (c *Client) AppsQuit(ctx context.Context) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/apps/quit"}, nil)
}

// AppsSettingsGet reads an app's complete settings, creating defaults if needed.
func (c *Client) AppsSettingsGet(ctx context.Context, appID string) (*AppSettingsDocument, error) {
	var out AppSettingsDocument
	return &out, c.do(ctx, request{
		method: http.MethodGet, path: "/apps/settings", query: url.Values{"app_id": {appID}},
	}, &out)
}

// AppsSettingsSet replaces the entire settings document. Include every schema
// field and the matching version; the device validates values and drops unknown keys.
func (c *Client) AppsSettingsSet(ctx context.Context, appID string, document AppSettingsDocument) error {
	req, err := jsonRequest(http.MethodPut, "/apps/settings", document)
	if err != nil {
		return err
	}
	req.query = url.Values{"app_id": {appID}}
	return c.do(ctx, req, nil)
}

// AppsSettingsReset restores all settings to the app's schema defaults.
func (c *Client) AppsSettingsReset(ctx context.Context, appID string) error {
	return c.do(ctx, request{
		method: http.MethodDelete, path: "/apps/settings", query: url.Values{"app_id": {appID}},
	}, nil)
}
