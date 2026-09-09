package busybar_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/douglascamata/busylib-go/busybar"
)

func TestBuildLayouts(t *testing.T) {
	// Reference offsets and asset widths: busylib-py 2.1.0 notification.py.
	for _, tc := range []struct {
		name, text        string
		opts              busybar.NotificationOptions
		x, y, width, rate int
	}{
		{"plain small", "Ready", busybar.NotificationOptions{}, 2, 7, 0, 0},
		{"tiny centered", "Ready", busybar.NotificationOptions{Font: busybar.FontTiny}, 2, 8, 0, 0},
		{"extra large centered", "Ready", busybar.NotificationOptions{Font: busybar.FontExtraLarge}, 2, 8, 0, 0},
		{"narrow icon", "Ready", busybar.NotificationOptions{Icon: "clock"}, 7, 7, 0, 0},
		{"wide icon scroll", "A long notification", busybar.NotificationOptions{Icon: "start"}, 13, 7, 59, 1200},
		{"unicode counts characters", "éééééééé", busybar.NotificationOptions{}, 2, 7, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params, err := busybar.BuildNotification(tc.text, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			text := params.Elements[len(params.Elements)-1].(busybar.TextElement)
			if text.X != tc.x || text.Y != tc.y || text.Width != tc.width || text.ScrollRate != tc.rate || text.Align != busybar.AlignMidLeft {
				t.Fatalf("text %+v", text)
			}
			if tc.opts.Icon != "" {
				icon := params.Elements[0].(busybar.ImageElement)
				if icon.Y != 8 || icon.Align != busybar.AlignMidLeft || icon.StockPath == "" {
					t.Fatalf("icon %+v", icon)
				}
			}
		})
	}
	for _, tc := range []struct {
		font        busybar.Font
		top, bottom int
	}{
		{busybar.FontTiny, 1, 15}, {busybar.FontSmall, 0, 16},
		{busybar.FontNormal, -1, 17}, {busybar.FontCondensed, -1, 17}, {busybar.FontBold, -1, 17},
	} {
		params, err := busybar.BuildNotification("First", busybar.NotificationOptions{Line2: "Second", Font: tc.font, Duration: 5, Color: "#FF0000FF", Line2Color: "#00FF00FF"})
		if err != nil {
			t.Fatal(err)
		}
		first, second := params.Elements[0].(busybar.TextElement), params.Elements[1].(busybar.TextElement)
		if first.Y != tc.top || first.Align != busybar.AlignTopLeft || second.Y != tc.bottom || second.Align != busybar.AlignBottomLeft || first.Timeout != 5 || second.Timeout != 5 || first.Color != "#FF0000FF" || second.Color != "#00FF00FF" {
			t.Fatalf("%s: %+v %+v", tc.font, first, second)
		}
	}
}

func TestBuildRejectsInvalidLayout(t *testing.T) {
	for _, opts := range []busybar.NotificationOptions{
		{Font: busybar.FontGlobal}, {Font: busybar.FontLarge, Line2: "Second"},
		{Icon: "missing"}, {Duration: -1}, {Priority: 101}, {Priority: -1},
	} {
		if _, err := busybar.BuildNotification("text", opts); err == nil {
			t.Fatalf("accepted %+v", opts)
		}
	}
}

func TestNotifyWireAndFailureOrdering(t *testing.T) {
	for _, tc := range []struct {
		name, version, sound               string
		drawStatus, audioStatus, wantCalls int
		wantErr                            bool
	}{
		{"draw then sound", "27.5.0", "event", 200, 200, 2, false},
		{"draw only", "24.3.0", "", 200, 200, 1, false},
		{"draw rejected", "27.5.0", "event", 409, 200, 1, true},
		{"sound rejected", "27.5.0", "event", 200, 404, 2, true},
		{"old firmware", "24.2.0", "event", 200, 200, 0, true},
		{"unknown sound", "27.5.0", "missing", 200, 200, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type call struct {
				path string
				body map[string]any
			}
			calls := make(chan call, 4)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/version" {
					json.NewEncoder(w).Encode(map[string]string{"api_semver": tc.version})
					return
				}
				if r.Method != http.MethodPost {
					t.Errorf("method %s", r.Method)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				calls <- call{r.URL.Path, body}
				switch r.URL.Path {
				case "/api/display/draw":
					w.WriteHeader(tc.drawStatus)
				case "/api/audio/play":
					w.WriteHeader(tc.audioStatus)
				default:
					t.Errorf("path %s", r.URL.Path)
					w.WriteHeader(404)
				}
				w.Write([]byte(`{"result":"OK"}`))
			}))
			defer srv.Close()
			client, err := busybar.New(busybar.Config{Addr: srv.URL})
			if err != nil {
				t.Fatal(err)
			}
			err = client.Notify(context.Background(), "Laundry done", busybar.NotificationOptions{
				Line2: "Ready", Icon: "check", Sound: tc.sound, BackgroundColor: "#0000FFFF", Duration: 10,
				Priority: busybar.NotificationPriorityInterrupt, ApplicationName: "laundry",
			})
			if (err != nil) != tc.wantErr {
				t.Fatalf("error %v", err)
			}
			if len(calls) != tc.wantCalls {
				t.Fatalf("%d calls, want %d", len(calls), tc.wantCalls)
			}
			if tc.wantCalls == 0 {
				return
			}
			draw := <-calls
			if draw.path != "/api/display/draw" || draw.body["application_name"] != "laundry" || draw.body["priority"] != float64(91) {
				t.Fatalf("draw %+v", draw)
			}
			elements := draw.body["elements"].([]any)
			background := elements[0].(map[string]any)
			if background["id"] != "0" || background["type"] != "rectangle" || background["width"] != float64(72) || background["height"] != float64(16) || background["border_width"] != float64(0) {
				t.Fatalf("background %v", background)
			}
			icon := elements[1].(map[string]any)
			if icon["stock_path"] != "shared/images/checkmark_front_8x8.image" {
				t.Fatalf("icon %v", icon)
			}
			for _, e := range elements {
				if e.(map[string]any)["timeout"] != float64(10) {
					t.Fatal("unequal element lifetimes")
				}
			}
			if tc.wantCalls == 2 {
				audio := <-calls
				if audio.path != "/api/audio/play" || audio.body["application_name"] != "laundry" || audio.body["stock_path"] != "shared/sounds/calendar_event_starts.snd" {
					t.Fatalf("audio %+v", audio)
				}
			}
			if tc.drawStatus != 200 || tc.audioStatus != 200 {
				var httpErr *busybar.HTTPError
				if !errors.As(err, &httpErr) {
					t.Fatalf("HTTP error lost: %v", err)
				}
			}
		})
	}
}
