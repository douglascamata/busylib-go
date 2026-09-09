package busybar

import (
	"reflect"
	"testing"
	"time"
)

func TestTimerStateAt(t *testing.T) {
	// Firmware 1.2.3 busy_timer.c uses even indices for work, waits after
	// each phase without autostart, and stops after the final work phase.
	for _, tc := range []struct {
		name                 string
		index                int
		elapsed              int64
		auto, paused         bool
		wantIndex            int
		wantPhase            TimerPhase
		wantLeft             int64
		wantPaused, finished bool
	}{
		{"within work", 0, 10000, true, false, 0, TimerWork, 20000, false, false},
		{"exact boundary", 0, 30000, true, false, 1, TimerRest, 300000, false, false},
		{"into rest", 0, 60000, true, false, 1, TimerRest, 270000, false, false},
		{"multiple phases", 0, 1560000, true, false, 3, TimerRest, 270000, false, false},
		{"manual boundary", 0, 60000, false, false, 1, TimerRest, 300000, true, false},
		{"manual long absence", 0, 1560000, false, false, 1, TimerRest, 300000, true, false},
		{"manual rest to work", 1, 60000, false, false, 2, TimerWork, 1200000, true, false},
		{"paused", 1, 1560000, true, true, 1, TimerRest, 30000, true, false},
		{"last work", 4, 60000, true, false, 5, "", 0, false, true},
		{"last manual work", 4, 60000, false, false, 5, "", 0, false, true},
		{"clock ahead", 0, -10000, true, false, 0, TimerWork, 30000, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := BusyTimerSettings{Type: BusyInterval, IntervalWorkMs: 1200000, IntervalRestMs: 300000, IntervalWorkCyclesCount: 3, IsAutostartEnabled: tc.auto}
			snapshot := BusySnapshot{SnapshotTimestampMs: 1000000, Snapshot: BusySnapshotState{
				Type: BusyInterval, CurrentInterval: tc.index, CurrentIntervalTimeLeftMs: 30000,
				IntervalSettings: &settings, IsPaused: tc.paused,
			}}
			before := snapshot
			beforeSettings := settings
			got, err := snapshot.StateAt(time.UnixMilli(1000000 + tc.elapsed))
			if err != nil {
				t.Fatal(err)
			}
			if got.Interval != tc.wantIndex || got.Phase != tc.wantPhase || *got.TimeLeftMs != tc.wantLeft || got.IsPaused != tc.wantPaused || got.IsFinished != tc.finished || got.IsRunning() == tc.finished {
				t.Fatalf("state = %+v, remaining %d", got, *got.TimeLeftMs)
			}
			if !reflect.DeepEqual(snapshot, before) || !reflect.DeepEqual(*snapshot.Snapshot.IntervalSettings, beforeSettings) {
				t.Fatal("snapshot mutated")
			}
		})
	}
}

func TestTimerModesAndEqualDurations(t *testing.T) {
	for _, tc := range []struct {
		kind     BusyTimerType
		paused   bool
		left     *int64
		finished bool
	}{
		{BusyNotStarted, false, nil, false}, {BusyInfinite, false, nil, false},
		{BusySimple, false, new(int64(40000)), false}, {BusySimple, true, new(int64(60000)), false},
	} {
		s := BusySnapshot{SnapshotTimestampMs: 1000000, Snapshot: BusySnapshotState{Type: tc.kind, IsPaused: tc.paused, TimeLeftMs: 60000}}
		got, err := s.StateAt(time.UnixMilli(1020000))
		if err != nil || !reflect.DeepEqual(got.TimeLeftMs, tc.left) || got.IsFinished != tc.finished {
			t.Fatalf("%s: %+v, %v", tc.kind, got, err)
		}
		if tc.paused && got.ElapsedMs != 0 {
			t.Fatal("paused time advanced")
		}
	}
	s := BusySnapshot{Snapshot: BusySnapshotState{Type: BusySimple, TimeLeftMs: 60000}}
	done, err := s.StateAt(time.UnixMilli(90000))
	if err != nil || !done.IsFinished || done.IsRunning() || *done.TimeLeftMs != 0 {
		t.Fatalf("finished: %+v %v", done, err)
	}
	s.Snapshot = BusySnapshotState{Type: BusyInterval, CurrentInterval: 1, CurrentIntervalTimeLeftMs: 60000, IntervalSettings: &BusyTimerSettings{IntervalWorkMs: 1500000, IntervalRestMs: 1500000, IntervalWorkCyclesCount: 3, IsAutostartEnabled: true}}
	got, err := s.StateAt(time.UnixMilli(10000))
	if err != nil || got.Phase != TimerRest || *got.TimeLeftMs != 50000 {
		t.Fatalf("equal durations: %+v %v", got, err)
	}
}

func TestTimerStateRejectsUnusableSnapshots(t *testing.T) {
	for _, s := range []BusySnapshotState{{}, {Type: BusyInterval}, {Type: BusyInterval, IntervalSettings: &BusyTimerSettings{}}} {
		if _, err := (BusySnapshot{Snapshot: s}).StateAt(time.Now()); err == nil {
			t.Fatal("unusable snapshot accepted")
		}
	}
}
