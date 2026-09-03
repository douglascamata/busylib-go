//go:build smoke

// Package smoke exercises the library against a running BUSY Bar emulator
// (https://github.com/maxswinkels/busybar-emulator). Run it with
// scripts/smoke.sh, or point BUSYBAR_EMULATOR_URL at an emulator and run
// `go test -tags smoke ./smoke/`.
package smoke

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/douglascamata/busylib-go/busybar"
	"github.com/douglascamata/busylib-go/statestream"
	"github.com/douglascamata/busylib-go/statestream/pb"
)

const app = "busylib-go-smoke"

func newClient(t *testing.T) (*busybar.Client, string) {
	t.Helper()
	addr := os.Getenv("BUSYBAR_EMULATOR_URL")
	if addr == "" {
		t.Skip("BUSYBAR_EMULATOR_URL not set")
	}
	c, err := busybar.New(busybar.Config{Addr: addr})
	if err != nil {
		t.Fatal(err)
	}
	return c, addr
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSystem(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()
	v, err := c.SystemVersionGet(ctx)
	must(t, err)
	if v.APISemver == "" {
		t.Fatal("empty api_semver")
	}
	st, err := c.SystemStatusGet(ctx)
	must(t, err)
	if st.System == nil || st.System.APISemver != v.APISemver || st.Power == nil || st.Device == nil || st.Firmware == nil {
		t.Fatalf("status %+v", st)
	}
	if c.APISemver() != v.APISemver {
		t.Fatalf("client did not cache semver: %q", c.APISemver())
	}
	power, err := c.SystemStatusPowerGet(ctx)
	must(t, err)
	if power.State == "" {
		t.Fatalf("power %+v", power)
	}
	tr, err := c.SystemTransportGet(ctx)
	must(t, err)
	if tr.Type != busybar.TransportUSB {
		t.Fatalf("localhost should report usb, got %+v", tr)
	}
	dump, err := c.SystemLogDump(ctx, "smoke")
	must(t, err)
	if dump.Path == "" {
		t.Fatalf("log dump %+v", dump)
	}
	access, err := c.SettingsAccessGet(ctx)
	must(t, err)
	if access.Mode == "" {
		t.Fatalf("access %+v", access)
	}
	tm, err := c.TimeGet(ctx)
	must(t, err)
	if _, err := time.Parse(time.RFC3339Nano, tm.Timestamp); err != nil {
		t.Fatalf("timestamp %q: %v", tm.Timestamp, err)
	}
}

func TestSettingsName(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()
	orig, err := c.SettingsNameGet(ctx)
	must(t, err)
	t.Cleanup(func() { c.SettingsNameSet(ctx, orig.Name) })
	must(t, c.SettingsNameSet(ctx, "busylib-go"))
	got, err := c.SettingsNameGet(ctx)
	must(t, err)
	if got.Name != "busylib-go" {
		t.Fatalf("name %q", got.Name)
	}
}

func TestDisplay(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()
	t.Cleanup(func() { c.DisplayClear(ctx, ""); c.DisplayBrightnessSet(ctx, busybar.BrightnessAuto) })

	must(t, c.DisplayBrightnessSet(ctx, busybar.BrightnessLevel(42)))
	b, err := c.DisplayBrightnessGet(ctx)
	must(t, err)
	if b.Value != "42" {
		t.Fatalf("brightness %q", b.Value)
	}
	must(t, c.DisplayBrightnessSet(ctx, busybar.BrightnessAuto))
	b, err = c.DisplayBrightnessGet(ctx)
	must(t, err)
	if b.Value != "auto" {
		t.Fatalf("brightness %q", b.Value)
	}

	// A 1x1 white PNG, used as an uploaded asset.
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 0x49, 0x48, 0x44, 0x52, 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89, 0, 0, 0, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0xf8, 0xff, 0xff, 0x3f, 0, 5, 0xfe, 2, 0xfe, 0xa7, 0x35, 0x81, 0x84, 0, 0, 0, 0, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}
	must(t, c.AssetsUpload(ctx, busybar.AssetsUploadParams{ApplicationName: app, File: "dot.png", Data: png}))
	t.Cleanup(func() { c.AssetsDelete(ctx, app) })

	must(t, c.DisplayDraw(ctx, busybar.DisplayDrawParams{
		ApplicationName: app,
		Priority:        60,
		Elements: []busybar.Element{
			busybar.TextElement{ElementBase: busybar.ElementBase{ID: "t", X: 36, Y: 8, Align: busybar.AlignCenter}, Text: "GO", Font: busybar.FontBold, Color: "#2B7FFFFF"},
			busybar.ImageElement{ElementBase: busybar.ElementBase{ID: "i", X: 1, Y: 1}, Path: "dot.png"},
			busybar.RectangleElement{ElementBase: busybar.ElementBase{ID: "r", X: 60, Y: 4}, Width: 8, Height: 8, Fill: busybar.FillSolid, FillColors: []string{"#FF0000FF"}, BorderWidth: new(0)},
		},
	}))

	// Another app with lower priority must be refused with 409.
	err = c.DisplayDraw(ctx, busybar.DisplayDrawParams{
		ApplicationName: app + "-intruder",
		Priority:        10,
		Elements:        []busybar.Element{busybar.TextElement{ElementBase: busybar.ElementBase{ID: "x"}, Text: "no", Font: busybar.FontSmall}},
	})
	var httpErr *busybar.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 409 {
		t.Fatalf("expected 409, got %v", err)
	}
	must(t, c.DisplayClear(ctx, app))
	must(t, c.AssetsDelete(ctx, app))
}

