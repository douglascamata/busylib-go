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

// The firmware stores the last snapshot, and ignores non-newer writes even
// though HTTP reports OK. It does not return a live countdown from GET.
func newTimerDevice(t *testing.T, initial BusySnapshot) *fakeDevice {
	t.Helper()
	d := newFakeDevice(t)
	var mu sync.Mutex
	held := initial
	d.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/busy/snapshot" {
			return false
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, held)
		case http.MethodPut:
			var incoming BusySnapshot
			if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
				t.Error(err)
			}
			if incoming.SnapshotTimestampMs > held.SnapshotTimestampMs {
				held = incoming
			}
			writeJSON(w, 200, map[string]string{"result": "OK"})
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
		return true
	}
	return d
}

func TestTimerControlsPreserveElapsedTime(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		paused, pause, theme, future bool
	}{
		{name: "pause", pause: true},
		{name: "resume paused", paused: true},
		{name: "resume running"},
		{name: "theme", theme: true},
		{name: "paused theme", paused: true, theme: true},
		{name: "clock ahead", pause: true, future: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stamp := time.Now().Add(-time.Minute).UnixMilli()
			if tc.future {
				stamp = time.Now().Add(time.Minute).UnixMilli()
			}
			initial := BusySnapshot{SnapshotTimestampMs: stamp, Snapshot: BusySnapshotState{
				Type: BusySimple, CardID: "00000000-0000-0000-0000-000000000000",
				TimeLeftMs: 300000, IsPaused: tc.paused,
				BusyBarSettings: BusyBarSettings{Theme: "busy", TriggerSmartHome: true},
			}}
			c := newTimerDevice(t, initial).client(t, Config{})
			ctx := context.Background()
			var err error
			if tc.theme {
				err = c.BusySessionThemeSet(ctx, "meeting")
			} else {
				err = c.BusySetPaused(ctx, tc.pause)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.BusySnapshotGet(ctx)
			if err != nil {
				t.Fatal(err)
			}
			wantLeft := int64(300000)
			if !tc.paused {
				wantLeft -= got.SnapshotTimestampMs - stamp
			}
			wantPaused, wantTheme := tc.pause, "busy"
			if tc.theme {
				wantPaused, wantTheme = tc.paused, "meeting"
			}
			if got.SnapshotTimestampMs <= stamp || got.Snapshot.TimeLeftMs != wantLeft || got.Snapshot.IsPaused != wantPaused || got.Snapshot.BusyBarSettings.Theme != wantTheme {
				t.Fatalf("stored snapshot %+v, want left=%d paused=%v theme=%s", got, wantLeft, wantPaused, wantTheme)
			}
			if got.Snapshot.CardID != initial.Snapshot.CardID || !got.Snapshot.BusyBarSettings.TriggerSmartHome {
				t.Fatal("control lost unrelated session settings")
			}
		})
	}
}

func TestTimerControlsAcrossPhases(t *testing.T) {
	for _, tc := range []struct {
		name                string
		index               int
		left, total         int64
		auto, next, stop    bool
		wantIndex           int
		wantLeft, wantTotal int64
		wantPaused          bool
		wantType            BusyTimerType
	}{
		{"manual boundary", 0, 30000, 600000, false, false, false, 1, 300000, 300000, true, BusyInterval},
		{"skip manual", 0, 300000, 600000, false, true, false, 1, 300000, 300000, false, BusyInterval},
		{"skip after elapsed boundary", 0, 30000, 600000, true, true, false, 2, 600000, 600000, false, BusyInterval},
		{"skip final work", 4, 300000, 600000, false, true, false, 0, 0, 0, false, BusyNotStarted},
		{"stop", 0, 300000, 600000, true, false, true, 0, 0, 0, false, BusyNotStarted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initial := BusySnapshot{SnapshotTimestampMs: time.Now().Add(-time.Minute).UnixMilli(), Snapshot: BusySnapshotState{
				Type: BusyInterval, CardID: "00000000-0000-0000-0000-000000000000",
				CurrentInterval: tc.index, CurrentIntervalTimeTotalMs: tc.total, CurrentIntervalTimeLeftMs: tc.left,
				IntervalSettings: &BusyTimerSettings{Type: BusyInterval, IntervalWorkMs: 600000, IntervalRestMs: 300000, IntervalWorkCyclesCount: 3, IsAutostartEnabled: tc.auto},
				BusyBarSettings:  BusyBarSettings{Theme: "busy", ShowWorkPhaseOnly: true},
			}}
			c := newTimerDevice(t, initial).client(t, Config{})
			ctx := context.Background()
			var err error
			switch {
			case tc.stop:
				err = c.BusyStop(ctx)
			case tc.next:
				err = c.BusyNextPhase(ctx)
			default:
				err = c.BusySessionThemeSet(ctx, "busy")
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.BusySnapshotGet(ctx)
			if err != nil {
				t.Fatal(err)
			}
			s := got.Snapshot
			if s.Type != tc.wantType || s.CurrentInterval != tc.wantIndex || s.CurrentIntervalTimeLeftMs != tc.wantLeft || s.CurrentIntervalTimeTotalMs != tc.wantTotal || s.IsPaused != tc.wantPaused {
				t.Fatalf("stored state %+v", s)
			}
			if s.BusyBarSettings != initial.Snapshot.BusyBarSettings {
				t.Fatal("control lost display settings")
			}
		})
	}
}

