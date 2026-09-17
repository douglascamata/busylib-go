package busybar

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"sync"
	"testing"
)

func TestDisplayElementsDeleteKeepsOtherElements(t *testing.T) {
	d := newFakeDevice(t)
	var mu sync.Mutex
	displayed := map[string]map[string]bool{
		"laundry": {"icon": true, "text": true},
		"clock":   {"icon": true},
	}
	d.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/display/draw" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		body, _ := io.ReadAll(r.Body)
		var ids []string
		if len(body) != 0 {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(body, &fields); err != nil {
				t.Error(err)
			}
			if _, ok := fields["application_name"]; ok {
				t.Error("application name must be in the query for firmware 1.2.4")
			}
			if err := json.Unmarshal(fields["element_ids"], &ids); err != nil {
				t.Error(err)
			}
		}
		mu.Lock()
		app := r.URL.Query().Get("application_name")
		if len(body) == 0 {
			delete(displayed, app)
		} else {
			for _, id := range ids {
				delete(displayed[app], id)
			}
		}
		mu.Unlock()
		writeJSON(w, 200, map[string]string{"result": "OK"})
		return true
	}
	c := d.client(t, Config{})
	ctx := context.Background()
	if err := c.DisplayElementsDelete(ctx, "laundry", []string{"icon"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	want := map[string]map[string]bool{"laundry": {"text": true}, "clock": {"icon": true}}
	if !reflect.DeepEqual(displayed, want) {
		t.Errorf("displayed = %v, want %v", displayed, want)
	}
	mu.Unlock()
	before := len(d.paths())
	if err := c.DisplayElementsDelete(ctx, "laundry", nil); err != nil {
		t.Fatal(err)
	}
	if len(d.paths()) != before {
		t.Fatal("empty selection sent a request")
	}
	if err := c.DisplayClear(ctx, "laundry"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(displayed, map[string]map[string]bool{"clock": {"icon": true}}) {
		t.Fatalf("clear changed another application: %v", displayed)
	}
}
