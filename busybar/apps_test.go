package busybar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"testing"
)

func TestAppsLifecycle(t *testing.T) {
	d := newFakeDevice(t)
	d.semver = "27.14.0"
	// Independent wire fixtures follow firmware 1.3.1's apps.yaml and api_apps.c.
	const appJSON = `{"id":"app.test","name":"Test","version":"1.0.0","api_version":"27.9.0","author":"Tester","description":"Test app","icon_path":"/ext/user_assets/app.test/appmeta/icon.png","is_debug":true}`
	const appID = "app.test"
	packageBytes := []byte{0x1f, 0x8b, 0, 0xff, 0x80, 0x01}
	var installed, running bool
	var key uint32
	nextKey := uint32(4294967295)
	d.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("X-API-Token") != "local-secret" {
			t.Error("missing local credential")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /api/apps/stage":
			data, err := io.ReadAll(r.Body)
			if err != nil || !bytes.Equal(data, packageBytes) || r.ContentLength != int64(len(packageBytes)) || r.Header.Get("Content-Type") != "application/octet-stream" {
				t.Errorf("upload changed bytes or headers: %x, length %d, %v", data, r.ContentLength, err)
			}
			key = nextKey
			nextKey--
			replacement := ""
			if installed {
				replacement = `,"installed":` + appJSON
			}
			io.WriteString(w, `{"result":"OK","install_key":`+strconv.FormatUint(uint64(key), 10)+`,"staged":`+appJSON+replacement+`}`)
		case "POST /api/apps/install":
			if key == 0 || r.URL.Query().Get("install_key") != strconv.FormatUint(uint64(key), 10) {
				writeJSON(w, 400, map[string]string{"error": "Invalid installation key"})
				break
			}
			installed, key = true, 0
			io.WriteString(w, `{"result":"OK"}`)
		case "GET /api/apps/list":
			if installed {
				io.WriteString(w, `{"apps":[`+appJSON+`]}`)
			} else {
				io.WriteString(w, `{"apps":[]}`)
			}
		case "POST /api/apps/launch", "DELETE /api/apps":
			if r.URL.Query().Get("app_id") != appID || !installed {
				writeJSON(w, 404, map[string]string{"error": "App not found"})
				break
			}
			if r.Method == http.MethodDelete {
				installed, running = false, false
			} else {
				running = true
			}
			io.WriteString(w, `{"result":"OK"}`)
		case "POST /api/apps/quit":
			if !running {
				writeJSON(w, 409, map[string]string{"error": "application is not running"})
				break
			}
			running = false
			io.WriteString(w, `{"result":"OK"}`)
		default:
			return false
		}
		return true
	}
	c := d.client(t, Config{HTTPAccessPassword: "local-secret"})
	ctx := context.Background()
	list, err := c.AppsList(ctx)
	if err != nil || len(list.Apps) != 0 {
		t.Fatalf("initial list: %+v, %v", list, err)
	}
	first, err := c.AppsStage(ctx, packageBytes)
	if err != nil || first.InstallKey != 4294967295 || first.Installed != nil {
		t.Fatalf("first stage: %+v, %v", first, err)
	}
	second, err := c.AppsStage(ctx, packageBytes)
	if err != nil || second.InstallKey != 4294967294 {
		t.Fatalf("second stage: %+v, %v", second, err)
	}
	var httpErr *HTTPError
	if err := c.AppsInstall(ctx, first.InstallKey); !errors.As(err, &httpErr) || httpErr.StatusCode != 400 {
		t.Fatalf("replaced key: %v", err)
	}
	if err := c.AppsInstall(ctx, second.InstallKey); err != nil {
		t.Fatal(err)
	}
	if err := c.AppsInstall(ctx, second.InstallKey); !errors.As(err, &httpErr) || httpErr.StatusCode != 400 {
		t.Fatalf("consumed key: %v", err)
	}
	list, err = c.AppsList(ctx)
	if err != nil || len(list.Apps) != 1 {
		t.Fatalf("installed list: %+v, %v", list, err)
	}
	app := list.Apps[0]
	if app.ID != appID || app.Name != "Test" || app.Version != "1.0.0" || app.APIVersion != "27.9.0" || app.Author != "Tester" || app.Description != "Test app" || app.IconPath != "/ext/user_assets/app.test/appmeta/icon.png" || !app.IsDebug {
		t.Fatalf("app fields lost: %+v", app)
	}
	replacement, err := c.AppsStage(ctx, packageBytes)
	if err != nil || replacement.Installed == nil || *replacement.Installed != app || replacement.Staged != app {
		t.Fatalf("replacement details: %+v, %v", replacement, err)
	}
	if err := c.AppsLaunch(ctx, appID); err != nil {
		t.Fatal(err)
	}
	if err := c.AppsQuit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.AppsQuit(ctx); !errors.As(err, &httpErr) || httpErr.StatusCode != 409 || httpErr.Message != "application is not running" {
		t.Fatalf("idle quit: %v", err)
	}
	if err := c.AppsRemove(ctx, appID); err != nil {
		t.Fatal(err)
	}
	list, err = c.AppsList(ctx)
	if err != nil || len(list.Apps) != 0 {
		t.Fatalf("removed list: %+v, %v", list, err)
	}
}

