package busybar

import (
	"context"
	"encoding/json"
	"fmt"
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

// MarshalJSON includes required zero values only for the selected timer type.
func (s BusyTimerSettings) MarshalJSON() ([]byte, error) {
	fields := map[string]any{"type": s.Type}
	switch s.Type {
	case BusyInfinite:
	case BusySimple:
		fields["total_time_ms"] = s.TotalTimeMs
	case BusyInterval:
		fields["interval_work_ms"] = s.IntervalWorkMs
		fields["interval_rest_ms"] = s.IntervalRestMs
		fields["interval_work_cycles_count"] = s.IntervalWorkCyclesCount
		fields["is_autostart_enabled"] = s.IsAutostartEnabled
	default:
		return nil, fmt.Errorf("busybar: unsupported timer settings type %q", s.Type)
	}
	return json.Marshal(fields)
}

// MarshalJSON includes the fields required by the selected snapshot type.
func (s BusySnapshotState) MarshalJSON() ([]byte, error) {
	fields := map[string]any{"type": s.Type, "busy_bar_settings": s.BusyBarSettings}
	switch s.Type {
	case BusyNotStarted:
		return json.Marshal(fields)
	case BusyInfinite:
	case BusySimple:
		fields["time_left_ms"] = s.TimeLeftMs
	case BusyInterval:
		fields["current_interval"] = s.CurrentInterval
		fields["current_interval_time_total_ms"] = s.CurrentIntervalTimeTotalMs
		fields["current_interval_time_left_ms"] = s.CurrentIntervalTimeLeftMs
		fields["interval_settings"] = s.IntervalSettings
	default:
		return nil, fmt.Errorf("busybar: unsupported snapshot type %q", s.Type)
	}
	fields["card_id"], fields["is_paused"] = s.CardID, s.IsPaused
	return json.Marshal(fields)
}

// BusySnapshotGet returns the last stored timer snapshot. Use StateAt to
// calculate its current state; repeated reads need not change the timestamp.
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
