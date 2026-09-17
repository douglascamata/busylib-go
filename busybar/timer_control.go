package busybar

import (
	"context"
	"errors"
	"time"
)

// ErrTimerNotRunning means the stored session is idle or has already finished.
var ErrTimerNotRunning = errors.New("busybar: no timer is running")

// BusyStop ends the session and keeps its display and smart-home settings.
func (c *Client) BusyStop(ctx context.Context) error {
	return c.updateTimer(ctx, func(s *BusySnapshotState) error {
		*s = BusySnapshotState{Type: BusyNotStarted, BusyBarSettings: s.BusyBarSettings}
		return nil
	})
}

// BusySetPaused pauses or resumes a session without restoring elapsed time.
// It returns ErrTimerNotRunning for an idle or finished session.
func (c *Client) BusySetPaused(ctx context.Context, paused bool) error {
	return c.updateTimer(ctx, func(s *BusySnapshotState) error {
		if s.Type == BusyNotStarted {
			return ErrTimerNotRunning
		}
		s.IsPaused = paused
		return nil
	})
}

// BusyNextPhase starts the next interval phase at its full length, even when
// autostart is off. Skipping the final work phase ends the session.
func (c *Client) BusyNextPhase(ctx context.Context) error {
	return c.updateTimer(ctx, func(s *BusySnapshotState) error {
		if s.Type == BusyNotStarted {
			return ErrTimerNotRunning
		}
		if s.Type != BusyInterval {
			return errors.New("busybar: next phase requires an interval timer")
		}
		s.CurrentInterval++
		if s.CurrentInterval >= s.IntervalSettings.IntervalWorkCyclesCount*2-1 {
			*s = BusySnapshotState{Type: BusyNotStarted, BusyBarSettings: s.BusyBarSettings}
			return nil
		}
		s.CurrentIntervalTimeTotalMs = intervalDuration(s.CurrentInterval, *s.IntervalSettings)
		s.CurrentIntervalTimeLeftMs = s.CurrentIntervalTimeTotalMs
		s.IsPaused = false
		return nil
	})
}

// BusySessionThemeSet changes the running session's theme without changing its
// remaining time or stored profile. The theme must exist on the device.
func (c *Client) BusySessionThemeSet(ctx context.Context, theme string) error {
	if theme == "" {
		return errors.New("busybar: theme must not be empty")
	}
	return c.updateTimer(ctx, func(s *BusySnapshotState) error {
		if s.Type == BusyNotStarted {
			return ErrTimerNotRunning
		}
		s.BusyBarSettings.Theme = theme
		return nil
	})
}

// updateTimer advances and stamps the snapshot at the same instant, then edits
// it. Callers must serialize controls for a device: HTTP read/write is not atomic.
func (c *Client) updateTimer(ctx context.Context, edit func(*BusySnapshotState) error) error {
	snapshot, err := c.BusySnapshotGet(ctx)
	if err != nil {
		return err
	}
	// The firmware ignores equal timestamps. A second write in the same
	// millisecond must still be newer, including when the device clock is ahead.
	stamp := max(time.Now().UnixMilli(), snapshot.SnapshotTimestampMs+1)
	state, err := snapshot.StateAt(time.UnixMilli(stamp))
	if err != nil {
		return err
	}
	s := &snapshot.Snapshot
	if state.IsFinished {
		*s = BusySnapshotState{Type: BusyNotStarted, BusyBarSettings: s.BusyBarSettings}
	} else {
		s.IsPaused = state.IsPaused
		switch s.Type {
		case BusySimple:
			s.TimeLeftMs = *state.TimeLeftMs
		case BusyInterval:
			if s.CurrentInterval != state.Interval {
				s.CurrentIntervalTimeTotalMs = intervalDuration(state.Interval, *s.IntervalSettings)
			}
			s.CurrentInterval = state.Interval
			s.CurrentIntervalTimeLeftMs = *state.TimeLeftMs
		}
	}
	if err := edit(s); err != nil {
		return err
	}
	snapshot.SnapshotTimestampMs = stamp
	return c.BusySnapshotSet(ctx, *snapshot)
}
