package busybar

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/douglascamata/busylib-go/frame"
)

func TestNewResolvesAddr(t *testing.T) {
	cases := []struct {
		name     string
		cfg      Config
		wantAddr string
		wantBase string
		wantErr  bool
	}{
		{"empty", Config{}, "http://10.0.4.20", "http://10.0.4.20/api", false},
		{"token only", Config{Token: "t"}, "https://api.busy.app", "https://api.busy.app/busybar", false},
		{"bare ip", Config{Addr: " 192.168.1.5 "}, "http://192.168.1.5", "http://192.168.1.5/api", false},
		{"host and port", Config{Addr: "bar.local:8080"}, "http://bar.local:8080", "http://bar.local:8080/api", false},
		{"drops path", Config{Addr: "https://example.com/busybar/"}, "https://example.com", "https://example.com/api", false},
		{"proxy defaults to https", Config{Addr: "api.busy.app", Token: "t"}, "https://api.busy.app", "https://api.busy.app/busybar", false},
		{"proxy keeps explicit http", Config{Addr: "http://api.dev.busy.app", Token: "t"}, "http://api.dev.busy.app", "http://api.dev.busy.app/busybar", false},
		{"proxy needs token", Config{Addr: "api.busy.app"}, "", "", true},
		{"garbage", Config{Addr: "http://"}, "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got addr %q", c.Addr())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.Addr() != tc.wantAddr || c.baseURL != tc.wantBase {
				t.Fatalf("got %q %q want %q %q", c.Addr(), c.baseURL, tc.wantAddr, tc.wantBase)
			}
		})
	}
}

// fakeDevice is an httptest server that speaks the version-gated API.
type fakeDevice struct {
	*httptest.Server
	mu       sync.Mutex
	semver   string
	requests []*http.Request
	versions atomic.Int32
	handle   func(w http.ResponseWriter, r *http.Request) bool
}

func newFakeDevice(t *testing.T) *fakeDevice {
	t.Helper()
	d := &fakeDevice{semver: "25.0.0"}
	d.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.requests = append(d.requests, r.Clone(r.Context()))
		semver := d.semver
		d.mu.Unlock()
		if r.URL.Path == "/api/version" {
			d.versions.Add(1)
			writeJSON(w, 200, map[string]string{"api_semver": semver})
			return
		}
		if got := r.Header.Get("X-API-Sem-Ver"); got != semver {
			writeJSON(w, 405, map[string]any{"error": "Incompatible API version", "code": 405})
			return
		}
		if d.handle != nil && d.handle(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/name":
			writeJSON(w, 200, map[string]string{"name": "Fake bar"})
		default:
			writeJSON(w, 404, map[string]any{"error": "no route", "code": 404})
		}
	}))
	t.Cleanup(d.Close)
	return d
}