func TestAppsSettingsRoundTrip(t *testing.T) {
	d := newFakeDevice(t)
	d.semver = "27.14.0"
	const defaults = `{"version":3,"values":{"enabled":true,"count":7,"group":{"label":"default"}}}`
	stored := []byte(defaults)
	d.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/apps/settings" {
			return false
		}
		if r.URL.Query().Get("app_id") != "app.test" {
			t.Errorf("wrong settings query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			w.Write(stored)
		case http.MethodPut:
			data, err := io.ReadAll(r.Body)
			if err != nil || r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("settings request: %v, %s", err, r.Header.Get("Content-Type"))
			}
			var body struct {
				Version int                        `json:"version"`
				Values  map[string]json.RawMessage `json:"values"`
			}
			if json.Unmarshal(data, &body) != nil || body.Version != 3 || len(body.Values) != 3 {
				writeJSON(w, 400, map[string]string{"error": "Invalid settings document"})
				break
			}
			stored = data
			io.WriteString(w, `{"result":"OK"}`)
		case http.MethodDelete:
			stored = []byte(defaults)
			io.WriteString(w, `{"result":"OK"}`)
		default:
			t.Errorf("wrong settings method: %s", r.Method)
		}
		return true
	}
	c := d.client(t, Config{})
	ctx := context.Background()
	document, err := c.AppsSettingsGet(ctx, "app.test")
	if err != nil {
		t.Fatal(err)
	}
	document.Values["enabled"] = json.RawMessage(`false`)
	document.Values["count"] = json.RawMessage(`0`)
	document.Values["group"] = json.RawMessage(`{"label":"changed","number":9007199254740993}`)
	if err := c.AppsSettingsSet(ctx, "app.test", *document); err != nil {
		t.Fatal(err)
	}
	got, err := c.AppsSettingsGet(ctx, "app.test")
	if err != nil || got.Version != 3 || string(got.Values["enabled"]) != "false" || string(got.Values["count"]) != "0" || string(got.Values["group"]) != `{"label":"changed","number":9007199254740993}` {
		t.Fatalf("settings values changed: %+v, %v", got, err)
	}
	var httpErr *HTTPError
	if err := c.AppsSettingsSet(ctx, "app.test", AppSettingsDocument{Version: 2, Values: document.Values}); !errors.As(err, &httpErr) || httpErr.StatusCode != 400 {
		t.Fatalf("schema version error: %v", err)
	}
	if err := c.AppsSettingsReset(ctx, "app.test"); err != nil {
		t.Fatal(err)
	}
	got, err = c.AppsSettingsGet(ctx, "app.test")
	if err != nil || string(got.Values["enabled"]) != "true" || string(got.Values["count"]) != "7" || string(got.Values["group"]) != `{"label":"default"}` {
		t.Fatalf("reset: %+v, %v", got, err)
	}
}

func TestAppsStagePreservesServerError(t *testing.T) {
	d := newFakeDevice(t)
	d.semver = "27.14.0"
	const body = `{"error":"API version mismatch","error_code":"api_mismatch"}`
	d.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost || r.URL.Path != "/api/apps/stage" {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400) // Released handler returns 400, although OpenAPI lists 409.
		io.WriteString(w, body)
		return true
	}
	c := d.client(t, Config{})
	_, err := c.AppsStage(context.Background(), []byte("package"))
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 400 || httpErr.Message != "API version mismatch" || string(httpErr.Body) != body {
		t.Fatalf("stage error lost: %v", err)
	}
}
