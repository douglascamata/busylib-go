package busybar_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/douglascamata/busylib-go/busybar"
)

func TestCustomNotificationAssets(t *testing.T) {
	for _, stock := range []bool{false, true} {
		for _, status := range []int{200, 409} {
			t.Run(map[bool]string{false: "upload", true: "stock"}[stock]+http.StatusText(status), func(t *testing.T) {
				iconAsset := busybar.NotificationAsset{Path: "icons/done.png"}
				soundAsset := busybar.NotificationAsset{Path: "sounds/ding.wav"}
				devicePath := "/ext/user_assets/laundry/icons/done.png"
				fixturePath := "testdata/notification-icon.png"
				if stock {
					iconAsset = busybar.NotificationAsset{StockPath: "shared/images/notification-icon.image"}
					soundAsset = busybar.NotificationAsset{StockPath: "shared/sounds/ding.snd"}
					devicePath = "/ext/apps_assets/shared/images/notification-icon.image"
					fixturePath = "testdata/notification-icon.image"
				}
				data, err := os.ReadFile(fixturePath)
				if err != nil {
					t.Fatal(err)
				}
				type call struct {
					path string
					body map[string]any
				}
				calls := make(chan call, 6)
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/version" {
						w.Write([]byte(`{"api_semver":"27.5.0"}`))
						return
					}
					if r.URL.Path == "/api/storage/read" {
						if r.Method != http.MethodGet || r.URL.Query().Get("path") != devicePath {
							t.Errorf("asset lookup %s %s, want %s", r.Method, r.URL, devicePath)
						}
						calls <- call{path: r.URL.Path}
						w.Write(data)
						return
					}
					if r.Method != http.MethodPost {
						t.Errorf("unexpected method %s", r.Method)
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					calls <- call{r.URL.Path, body}
					if r.URL.Path == "/api/display/draw" {
						w.WriteHeader(status)
					}
					w.Write([]byte(`{"result":"OK"}`))
				}))
				defer srv.Close()
				client, err := busybar.New(busybar.Config{Addr: srv.URL})
				if err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				icon, err := client.NotificationIconGet(ctx, "laundry", iconAsset)
				if err != nil || icon.Width != 16 || icon.Height != 10 {
					t.Fatalf("icon %+v, %v", icon, err)
				}
				if len(calls) != 1 {
					t.Fatalf("icon lookup made %d requests, want 1", len(calls))
				}
				if got := <-calls; got.path != "/api/storage/read" {
					t.Fatalf("lookup %+v", got)
				}
				for range 2 {
					err = client.Notify(ctx, "A long laundry notification", busybar.NotificationOptions{
						ApplicationName: "laundry", CustomIcon: icon, CustomSound: &soundAsset,
					})
					if (err != nil) != (status != 200) {
						t.Fatalf("Notify error %v", err)
					}
					if status != 200 {
						var httpErr *busybar.HTTPError
						if !errors.As(err, &httpErr) || httpErr.StatusCode != status {
							t.Fatalf("draw failure lost: %v", err)
						}
					}
					wantCalls := 1
					if status == 200 {
						wantCalls = 2
					}
					if len(calls) != wantCalls {
						t.Fatalf("Notify made %d requests, want %d", len(calls), wantCalls)
					}
					draw := <-calls
					if draw.path != "/api/display/draw" || draw.body["application_name"] != "laundry" {
						t.Fatalf("draw %+v", draw)
					}
					elements := draw.body["elements"].([]any)
					imageElement, textElement := elements[0].(map[string]any), elements[1].(map[string]any)
					field, absent := "path", "stock_path"
					wantIcon, wantSound := iconAsset.Path, soundAsset.Path
					if stock {
						field, absent = absent, field
						wantIcon, wantSound = iconAsset.StockPath, soundAsset.StockPath
					}
					if imageElement[field] != wantIcon || imageElement[absent] != nil || imageElement["x"] != float64(0) || imageElement["y"] != float64(8) {
						t.Fatalf("image %+v", imageElement)
					}
					// 16 pixels of image, a two-pixel gap, then 54 pixels of
					// scrolling text use the whole 72-pixel panel without overlap.
					if textElement["x"] != float64(18) || textElement["width"] != float64(54) || textElement["scroll_rate"] != float64(1200) {
						t.Fatalf("text overlaps or exceeds panel: %+v", textElement)
					}
					if status == 200 {
						sound := <-calls
						if sound.path != "/api/audio/play" || sound.body["application_name"] != "laundry" || sound.body[field] != wantSound || sound.body[absent] != nil {
							t.Fatalf("sound %+v", sound)
						}
					}
					if len(calls) != 0 {
						t.Fatal("unexpected lookup or audio request")
					}
				}
			})
		}
	}
}

