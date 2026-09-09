package busybar

import (
	"fmt"
	"time"
)

type TimerPhase string

const (
	TimerWork TimerPhase = "work"
	TimerRest TimerPhase = "rest"
)

// TimerState is the state estimated from a snapshot at one moment.
// It does not modify the snapshot or account for later device/user changes.
type TimerState struct {
	Mode     BusyTimerType
	IsPaused bool
	// Phase is empty for idle, simple, and finished timers.
	Phase TimerPhase
	// Interval is the zero-based phase index, meaningful only in interval mode.
	Interval int
	// TimeLeftMs is nil for idle and infinite timers.
	TimeLeftMs *int64
	IsFinished bool
	// ElapsedMs is time since the snapshot, not total session time.
	// It is zero if the snapshot was already paused.
	ElapsedMs int64
}

// IsRunning reports whether a session is active, including a paused session.
func (s TimerState) IsRunning() bool { return s.Mode != BusyNotStarted && !s.IsFinished }

// StateAt advances the snapshot to now without changing it. It uses millisecond
// arithmetic like busylib-py; the device display itself ticks in seconds.
// Supply time aligned with the device clock. A future snapshot is not advanced.
// Interval timers wait at the next phase when autostart is disabled.
func (s BusySnapshot) StateAt(now time.Time) (TimerState, error) {
	inner := s.Snapshot
	state := TimerState{Mode: inner.Type, IsPaused: inner.IsPaused}
	if !inner.IsPaused {
		state.ElapsedMs = max(0, now.UnixMilli()-s.SnapshotTimestampMs)
	}
	switch inner.Type {
	case BusyNotStarted:
		return TimerState{Mode: BusyNotStarted}, nil
	case BusyInfinite:
		state.Phase = TimerWork
		return state, nil
	case BusySimple:
		left := inner.TimeLeftMs
		if !inner.IsPaused {
			left -= state.ElapsedMs
			state.IsFinished = left <= 0
		}
		state.TimeLeftMs = new(max(0, left))
		return state, nil
	case BusyInterval:
		// Callers can construct snapshots themselves; interval arithmetic needs
		// settings and positive phase lengths. Device snapshots satisfy these.
		settings := inner.IntervalSettings
		if settings == nil || settings.IntervalWorkMs <= 0 || settings.IntervalRestMs <= 0 || settings.IntervalWorkCyclesCount <= 0 {
			return TimerState{}, fmt.Errorf("busybar: interval snapshot needs positive work/rest durations and cycle count")
		}
		state.Interval = inner.CurrentInterval
		left := inner.CurrentIntervalTimeLeftMs
		if !inner.IsPaused {
			left -= state.ElapsedMs
			last := settings.IntervalWorkCyclesCount*2 - 1
			for left <= 0 {
				state.Interval++
				if state.Interval >= last {
					state.Interval = last
					state.TimeLeftMs = new(int64(0))
					state.IsFinished = true
					return state, nil
				}
				duration := settings.IntervalWorkMs
				if state.Interval%2 != 0 {
					duration = settings.IntervalRestMs
				}
				if !settings.IsAutostartEnabled {
					left = duration
					state.IsPaused = true
					break
				}
				left += duration
			}
		}
		state.Phase = TimerWork
		if state.Interval%2 != 0 {
			state.Phase = TimerRest
		}
		state.TimeLeftMs = new(left)
		return state, nil
	default:
		return TimerState{}, fmt.Errorf("busybar: unsupported snapshot type %q", inner.Type)
	}
}
