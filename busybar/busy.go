package busybar

import (
	"context"
	"net/http"
)

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

// BusySnapshotGet returns the running timer state.
func (c *Client) BusySnapshotGet(ctx context.Context) (*BusySnapshot, error) {
	var out BusySnapshot
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/busy/snapshot"}, &out)
}

// BusySnapshotSet replaces the running timer state.
func (c *Client) BusySnapshotSet(ctx context.Context, snapshot BusySnapshot) error {
	req, err := jsonRequest(http.MethodPut, "/busy/snapshot", snapshot)
	if err != nil {
		return err
	}
	return c.do(ctx, req, nil)
}

// BusyProfileGet returns the profile stored in a switch slot.
func (c *Client) BusyProfileGet(ctx context.Context, slot BusyProfileSlot) (*BusyProfile, error) {
	var out BusyProfile
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/busy/profiles/" + string(slot)}, &out)
}

// BusyProfileSet replaces the profile stored in a switch slot.
func (c *Client) BusyProfileSet(ctx context.Context, slot BusyProfileSlot, profile BusyProfile) error {
	req, err := jsonRequest(http.MethodPut, "/busy/profiles/"+string(slot), profile)
	if err != nil {
		return err
	}
	return c.do(ctx, req, nil)
}
