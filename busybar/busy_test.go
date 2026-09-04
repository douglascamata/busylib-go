package busybar

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

// Expected fields follow the timer unions in the pinned busylib-ts schema.
func TestBusyTimerWireFields(t *testing.T) {
	for _, tc := range []struct {
		name     string
		kind     BusyTimerType
		snapshot bool
		want     string
	}{
		{"infinite profile", BusyInfinite, false, `{"type":"INFINITE"}`},
		{"simple profile", BusySimple, false, `{"type":"SIMPLE","total_time_ms":0}`},
		{"interval profile", BusyInterval, false, `{"type":"INTERVAL","interval_work_ms":0,"interval_rest_ms":0,"interval_work_cycles_count":0,"is_autostart_enabled":false}`},
		{"not started snapshot", BusyNotStarted, true, `{"type":"NOT_STARTED","busy_bar_settings":{"theme":"","show_work_phase_only":false,"trigger_smart_home":false}}`},
		{"infinite snapshot", BusyInfinite, true, `{"type":"INFINITE","card_id":"card","is_paused":false,"busy_bar_settings":{"theme":"","show_work_phase_only":false,"trigger_smart_home":false}}`},
		{"simple snapshot", BusySimple, true, `{"type":"SIMPLE","card_id":"card","is_paused":false,"time_left_ms":0,"busy_bar_settings":{"theme":"","show_work_phase_only":false,"trigger_smart_home":false}}`},
		{"interval snapshot", BusyInterval, true, `{"type":"INTERVAL","card_id":"card","is_paused":false,"current_interval":0,"current_interval_time_total_ms":0,"current_interval_time_left_ms":0,"interval_settings":{"type":"INTERVAL","interval_work_ms":0,"interval_rest_ms":0,"interval_work_cycles_count":0,"is_autostart_enabled":false},"busy_bar_settings":{"theme":"","show_work_phase_only":false,"trigger_smart_home":false}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newFakeDevice(t)
			body := make(chan map[string]any, 1)
			d.handle = func(w http.ResponseWriter, r *http.Request) bool {
				var got map[string]any
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Error(err)
				}
				body <- got
				writeJSON(w, 200, map[string]string{"result": "OK"})
				return true
			}
			c := d.client(t, Config{})
			field := "timer_settings"
			var err error
			if tc.snapshot {
				field = "snapshot"
				err = c.BusySnapshotSet(context.Background(), BusySnapshot{Snapshot: BusySnapshotState{
					Type: tc.kind, CardID: "card", IntervalSettings: &BusyTimerSettings{Type: BusyInterval},
				}})
			} else {
				err = c.BusyProfileSet(context.Background(), BusySlotBusy, BusyProfile{TimerSettings: BusyTimerSettings{Type: tc.kind}})
			}
			if err != nil {
				t.Fatal(err)
			}
			var want any
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if got := (<-body)[field]; !reflect.DeepEqual(got, want) {
				t.Errorf("wire fields = %#v; want %#v", got, want)
			}
		})
	}
}