func TestCustomNotificationRejectsUnusableAssets(t *testing.T) {
	icon := &busybar.NotificationIcon{Asset: busybar.NotificationAsset{Path: "done.png"}, Width: 16, Height: 16}
	for _, opts := range []busybar.NotificationOptions{
		{Icon: "check", CustomIcon: icon},
		{CustomIcon: &busybar.NotificationIcon{}},
		{CustomIcon: &busybar.NotificationIcon{Asset: icon.Asset, Width: 72, Height: 16}},
		{CustomIcon: &busybar.NotificationIcon{Asset: icon.Asset, Width: 16, Height: 17}},
		{CustomIcon: &busybar.NotificationIcon{Asset: busybar.NotificationAsset{StockPath: "busy/images/check.image"}, Width: 8, Height: 8}},
		{CustomSound: &busybar.NotificationAsset{}},
		{CustomSound: &busybar.NotificationAsset{Path: "ding.wav", StockPath: "shared/sounds/ding.snd"}},
		{CustomSound: &busybar.NotificationAsset{StockPath: "busy/sounds/ding.snd"}},
		{Sound: "event", CustomSound: &busybar.NotificationAsset{Path: "ding.wav"}},
	} {
		client, err := busybar.New(busybar.Config{HTTPClient: &http.Client{Transport: notificationTransport(func(*http.Request) (*http.Response, error) {
			t.Error("unusable asset reached the network")
			return nil, errors.New("unexpected request")
		})}})
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Notify(context.Background(), "text", opts); err == nil {
			t.Fatalf("accepted %+v", opts)
		}
	}
}

type notificationTransport func(*http.Request) (*http.Response, error)

func (f notificationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNotificationIconReadErrors(t *testing.T) {
	var wide bytes.Buffer
	if err := png.Encode(&wide, image.NewRGBA(image.Rect(0, 0, 72, 16))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		data   []byte
		status int
	}{
		{"missing", nil, 404},
		{"server error", nil, 503},
		{"wrong format", []byte("not an image"), 200},
		{"truncated PNG", []byte("\x89PNG\r\n\x1a\n"), 200},
		{"truncated LVGL", []byte{0x19, 0, 0, 0, 16, 0, 16, 0}, 200},
		{"no room for text", wide.Bytes(), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					w.Write([]byte(`{"api_semver":"27.5.0"}`))
					return
				}
				if r.URL.Query().Get("path") != "/ext/user_assets/busylib/done.png" {
					t.Errorf("default app path %s", r.URL)
				}
				w.WriteHeader(tc.status)
				w.Write(tc.data)
			}))
			defer srv.Close()
			client, err := busybar.New(busybar.Config{Addr: srv.URL})
			if err != nil {
				t.Fatal(err)
			}
			icon, err := client.NotificationIconGet(context.Background(), "", busybar.NotificationAsset{Path: "done.png"})
			if err == nil || icon != nil {
				t.Fatalf("icon %+v, error %v", icon, err)
			}
			if tc.status != 200 {
				var httpErr *busybar.HTTPError
				if !errors.As(err, &httpErr) || httpErr.StatusCode != tc.status {
					t.Fatalf("storage error lost: %v", err)
				}
			}
		})
	}
}
