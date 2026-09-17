package busybar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const testCardID = "12345678-abcd-1234-abcd-123456789abc"

func newProfileDevice(t *testing.T, profile BusyProfile, initial BusySnapshot) *fakeDevice {
	t.Helper()
	d := newTimerDevice(t, initial)
	timerHandler := d.handle
	var mu sync.Mutex
	d.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/busy/profiles/custom" && r.URL.Path != "/api/busy/profiles/busy" {
			return timerHandler(w, r)
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, profile)
		case http.MethodPut:
			var incoming BusyProfile
			if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
				t.Error(err)
			}
			if incoming.ProfileTimestampMs > profile.ProfileTimestampMs {
				profile = incoming
			}
			writeJSON(w, 200, map[string]string{"result": "OK"})
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
		return true
	}
	return d
}

func TestProfileEditingAndSessionOverrides(t *testing.T) {
	ctx := context.Background()
	profile := BusyProfile{ID: testCardID, Title: "Focus", SortOrder: 7,
		TimerSettings:      BusyTimerSettings{Type: BusyInfinite},
		BusyBarSettings:    BusyBarSettings{Theme: "busy", ShowWorkPhaseOnly: true, TriggerSmartHome: true},
		ProfileTimestampMs: time.Now().Add(time.Minute).UnixMilli(),
	}
	initial := BusySnapshot{SnapshotTimestampMs: time.Now().Add(time.Minute).UnixMilli(), Snapshot: BusySnapshotState{
		Type: BusyInfinite, CardID: testCardID, BusyBarSettings: profile.BusyBarSettings,
	}}
	d := newProfileDevice(t, profile, initial)
	c := d.client(t, Config{})
	// Changing kinds uses explicit settings, including false and zero values.
	for _, settings := range []BusyTimerSettings{
		{Type: BusyInterval, IntervalWorkMs: 300000, IntervalRestMs: 28800000, IntervalWorkCyclesCount: 35, IsAutostartEnabled: false},
		{Type: BusySimple, TotalTimeMs: 0},
		{Type: BusyInfinite},
	} {
		written, err := c.BusyProfileUpdate(ctx, BusySlotCustom, func(p *BusyProfile) error {
			p.TimerSettings = settings
			p.BusyBarSettings.Theme = "meeting"
			p.BusyBarSettings.TriggerSmartHome = false
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		stored, err := c.BusyProfileGet(ctx, BusySlotCustom)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(written, stored) || stored.TimerSettings != settings || stored.ProfileTimestampMs <= profile.ProfileTimestampMs {
			t.Fatalf("profile write not preserved: %+v, want %+v", stored, written)
		}
		if stored.Title != "Focus" || stored.SortOrder != 7 || stored.ID != testCardID || !stored.BusyBarSettings.ShowWorkPhaseOnly || stored.BusyBarSettings.TriggerSmartHome {
			t.Fatalf("profile edit lost fields: %+v", stored)
		}
		profile = *stored
	}
	unchanged, err := c.BusySnapshotGet(ctx)
	if err != nil || !reflect.DeepEqual(*unchanged, initial) {
		t.Fatalf("profile edit changed the session: %+v, %v", unchanged, err)
	}

	// A temporary countdown and theme override must never update the profile.
	timer := BusyTimerSettings{Type: BusySimple, TotalTimeMs: 120000}
	display := BusyBarSettings{Theme: "dnd", ShowWorkPhaseOnly: false, TriggerSmartHome: false}
	before := len(d.paths())
	if err := c.BusyStart(ctx, BusyStartParams{Slot: BusySlotCustom, TimerSettings: &timer, BusyBarSettings: &display}); err != nil {
		t.Fatal(err)
	}
	session, err := c.BusySnapshotGet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if session.Snapshot.Type != BusySimple || session.Snapshot.TimeLeftMs != 120000 || session.Snapshot.BusyBarSettings != display || session.Snapshot.CardID != profile.ID || session.SnapshotTimestampMs <= initial.SnapshotTimestampMs {
		t.Fatalf("session overrides lost: %+v", session)
	}
	for _, path := range d.paths()[before:] {
		if strings.HasPrefix(path, "PUT /api/busy/profiles/") {
			t.Fatal("temporary start wrote a profile")
		}
	}
	stored, err := c.BusyProfileGet(ctx, BusySlotCustom)
	if err != nil || !reflect.DeepEqual(*stored, profile) {
		t.Fatalf("temporary settings changed the profile: %+v, %v", stored, err)
	}
}

func TestBusyStartSettingsSources(t *testing.T) {
	for _, kind := range []BusyTimerType{BusyInfinite, BusySimple, BusyInterval} {
		for _, explicit := range []bool{false, true} {
			t.Run(string(kind)+map[bool]string{false: "/profile", true: "/explicit"}[explicit], func(t *testing.T) {
				timer := BusyTimerSettings{Type: kind}
				switch kind {
				case BusySimple:
					timer.TotalTimeMs = 86400000
				case BusyInterval:
					timer.IntervalWorkMs, timer.IntervalRestMs, timer.IntervalWorkCyclesCount = 300000, 300000, 2
				}
				display := BusyBarSettings{Theme: "busy", TriggerSmartHome: true}
				d := newProfileDevice(t, BusyProfile{ID: testCardID, TimerSettings: timer, BusyBarSettings: display},
					BusySnapshot{Snapshot: BusySnapshotState{Type: BusyNotStarted}})
				c := d.client(t, Config{})
				params := BusyStartParams{}
				card := testCardID
				if explicit {
					card = "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"
					params = BusyStartParams{CardID: card, TimerSettings: &timer, BusyBarSettings: &display}
				}
				ctx := context.Background()
				if err := c.BusyStart(ctx, params); err != nil {
					t.Fatal(err)
				}
				got, err := c.BusySnapshotGet(ctx)
				if err != nil {
					t.Fatal(err)
				}
				s := got.Snapshot
				if s.Type != kind || s.CardID != card || s.IsPaused || s.BusyBarSettings != display || got.SnapshotTimestampMs == 0 {
					t.Fatalf("session %+v", got)
				}
				if kind == BusySimple && s.TimeLeftMs != 86400000 {
					t.Fatalf("countdown %+v", s)
				}
				if kind == BusyInterval && (s.CurrentInterval != 0 || s.CurrentIntervalTimeLeftMs != 300000 || s.CurrentIntervalTimeTotalMs != 300000 || !reflect.DeepEqual(s.IntervalSettings, &timer)) {
					t.Fatalf("interval %+v", s)
				}
				for _, path := range d.paths() {
					if explicit && strings.Contains(path, "/busy/profiles/") {
						t.Fatal("explicit card start accessed a profile")
					}
				}
			})
		}
	}
}

func TestTimerSettingsRejectedBeforeWriting(t *testing.T) {
	valid := BusyTimerSettings{Type: BusyInterval, IntervalWorkMs: 300000, IntervalRestMs: 300000, IntervalWorkCyclesCount: 2}
	for _, change := range []func(*BusyTimerSettings){
		func(s *BusyTimerSettings) { s.IntervalWorkMs-- },
		func(s *BusyTimerSettings) { s.IntervalRestMs = 28800001 },
		func(s *BusyTimerSettings) { s.IntervalWorkCyclesCount = 1 },
		func(s *BusyTimerSettings) { s.IntervalWorkCyclesCount = 36 },
		func(s *BusyTimerSettings) { *s = BusyTimerSettings{Type: BusySimple, TotalTimeMs: -1} },
		func(s *BusyTimerSettings) { *s = BusyTimerSettings{Type: BusySimple, TotalTimeMs: 86400001} },
		func(s *BusyTimerSettings) { s.Type = BusyNotStarted },
	} {
		timer := valid
		change(&timer)
		display := BusyBarSettings{Theme: "busy"}
		d := newProfileDevice(t, BusyProfile{ID: testCardID, TimerSettings: valid, BusyBarSettings: display}, BusySnapshot{Snapshot: BusySnapshotState{Type: BusyNotStarted}})
		c := d.client(t, Config{})
		ctx := context.Background()
		if err := c.BusyStart(ctx, BusyStartParams{CardID: testCardID, TimerSettings: &timer, BusyBarSettings: &display}); err == nil {
			t.Fatalf("start accepted invalid timer %+v", timer)
		}
		if _, err := c.BusyProfileUpdate(ctx, BusySlotBusy, func(p *BusyProfile) error { p.TimerSettings = timer; return nil }); err == nil {
			t.Fatalf("profile accepted invalid timer %+v", timer)
		}
		for _, path := range d.paths() {
			if strings.HasPrefix(path, "PUT ") {
				t.Fatal("invalid settings reached the device")
			}
		}
	}
}

func TestProfileEditErrorAndInvalidCard(t *testing.T) {
	d := newProfileDevice(t, BusyProfile{ID: testCardID, TimerSettings: BusyTimerSettings{Type: BusyInfinite}}, BusySnapshot{Snapshot: BusySnapshotState{Type: BusyNotStarted}})
	c := d.client(t, Config{})
	ctx := context.Background()
	editErr := errors.New("edit cancelled")
	if _, err := c.BusyProfileUpdate(ctx, BusySlotBusy, func(p *BusyProfile) error { p.Title = "unsaved"; return editErr }); !errors.Is(err, editErr) {
		t.Fatalf("edit error lost: %v", err)
	}
	if _, err := c.BusyProfileUpdate(ctx, BusySlotBusy, func(p *BusyProfile) error { p.ID = "bad"; return nil }); err == nil {
		t.Fatal("invalid profile ID accepted")
	}
	timer, display := BusyTimerSettings{Type: BusyInfinite}, BusyBarSettings{Theme: "busy"}
	for _, params := range []BusyStartParams{
		{Slot: BusySlotBusy, CardID: testCardID, TimerSettings: &timer, BusyBarSettings: &display},
		{CardID: testCardID},
		{CardID: "bad", TimerSettings: &timer, BusyBarSettings: &display},
	} {
		if err := c.BusyStart(ctx, params); err == nil {
			t.Fatalf("invalid start accepted: %+v", params)
		}
	}
	for _, path := range d.paths() {
		if strings.HasPrefix(path, "PUT ") {
			t.Fatal("invalid edit reached the device")
		}
	}
}
