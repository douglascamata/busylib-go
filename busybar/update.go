package busybar

import (
	"context"
	"net/http"
	"net/url"
)

type UpdateEvent string

const (
	UpdateEventSessionStart   UpdateEvent = "session_start"
	UpdateEventSessionStop    UpdateEvent = "session_stop"
	UpdateEventActionBegin    UpdateEvent = "action_begin"
	UpdateEventActionDone     UpdateEvent = "action_done"
	UpdateEventDetailChange   UpdateEvent = "detail_change"
	UpdateEventActionProgress UpdateEvent = "action_progress"
	UpdateEventNone           UpdateEvent = "none"
)

type UpdateAction string

const (
	UpdateActionDownload        UpdateAction = "download"
	UpdateActionShaVerification UpdateAction = "sha_verification"
	UpdateActionUnpack          UpdateAction = "unpack"
	UpdateActionPrepare         UpdateAction = "prepare"
	UpdateActionApply           UpdateAction = "apply"
	UpdateActionNone            UpdateAction = "none"
)

type UpdateResult string

const (
	UpdateOK                          UpdateResult = "ok"
	UpdateBatteryLow                  UpdateResult = "battery_low"
	UpdateBusy                        UpdateResult = "busy"
	UpdateDownloadFailure             UpdateResult = "download_failure"
	UpdateDownloadAbort               UpdateResult = "download_abort"
	UpdateShaMismatch                 UpdateResult = "sha_mismatch"
	UpdateUnpackStagingDirFailure     UpdateResult = "unpack_staging_dir_failure"
	UpdateUnpackArchiveOpenFailure    UpdateResult = "unpack_archive_open_failure"
	UpdateUnpackArchiveUnpackFailure  UpdateResult = "unpack_archive_unpack_failure"
	UpdateInstallManifestNotFound     UpdateResult = "install_manifest_not_found"
	UpdateInstallManifestInvalid      UpdateResult = "install_manifest_invalid"
	UpdateInstallSessionConfigFailure UpdateResult = "install_session_config_failure"
	UpdateInstallPointerSetupFailure  UpdateResult = "install_pointer_setup_failure"
	UpdateUnknownFailure              UpdateResult = "unknown_failure"
)

type UpdateCheckEvent string

const (
	CheckEventStart UpdateCheckEvent = "start"
	CheckEventStop  UpdateCheckEvent = "stop"
	CheckEventNone  UpdateCheckEvent = "none"
)

type UpdateCheckResult string

const (
	CheckAvailable    UpdateCheckResult = "available"
	CheckNotAvailable UpdateCheckResult = "not_available"
	CheckFailure      UpdateCheckResult = "failure"
	CheckNone         UpdateCheckResult = "none"
)

type UpdateDownloadProgress struct {
	SpeedBytesPerSec int64 `json:"speed_bytes_per_sec"`
	ReceivedBytes    int64 `json:"received_bytes"`
	TotalBytes       int64 `json:"total_bytes"`
}

type UpdateInstallStatus struct {
	IsAllowed bool                    `json:"is_allowed"`
	Event     UpdateEvent             `json:"event"`
	Action    UpdateAction            `json:"action"`
	Status    UpdateResult            `json:"status"`
	Detail    string                  `json:"detail"`
	Download  *UpdateDownloadProgress `json:"download,omitempty"`
}

type UpdateCheckStatus struct {
	AvailableVersion string            `json:"available_version"`
	Event            UpdateCheckEvent  `json:"event"`
	Status           UpdateCheckResult `json:"status"`
}

type UpdateStatus struct {
	Install *UpdateInstallStatus `json:"install,omitempty"`
	Check   *UpdateCheckStatus   `json:"check,omitempty"`
}

type UpdateChangelog struct {
	Changelog string `json:"changelog"`
}

// AutoUpdateSettings configures automatic updates. All fields are optional
// when setting; IsEnabled is a pointer so that false can be sent.
type AutoUpdateSettings struct {
	IsEnabled *bool `json:"is_enabled,omitempty"`
	// IntervalStart and IntervalEnd are HH:MM.
	IntervalStart string `json:"interval_start,omitempty"`
	IntervalEnd   string `json:"interval_end,omitempty"`
}

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
