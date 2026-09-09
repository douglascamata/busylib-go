package busybar

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
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

func TestSnapshotReadWritePreservesSettings(t *testing.T) {
	// Every firmware snapshot variant carries busy_bar_settings inside snapshot.
	for _, variant := range []string{
		`"type":"NOT_STARTED"`,
		`"type":"INFINITE","card_id":"00000000-0000-0000-0000-000000000000","is_paused":false`,
		`"type":"SIMPLE","card_id":"00000000-0000-0000-0000-000000000000","is_paused":false,"time_left_ms":0`,
		`"type":"INTERVAL","card_id":"00000000-0000-0000-0000-000000000000","is_paused":false,"current_interval":0,"current_interval_time_total_ms":300000,"current_interval_time_left_ms":0,"interval_settings":{"type":"INTERVAL","interval_work_ms":300000,"interval_rest_ms":300000,"interval_work_cycles_count":3,"is_autostart_enabled":false}`,
	} {
		fixture := `{"snapshot_timestamp_ms":1000000,"snapshot":{` + variant + `,"busy_bar_settings":{"theme":"busy","show_work_phase_only":false,"trigger_smart_home":true}}}`
		d := newFakeDevice(t)
		stored := make(chan map[string]any, 1)
		d.handle = func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path != "/api/busy/snapshot" {
				t.Errorf("unexpected path %s", r.URL.Path)
			}
			if r.Method == http.MethodGet {
				w.Write([]byte(fixture))
				return true
			}
			if r.Method != http.MethodPut {
				t.Errorf("unexpected method %s", r.Method)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			stored <- body
			writeJSON(w, 200, map[string]string{"result": "OK"})
			return true
		}
		c := d.client(t, Config{})
		snapshot, err := c.BusySnapshotGet(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err := c.BusySnapshotSet(context.Background(), *snapshot); err != nil {
			t.Fatal(err)
		}
		var want map[string]any
		if err := json.NewDecoder(strings.NewReader(fixture)).Decode(&want); err != nil {
			t.Fatal(err)
		}
		if got := <-stored; !reflect.DeepEqual(got, want) {
			t.Fatalf("round trip lost fields: %#v; want %#v", got, want)
		}
	}
}