func TestStorage(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()
	const dir = "_busylib_go_smoke/"
	path, renamed := dir+"hello.txt", dir+"renamed.txt"
	t.Cleanup(func() { c.StorageRemove(ctx, path); c.StorageRemove(ctx, renamed) })

	must(t, c.StorageWrite(ctx, path, []byte("hello from go")))
	got, err := c.StorageRead(ctx, path)
	must(t, err)
	if !bytes.Equal(got, []byte("hello from go")) {
		t.Fatalf("read %q", got)
	}
	list, err := c.StorageListGet(ctx, dir)
	must(t, err)
	found := false
	for _, e := range list.List {
		found = found || (e.Name == path && e.Type == busybar.StorageFile && e.Size == 13)
	}
	if !found {
		t.Fatalf("list %+v", list.List)
	}
	must(t, c.StorageRename(ctx, path, renamed))
	if _, err := c.StorageRead(ctx, path); err == nil {
		t.Fatal("old path still readable after rename")
	}
	must(t, c.StorageRemove(ctx, renamed))
	status, err := c.StorageStatusGet(ctx)
	must(t, err)
	if status.TotalBytes == 0 {
		t.Fatalf("status %+v", status)
	}
}

func TestBusySnapshot(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()
	orig, err := c.BusySnapshotGet(ctx)
	must(t, err)
	t.Cleanup(func() { c.BusySnapshotSet(ctx, *orig) })

	must(t, c.BusySnapshotSet(ctx, busybar.BusySnapshot{Snapshot: busybar.BusySnapshotState{
		Type: busybar.BusySimple, CardID: "00000000-0000-0000-0000-000000000000", TimeLeftMs: 9000,
		BusyBarSettings: busybar.BusyBarSettings{Theme: "on_air"},
	}}))
	got, err := c.BusySnapshotGet(ctx)
	must(t, err)
	if got.Snapshot.Type != busybar.BusySimple || got.Snapshot.TimeLeftMs != 9000 || got.SnapshotTimestampMs == 0 {
		t.Fatalf("snapshot %+v", got)
	}
	profile, err := c.BusyProfileGet(ctx, busybar.BusySlotBusy)
	must(t, err)
	if profile.TimerSettings.Type == "" {
		t.Fatalf("profile %+v", profile)
	}
}

func TestAudioVolume(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()
	orig, err := c.AudioVolumeGet(ctx)
	must(t, err)
	t.Cleanup(func() { c.AudioVolumeSet(ctx, busybar.AudioVolumeParams{Volume: orig.Volume, Silent: true}) })
	must(t, c.AudioVolumeSet(ctx, busybar.AudioVolumeParams{Volume: 30, Silent: true}))
	got, err := c.AudioVolumeGet(ctx)
	must(t, err)
	if got.Volume != 30 {
		t.Fatalf("volume %d", got.Volume)
	}
}

// TestInputReachesStateStream sends keys over HTTP and expects the emulator
// to deliver them as protobuf input events on the WebSocket state stream.
func TestInputReachesStateStream(t *testing.T) {
	c, addr := newClient(t)
	ctx := context.Background()
	s, err := statestream.NewLocal(statestream.Options{Addr: addr})
	must(t, err)
	updates := make(chan statestream.Update, 16)
	must(t, s.Start(ctx, statestream.Callbacks{
		Data: func(st *statestream.State) {
			for _, u := range st.Updates {
				updates <- u
			}
		},
		Error: func(e *statestream.Error) { t.Errorf("stream error: %v", e) },
	}))
	if st := s.Status(); st.Main != statestream.Running {
		t.Fatalf("status %+v", st)
	}

	must(t, c.InputSend(ctx, busybar.KeyOK))
	must(t, c.InputSend(ctx, busybar.KeyUp))
	must(t, c.InputSend(ctx, busybar.KeySettings))

	want := []func(u statestream.Update) bool{
		func(u statestream.Update) bool {
			b := u.GetInput().GetButtonEvent()
			return b != nil && b.GetButton() == pb.Button_OK && b.GetAction() == pb.ButtonAction_PRESS
		},
		func(u statestream.Update) bool {
			b := u.GetInput().GetButtonEvent()
			return b != nil && b.GetButton() == pb.Button_OK && b.GetAction() == pb.ButtonAction_RELEASE
		},
		func(u statestream.Update) bool { return u.GetInput().GetEncoderEvent().GetDelta() == 1 },
		func(u statestream.Update) bool {
			return u.GetInput().GetSwitchEvent().GetPosition() == pb.SwitchPosition_SETTINGS
		},
	}
	for i, ok := range want {
		select {
		case u := <-updates:
			if u.Kind != statestream.KindInput || !ok(u) {
				t.Fatalf("update %d: %v", i, u.StateUpdate)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for input event %d", i)
		}
	}
	must(t, s.Stop(ctx))
	if st := s.Status(); st.Main != statestream.Stopped {
		t.Fatalf("status after stop %+v", st)
	}
}
