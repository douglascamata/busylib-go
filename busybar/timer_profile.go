package busybar

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// BusyStartParams selects the settings for a new session. The zero value starts
// the busy slot's profile. Overrides affect only this session.
type BusyStartParams struct {
	// Slot defaults to BusySlotBusy. It cannot be combined with CardID.
	Slot BusyProfileSlot
	// CardID starts a card without reading either stored profile. In this case,
	// TimerSettings and BusyBarSettings must both be supplied.
	CardID string
	// Nil inherits the selected profile's settings. Non-nil replaces them.
	TimerSettings   *BusyTimerSettings
	BusyBarSettings *BusyBarSettings
}

// BusyStart replaces the current session. It reads the latest snapshot to choose
// a newer timestamp. It never writes a stored profile, including for overrides.
func (c *Client) BusyStart(ctx context.Context, params BusyStartParams) error {
	var timer BusyTimerSettings
	var settings BusyBarSettings
	cardID := params.CardID
	if cardID == "" {
		slot := params.Slot
		if slot == "" {
			slot = BusySlotBusy
		}
		profile, err := c.BusyProfileGet(ctx, slot)
		if err != nil {
			return err
		}
		cardID, timer, settings = profile.ID, profile.TimerSettings, profile.BusyBarSettings
	} else {
		if params.Slot != "" {
			return errors.New("busybar: select either a profile slot or a card ID")
		}
		if params.TimerSettings == nil || params.BusyBarSettings == nil {
			return errors.New("busybar: an explicit card ID needs timer and display settings")
		}
	}
	if params.TimerSettings != nil {
		timer = *params.TimerSettings
	}
	if params.BusyBarSettings != nil {
		settings = *params.BusyBarSettings
	}
	if err := validateBusyTimer(cardID, timer); err != nil {
		return err
	}

	s := BusySnapshotState{Type: timer.Type, CardID: cardID, BusyBarSettings: settings}
	switch timer.Type {
	case BusySimple:
		s.TimeLeftMs = timer.TotalTimeMs
	case BusyInterval:
		s.IntervalSettings = &timer
		s.CurrentIntervalTimeTotalMs = timer.IntervalWorkMs
		s.CurrentIntervalTimeLeftMs = timer.IntervalWorkMs
	}
	previous, err := c.BusySnapshotGet(ctx)
	if err != nil {
		return err
	}
	return c.BusySnapshotSet(ctx, BusySnapshot{
		Snapshot:            s,
		SnapshotTimestampMs: max(time.Now().UnixMilli(), previous.SnapshotTimestampMs+1),
	})
}

// BusyProfileUpdate reads a profile, calls edit synchronously, validates its
// timer, and writes it with a newer timestamp. It returns the profile sent.
// An edit error prevents the write. This does not change the running session.
// Replace TimerSettings to change the timer type; no durations are filled in.
func (c *Client) BusyProfileUpdate(ctx context.Context, slot BusyProfileSlot, edit func(*BusyProfile) error) (*BusyProfile, error) {
	profile, err := c.BusyProfileGet(ctx, slot)
	if err != nil {
		return nil, err
	}
	previousStamp := profile.ProfileTimestampMs
	if err := edit(profile); err != nil {
		return nil, err
	}
	if err := validateBusyTimer(profile.ID, profile.TimerSettings); err != nil {
		return nil, err
	}
	profile.ProfileTimestampMs = max(time.Now().UnixMilli(), previousStamp+1)
	if err := c.BusyProfileSet(ctx, slot, *profile); err != nil {
		return nil, err
	}
	return profile, nil
}

var busyCardIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// These limits come from firmware busy_timer_common.{h,c}. Invalid profiles
// otherwise receive HTTP OK but are silently discarded by the timer service.
func validateBusyTimer(cardID string, s BusyTimerSettings) error {
	if !busyCardIDPattern.MatchString(cardID) {
		return fmt.Errorf("busybar: card ID %q must have the form xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx in hexadecimal", cardID)
	}
	switch s.Type {
	case BusyInfinite:
		return nil
	case BusySimple:
		if s.TotalTimeMs < 0 || s.TotalTimeMs > (24*time.Hour).Milliseconds() {
			return errors.New("busybar: countdown duration must be between 0 and 24 hours")
		}
	case BusyInterval:
		for _, phase := range []struct {
			name string
			ms   int64
		}{{"work", s.IntervalWorkMs}, {"rest", s.IntervalRestMs}} {
			if phase.ms < (5*time.Minute).Milliseconds() || phase.ms > (8*time.Hour).Milliseconds() {
				return fmt.Errorf("busybar: interval %s duration must be between 5 minutes and 8 hours", phase.name)
			}
		}
		if s.IntervalWorkCyclesCount < 2 || s.IntervalWorkCyclesCount > 35 {
			return errors.New("busybar: interval timer must have between 2 and 35 work cycles")
		}
	default:
		return fmt.Errorf("busybar: unsupported timer settings type %q", s.Type)
	}
	return nil
}
