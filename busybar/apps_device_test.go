package busybar

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestAppsDeviceLifecycle is opt-in because it installs and launches a real app.
// It refuses to overwrite the test ID or its files and cleans up on failure too.
func TestAppsDeviceLifecycle(t *testing.T) {
	addr := os.Getenv("BUSYBAR_DEVICE_URL")
	if addr == "" {
		t.Skip("set BUSYBAR_DEVICE_URL to test app management on a physical bar")
	}
	c, err := New(Config{Addr: addr, HTTPAccessPassword: os.Getenv("BUSYBAR_DEVICE_TOKEN")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	const appID = "app.busylib.go_test"
	const settingsPath = "/ext/apps_data/jsrunner/" + appID + ".settings.json"
	const markerPath = "/ext/apps_data/jsrunner/" + appID + ".localstorage.json"
	locations := map[string][]string{
		"/ext/user_assets":        {appID},
		"/ext/apps_data/jsrunner": {appID + ".settings.json", appID + ".localstorage.json"},
	}
	before, err := c.AppsList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("device API %s; %d apps installed before test", c.APISemver(), len(before.Apps))
	for directory, names := range locations {
		entries, err := c.StorageListGet(ctx, directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries.List {
			if slices.Contains(names, entry.Name) {
				t.Fatalf("refusing to overwrite existing test files: %s/%s", directory, entry.Name)
			}
		}
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		var httpErr *HTTPError
		if err := c.AppsRemove(cleanupCtx, appID); err != nil && (!errors.As(err, &httpErr) || httpErr.StatusCode != 404) {
			t.Errorf("remove test app: %v", err)
			return
		}
		for directory, names := range locations {
			entries, err := c.StorageListGet(cleanupCtx, directory)
			if err != nil {
				t.Error(err)
				continue
			}
			for _, entry := range entries.List {
				if slices.Contains(names, entry.Name) {
					if err := c.StorageRemove(cleanupCtx, directory+"/"+entry.Name); err != nil {
						t.Errorf("remove test files: %v", err)
					}
				}
			}
			entries, err = c.StorageListGet(cleanupCtx, directory)
			if err != nil {
				t.Error(err)
				continue
			}
			for _, entry := range entries.List {
				if slices.Contains(names, entry.Name) {
					t.Errorf("test file remains: %s/%s", directory, entry.Name)
				}
			}
		}
		after, err := c.AppsList(cleanupCtx)
		if err != nil {
			t.Error(err)
			return
		}
		byID := func(a, b AppInfo) int { return strings.Compare(a.ID, b.ID) }
		slices.SortFunc(before.Apps, byID)
		slices.SortFunc(after.Apps, byID)
		if !reflect.DeepEqual(before.Apps, after.Apps) {
			t.Errorf("installed apps changed after cleanup: before %+v, after %+v", before.Apps, after.Apps)
		}
		t.Log("cleanup checked: test app, settings, and marker absent; original apps preserved")
	})

	var archive bytes.Buffer
	compressed := gzip.NewWriter(&archive)
	writer := tar.NewWriter(compressed)
	if err := writer.AddFS(os.DirFS("testdata/apps")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	first, err := c.AppsStage(ctx, archive.Bytes())
	if err != nil || first.Staged.ID != appID || first.Staged.APIVersion != "27.14.0" || first.Installed != nil {
		t.Fatalf("stage: %+v, %v", first, err)
	}
	second, err := c.AppsStage(ctx, archive.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	var httpErr *HTTPError
	if err := c.AppsInstall(ctx, first.InstallKey); !errors.As(err, &httpErr) || httpErr.StatusCode != 400 {
		t.Fatalf("replaced installation key: %v", err)
	}
	if err := c.AppsInstall(ctx, second.InstallKey); err != nil {
		t.Fatal(err)
	}
	replacement, err := c.AppsStage(ctx, archive.Bytes())
	if err != nil || replacement.Installed == nil || replacement.Installed.ID != appID {
		t.Fatalf("replacement: %+v, %v", replacement, err)
	}
	if err := c.AppsInstall(ctx, replacement.InstallKey); err != nil {
		t.Fatal(err)
	}
	t.Log("staging, replacement details, and single-use installation keys checked")
	if err := c.AppsLaunch(ctx, appID); err != nil {
		t.Fatal(err)
	}
	// Launch acknowledges the request before the script starts. Wait for the
	// fixture's localStorage marker before trying to quit the running app.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	started := false
	for !started {
		select {
		case <-ctx.Done():
			t.Fatal("script did not write its ready marker: ", ctx.Err())
		case <-ticker.C:
			entries, err := c.StorageListGet(ctx, "/ext/apps_data/jsrunner")
			if err != nil {
				t.Fatal(err)
			}
			if !slices.ContainsFunc(entries.List, func(entry StorageEntry) bool {
				return entry.Name == appID+".localstorage.json"
			}) {
				continue
			}
			data, err := c.StorageRead(ctx, markerPath)
			if err != nil {
				t.Fatal(err)
			}
			var marker struct {
				Data map[string]string `json:"data"`
			}
			if err := json.Unmarshal(data, &marker); err != nil {
				t.Fatal(err)
			}
			started = marker.Data["ready"] == "true"
		}
	}
	if err := c.AppsQuit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("script execution proved by localStorage marker; app quit")

	document, err := c.AppsSettingsGet(ctx, appID)
	if err != nil || document.Version != 1 || string(document.Values["enabled"]) != "true" || string(document.Values["count"]) != "7" {
		t.Fatalf("defaults: %+v, %v", document, err)
	}
	document.Values["enabled"] = json.RawMessage(`false`)
	document.Values["count"] = json.RawMessage(`0`)
	document.Values["details"] = json.RawMessage(`{"label":"changed"}`)
	if err := c.AppsSettingsSet(ctx, appID, *document); err != nil {
		t.Fatal(err)
	}
	got, err := c.AppsSettingsGet(ctx, appID)
	if err != nil || string(got.Values["enabled"]) != "false" || string(got.Values["count"]) != "0" {
		t.Fatalf("stored settings: %+v, %v", got, err)
	}
	var details struct {
		Label string `json:"label"`
	}
	if err := json.Unmarshal(got.Values["details"], &details); err != nil || details.Label != "changed" {
		t.Fatalf("nested settings: %+v, %v", details, err)
	}
	if err := c.AppsSettingsReset(ctx, appID); err != nil {
		t.Fatal(err)
	}
	got, err = c.AppsSettingsGet(ctx, appID)
	if err != nil || string(got.Values["enabled"]) != "true" || string(got.Values["count"]) != "7" {
		t.Fatalf("reset settings: %+v, %v", got, err)
	}
	t.Log("settings readback checked, including false, zero, nested objects, and reset")
	if err := c.AppsRemove(ctx, appID); err != nil {
		t.Fatal(err)
	}
	persisted, err := c.StorageRead(ctx, settingsPath)
	if err != nil {
		t.Fatalf("removal unexpectedly deleted settings: %v", err)
	}
	var retained AppSettingsDocument
	if err := json.Unmarshal(persisted, &retained); err != nil || retained.Version != 1 {
		t.Fatalf("retained settings: %s, %v", persisted, err)
	}
	t.Log("removal kept settings; cleanup will remove the test settings file")
}