func (d *fakeDevice) client(t *testing.T, cfg Config) *Client {
	t.Helper()
	cfg.Addr = d.URL
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (d *fakeDevice) paths() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.requests))
	for i, r := range d.requests {
		out[i] = r.Method + " " + r.URL.RequestURI()
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func TestVersionFetchedOnceAndHeadersSet(t *testing.T) {
	d := newFakeDevice(t)
	c := d.client(t, Config{HTTPAccessPassword: "1234", Token: "bearer-me"})
	ctx := context.Background()

	for range 2 {
		name, err := c.SettingsNameGet(ctx)
		if err != nil || name.Name != "Fake bar" {
			t.Fatalf("got %+v, %v", name, err)
		}
	}
	if got := d.versions.Load(); got != 1 {
		t.Fatalf("/version fetched %d times, want 1", got)
	}
	if c.APISemver() != "25.0.0" {
		t.Fatalf("cached semver %q", c.APISemver())
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	versionReq, nameReq := d.requests[0], d.requests[1]
	if versionReq.Header.Get("X-API-Sem-Ver") != "" || versionReq.Header.Get("X-API-Token") != "" {
		t.Fatalf("/version must not carry device headers: %v", versionReq.Header)
	}
	if versionReq.Header.Get("Authorization") != "Bearer bearer-me" {
		t.Fatalf("/version missing bearer: %v", versionReq.Header)
	}
	if nameReq.Header.Get("X-API-Token") != "1234" || nameReq.Header.Get("Authorization") != "Bearer bearer-me" {
		t.Fatalf("headers %v", nameReq.Header)
	}
}

func TestConcurrentCallsShareOneVersionFetch(t *testing.T) {
	d := newFakeDevice(t)
	c := d.client(t, Config{})
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if _, err := c.SettingsNameGet(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got := d.versions.Load(); got != 1 {
		t.Fatalf("/version fetched %d times, want 1", got)
	}
}

func TestRetriesOnceAfter405(t *testing.T) {
	d := newFakeDevice(t)
	c := d.client(t, Config{})
	ctx := context.Background()
	if _, err := c.SettingsNameGet(ctx); err != nil {
		t.Fatal(err)
	}
	// Device "updated": the cached semver is now stale.
	d.mu.Lock()
	d.semver = "26.0.0"
	d.mu.Unlock()
	if _, err := c.SettingsNameGet(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /api/version", "GET /api/name", "GET /api/name", "GET /api/version", "GET /api/name"}
	if got := d.paths(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
	if c.APISemver() != "26.0.0" {
		t.Fatalf("semver not refreshed: %q", c.APISemver())
	}
}

func TestHTTPErrorCarriesStatusAndMessage(t *testing.T) {
	d := newFakeDevice(t)
	d.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/display/draw" {
			writeJSON(w, 409, map[string]any{"error": "Not drawn due to low priority", "code": 409})
			return true
		}
		return false
	}
	c := d.client(t, Config{})
	err := c.DisplayDraw(context.Background(), DisplayDrawParams{
		ApplicationName: "t",
		Elements:        []Element{TextElement{ElementBase: ElementBase{ID: "a"}, Text: "x"}},
	})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("got %T %v", err, err)
	}
	if httpErr.StatusCode != 409 || httpErr.Message != "Not drawn due to low priority" || err.Error() != httpErr.Message {
		t.Fatalf("%+v", httpErr)
	}
}

func TestDefaultTimeoutOnlyWithoutDeadline(t *testing.T) {
	d := newFakeDevice(t)
	d.handle = func(w http.ResponseWriter, r *http.Request) bool {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		return true
	}
	c := d.client(t, Config{Timeout: 50 * time.Millisecond})
	start := time.Now()
	_, err := c.SettingsNameGet(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("got %v after %v", err, time.Since(start))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start = time.Now()
	_, err = c.SettingsNameGet(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) < 250*time.Millisecond {
		t.Fatalf("caller deadline not honored: %v after %v", err, time.Since(start))
	}
}

func TestDisplayDrawBody(t *testing.T) {
	d := newFakeDevice(t)
	var body map[string]any
	d.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/display/draw" {
			json.NewDecoder(r.Body).Decode(&body)
			writeJSON(w, 200, map[string]string{"result": "OK"})
			return true
		}
		return false
	}
	c := d.client(t, Config{})
	err := c.DisplayDraw(context.Background(), DisplayDrawParams{
		ApplicationName: "demo",
		Elements: []Element{
			TextElement{ElementBase: ElementBase{ID: "t", X: 36, Align: AlignCenter}, Text: "HI", Font: FontBold},
			ImageElement{ElementBase: ElementBase{ID: "i"}, StockPath: "sun", Opacity: new(40)},
			RectangleElement{ElementBase: ElementBase{ID: "r"}, Width: 4, Height: 2, BorderWidth: new(0)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["priority"] != float64(50) || body["application_name"] != "demo" {
		t.Fatalf("body %v", body)
	}
	els := body["elements"].([]any)
	text, img, rect := els[0].(map[string]any), els[1].(map[string]any), els[2].(map[string]any)
	if text["type"] != "text" || text["align"] != "center" || text["x"] != float64(36) || text["font"] != "bold" {
		t.Fatalf("text %v", text)
	}
	if _, has := text["timeout"]; has {
		t.Fatalf("zero timeout must be omitted: %v", text)
	}
	if img["type"] != "image" || img["opacity"] != float64(40) || img["stock_path"] != "sun" {
		t.Fatalf("image %v", img)
	}
	if rect["type"] != "rectangle" || rect["border_width"] != float64(0) {
		t.Fatalf("rect %v", rect)
	}
}

func TestScreenFrameDecodesBase64AndConverts(t *testing.T) {
	d := newFakeDevice(t)
	w, h := frame.Dimensions(frame.Front)
	bgr := make([]byte, w*h*3)
	bgr[0], bgr[1], bgr[2] = 10, 20, 30 // first pixel B,G,R
	d.handle = func(rw http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/screen" && r.URL.Query().Get("display") == "0" {
			rw.Write([]byte(base64.StdEncoding.EncodeToString(bgr)))
			return true
		}
		return false
	}
	c := d.client(t, Config{})
	raw, err := c.DisplayScreenFrameGet(context.Background(), frame.Front, FrameRaw)
	if err != nil || len(raw) != len(bgr) || raw[0] != 10 {
		t.Fatalf("raw %d %v", len(raw), err)
	}
	rgba, err := c.DisplayScreenFrameGet(context.Background(), frame.Front, FrameRGBA)
	if err != nil || len(rgba) != w*h*4 || rgba[0] != 30 || rgba[2] != 10 || rgba[3] != 255 {
		t.Fatalf("rgba %v %v", rgba[:4], err)
	}
}

func TestClientSideValidation(t *testing.T) {
	d := newFakeDevice(t)
	c := d.client(t, Config{})
	ctx := context.Background()
	if err := c.AudioVolumeSet(ctx, AudioVolumeParams{Volume: 101}); err == nil {
		t.Fatal("volume 101 accepted")
	}
	if err := c.DisplayBrightnessSet(ctx, Brightness("bright")); err == nil {
		t.Fatal("bad brightness accepted")
	}
	if err := c.SettingsAccessSet(ctx, HTTPAccessParams{Mode: HTTPAccessKey, Key: "12"}); err == nil {
		t.Fatal("short key accepted")
	}
	if got := d.versions.Load(); got != 0 {
		t.Fatalf("validation must fail before any request, got %d requests", got)
	}
}

func TestVersionWaitHonorsCallerDeadline(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			close(started)
			<-release
			w.Write([]byte(`{"api_semver":"1.0.0"}`))
			return
		}
		w.Write([]byte(`{"name":"desk"}`))
	}))
	defer srv.Close()
	c, err := New(Config{Addr: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	leader := make(chan error, 1)
	go func() { _, err := c.SettingsNameGet(context.Background()); leader <- err }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	follower := make(chan error, 1)
	go func() { _, err := c.SettingsNameGet(ctx); follower <- err }()
	var got error
	returned := false
	select {
	case got = <-follower:
		returned = true
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	if err := <-leader; err != nil {
		t.Fatal(err)
	}
	if !returned {
		got = <-follower
	}
	if !returned || !errors.Is(got, context.DeadlineExceeded) {
		t.Errorf("20ms caller returned before version fetch was released: %v; error: %v", returned, got)
	}
}

func TestVersionFetchCanRetryAfterFailure(t *testing.T) {
	var versions atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			if versions.Add(1) == 1 {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "restarting"})
			} else {
				writeJSON(w, http.StatusOK, map[string]string{"api_semver": "25.0.0"})
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"name": "desk"})
	}))
	defer srv.Close()
	c, err := New(Config{Addr: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.SettingsNameGet(context.Background())
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("first fetch: %v", err)
	}
	name, err := c.SettingsNameGet(context.Background())
	if err != nil || name.Name != "desk" || versions.Load() != 2 {
		t.Fatalf("retry: %+v, %v; fetches %d", name, err, versions.Load())
	}
}