func TestTimerThemePreservesExtendedPhase(t *testing.T) {
	initial := BusySnapshot{SnapshotTimestampMs: time.Now().Add(-time.Minute).UnixMilli(), Snapshot: BusySnapshotState{
		Type: BusyInterval, CardID: "00000000-0000-0000-0000-000000000000",
		CurrentIntervalTimeTotalMs: 900000, CurrentIntervalTimeLeftMs: 900000,
		IntervalSettings: &BusyTimerSettings{Type: BusyInterval, IntervalWorkMs: 600000, IntervalRestMs: 300000, IntervalWorkCyclesCount: 3},
		BusyBarSettings:  BusyBarSettings{Theme: "busy"},
	}}
	c := newTimerDevice(t, initial).client(t, Config{})
	ctx := context.Background()
	if err := c.BusySessionThemeSet(ctx, "meeting"); err != nil {
		t.Fatal(err)
	}
	got, err := c.BusySnapshotGet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Snapshot.CurrentIntervalTimeTotalMs != 900000 || got.Snapshot.CurrentIntervalTimeLeftMs != 900000-(got.SnapshotTimestampMs-initial.SnapshotTimestampMs) {
		t.Fatalf("extended phase lost time: %+v", got)
	}
}

func TestTimerControlsDoNotRestartFinishedSessions(t *testing.T) {
	for _, kind := range []BusyTimerType{BusyNotStarted, BusySimple, BusyInterval} {
		initial := BusySnapshot{SnapshotTimestampMs: time.Now().Add(-time.Minute).UnixMilli(), Snapshot: BusySnapshotState{
			Type: kind, TimeLeftMs: 1, CurrentInterval: 4, CurrentIntervalTimeLeftMs: 1,
			IntervalSettings: &BusyTimerSettings{Type: BusyInterval, IntervalWorkMs: 300000, IntervalRestMs: 300000, IntervalWorkCyclesCount: 3},
		}}
		d := newTimerDevice(t, initial)
		c := d.client(t, Config{})
		ctx := context.Background()
		for _, err := range []error{c.BusySetPaused(ctx, true), c.BusySetPaused(ctx, false), c.BusyNextPhase(ctx), c.BusySessionThemeSet(ctx, "meeting")} {
			if !errors.Is(err, ErrTimerNotRunning) {
				t.Fatalf("%s: %v", kind, err)
			}
		}
		for _, path := range d.paths() {
			if strings.HasPrefix(path, "PUT ") {
				t.Fatalf("finished session was rewritten: %v", d.paths())
			}
		}
	}
}

func TestTimerControlErrors(t *testing.T) {
	for _, fail := range []string{http.MethodGet, http.MethodPut} {
		d := newFakeDevice(t)
		d.handle = func(w http.ResponseWriter, r *http.Request) bool {
			if r.Method == fail {
				writeJSON(w, 503, map[string]string{"error": "offline"})
			} else {
				writeJSON(w, 200, BusySnapshot{Snapshot: BusySnapshotState{Type: BusyInfinite}})
			}
			return true
		}
		c := d.client(t, Config{})
		err := c.BusySetPaused(context.Background(), true)
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != 503 {
			t.Fatalf("%s failure lost: %v", fail, err)
		}
		if fail == http.MethodGet && !reflect.DeepEqual(d.paths(), []string{"GET /api/version", "GET /api/busy/snapshot"}) {
			t.Fatalf("write after failed read: %v", d.paths())
		}
	}
}

func TestSequentialTimerControlsUseNewerTimestamps(t *testing.T) {
	stamp := time.Now().Add(time.Minute).UnixMilli()
	c := newTimerDevice(t, BusySnapshot{SnapshotTimestampMs: stamp, Snapshot: BusySnapshotState{
		Type: BusyInfinite, BusyBarSettings: BusyBarSettings{Theme: "busy"},
	}}).client(t, Config{})
	ctx := context.Background()
	for _, paused := range []bool{true, false, true} {
		if err := c.BusySetPaused(ctx, paused); err != nil {
			t.Fatal(err)
		}
		got, err := c.BusySnapshotGet(ctx)
		if err != nil {
			t.Fatal(err)
		}
		stamp++
		if got.SnapshotTimestampMs != stamp || got.Snapshot.IsPaused != paused {
			t.Fatalf("write ignored: %+v, want timestamp %d, paused %v", got, stamp, paused)
		}
	}
}
