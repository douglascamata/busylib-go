package busybar

// Account

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

// Assets

type AssetsUploadParams struct {
	ApplicationName string
	// File is the file name inside the application's asset directory.
	File string
	Data []byte
}

// Audio

// AudioPlayParams plays either an uploaded asset (Path) or a stock sound (StockPath).
type AudioPlayParams struct {
	ApplicationName string `json:"application_name"`
	Path            string `json:"path,omitempty"`
	StockPath       string `json:"stock_path,omitempty"`
}

type AudioVolumeInfo struct {
	Volume int `json:"volume"`
}

type AudioVolumeParams struct {
	// Volume is 0-100.
	Volume int
	// Silent suppresses the volume-change sound.
	Silent bool
}

// BLE

type BleState string

const (
	BleReset          BleState = "reset"
	BleInitialization BleState = "initialization"
	BleDisabled       BleState = "disabled"
	BleEnabled        BleState = "enabled"
	BleConnectable    BleState = "connectable"
	BleConnected      BleState = "connected"
	BleInternalError  BleState = "internal error"
)

type BleStatus struct {
	Status BleState `json:"status"`
	// Address of the remote device. Only present when connected.
	Address string `json:"address,omitempty"`
}

// Busy

type BusyTimerType string

const (
	BusyNotStarted BusyTimerType = "NOT_STARTED"
	BusyInfinite   BusyTimerType = "INFINITE"
	BusySimple     BusyTimerType = "SIMPLE"
	BusyInterval   BusyTimerType = "INTERVAL"
)

type BusyProfileSlot string

const (
	BusySlotBusy   BusyProfileSlot = "busy"
	BusySlotCustom BusyProfileSlot = "custom"
)

type BusyBarSettings struct {
	Theme             string `json:"theme"`
	ShowWorkPhaseOnly bool   `json:"show_work_phase_only"`
	TriggerSmartHome  bool   `json:"trigger_smart_home"`
}

// BusyTimerSettings describes a timer. Which fields apply depends on Type:
// SIMPLE uses TotalTimeMs, INTERVAL uses the Interval* fields.
type BusyTimerSettings struct {
	Type                    BusyTimerType `json:"type"`
	TotalTimeMs             int64         `json:"total_time_ms,omitempty"`
	IntervalWorkMs          int64         `json:"interval_work_ms,omitempty"`
	IntervalRestMs          int64         `json:"interval_rest_ms,omitempty"`
	IntervalWorkCyclesCount int           `json:"interval_work_cycles_count,omitempty"`
	IsAutostartEnabled      bool          `json:"is_autostart_enabled,omitempty"`
}

// BusySnapshotState is the running timer. Which fields apply depends on Type.
type BusySnapshotState struct {
	Type                       BusyTimerType      `json:"type"`
	CardID                     string             `json:"card_id,omitempty"`
	IsPaused                   bool               `json:"is_paused"`
	TimeLeftMs                 int64              `json:"time_left_ms,omitempty"`
	CurrentInterval            int                `json:"current_interval,omitempty"`
	CurrentIntervalTimeTotalMs int64              `json:"current_interval_time_total_ms,omitempty"`
	CurrentIntervalTimeLeftMs  int64              `json:"current_interval_time_left_ms,omitempty"`
	IntervalSettings           *BusyTimerSettings `json:"interval_settings,omitempty"`
	BusyBarSettings            BusyBarSettings    `json:"busy_bar_settings"`
}

type BusySnapshot struct {
	Snapshot            BusySnapshotState `json:"snapshot"`
	SnapshotTimestampMs int64             `json:"snapshot_timestamp_ms"`
}

type BusyProfile struct {
	SortOrder          int               `json:"sort_order"`
	Title              string            `json:"title"`
	ID                 string            `json:"id"`
	TimerSettings      BusyTimerSettings `json:"timer_settings"`
	BusyBarSettings    BusyBarSettings   `json:"busy_bar_settings"`
	ProfileTimestampMs int64             `json:"profile_timestamp_ms"`
}

// Display

type DisplayBrightnessInfo struct {
	// Value is "auto" or "0".."100".
	Value string `json:"value"`
}

// Brightness is "auto" or a level from BrightnessLevel.
type Brightness string

const BrightnessAuto Brightness = "auto"

// Input

type InputKey string

const (
	KeyUp       InputKey = "up"
	KeyDown     InputKey = "down"
	KeyOK       InputKey = "ok"
	KeyBack     InputKey = "back"
	KeyStart    InputKey = "start"
	KeyBusy     InputKey = "busy"
	KeyCustom   InputKey = "custom"
	KeyOff      InputKey = "off"
	KeyApps     InputKey = "apps"
	KeySettings InputKey = "settings"
)

// Settings

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

// Smart home

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

// Storage

type StorageEntryType string

const (
	StorageFile StorageEntryType = "file"
	StorageDir  StorageEntryType = "dir"
)

type StorageEntry struct {
	Type StorageEntryType `json:"type"`
	Name string           `json:"name"`
	// Size in bytes. Only set for files.
	Size int64 `json:"size,omitempty"`
}

type StorageList struct {
	List []StorageEntry `json:"list"`
}

type StorageStatus struct {
	UsedBytes  int64 `json:"used_bytes"`
	FreeBytes  int64 `json:"free_bytes"`
	TotalBytes int64 `json:"total_bytes"`
}

// System

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

// Time

type TimestampInfo struct {
	// Timestamp is ISO 8601 with a timezone, e.g. 2025-10-02T14:30:45+04:00.
	Timestamp string `json:"timestamp"`
}

type TimezoneInfo struct {
	Name   string `json:"name"`
	Offset string `json:"offset"`
	Abbr   string `json:"abbr"`
}

type TimezoneList struct {
	List []TimezoneInfo `json:"list"`
}

// Update

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

// Wifi

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
