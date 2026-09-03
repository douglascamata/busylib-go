package statestream

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/douglascamata/busylib-go/statestream/pb"
)

const testTimeout = 3 * time.Second

type wsServer struct {
	*httptest.Server
	accepts atomic.Int32
}

// newWSServer runs handler for every accepted WebSocket connection.
func newWSServer(t *testing.T, handler func(conn *websocket.Conn, n int32)) *wsServer {
	t.Helper()
	s := &wsServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		handler(conn, s.accepts.Add(1))
	}))
	t.Cleanup(s.Close)
	return s
}

func recv[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(testTimeout):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

func inputState(t *testing.T) []byte {
	t.Helper()
	b, err := proto.Marshal(&pb.State{Timestamp: 7, Updates: []*pb.StateUpdate{
		{State: &pb.StateUpdate_Input{Input: &pb.InputEvent{Event: &pb.InputEvent_ButtonEvent{ButtonEvent: &pb.ButtonEvent{Button: pb.Button_BACK}}}}},
		{State: &pb.StateUpdate_Frame{Frame: &pb.Frame{Width: 4, Height: 1, Encoding: pb.Encoding_RUN_LENGTH, PixelFormat: pb.PixelFormat_RGB888, Data: []byte{0x81, 1, 2, 3, 0x03, 9, 9, 9}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func waitStatus(t *testing.T, s *Stream, ok func(Status) bool) Status {
	t.Helper()
	deadline := time.Now().Add(testTimeout)
	for time.Now().Before(deadline) {
		if st := s.Status(); ok(st) {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("status never matched: %+v", s.Status())
	panic("unreachable")
}

func TestLocalStreamDeliversDecodedUpdatesAndStopsCleanly(t *testing.T) {
	handshake := make(chan string, 1)
	closed := make(chan websocket.StatusCode, 1)
	srv := newWSServer(t, func(conn *websocket.Conn, _ int32) {
		ctx := context.Background()
		_, msg, err := conn.Read(ctx)
		if err != nil {
			return
		}
		handshake <- string(msg)
		conn.Write(ctx, websocket.MessageBinary, inputState(t))
		_, _, err = conn.Read(ctx)
		closed <- websocket.CloseStatus(err)
	})
	s, err := NewLocal(Options{Addr: srv.URL, HTTPAccessPassword: "1234"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "ws" + srv.URL[len("http"):] + "/api/status/ws?x-api-token=1234"; s.URL() != want {
		t.Fatalf("url %q want %q", s.URL(), want)
	}

	data := make(chan *State, 4)
	raw := make(chan []byte, 4)
	ctx := context.Background()
	err = s.Start(ctx, Callbacks{
		Data:    func(st *State) { data <- st },
		RawData: func(b []byte) { raw <- b },
		Error:   func(e *Error) { t.Errorf("unexpected error %v", e) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := recv(t, handshake, "handshake"); got != `{"enable":true}` {
		t.Fatalf("handshake %q", got)
	}
	st := recv(t, data, "state")
	if st.Timestamp != 7 || len(st.Updates) != 2 || st.Updates[0].Kind != KindInput || st.Updates[1].Kind != KindFrame {
		t.Fatalf("state %+v", st)
	}
	if st.Updates[0].GetInput().GetButtonEvent().GetButton() != pb.Button_BACK {
		t.Fatalf("button %v", st.Updates[0].GetInput())
	}
	if f := st.Updates[1].Frame; f == nil || f.RGBA[0] != 3 || f.RGBA[4] != 9 || len(f.RGBA) != 16 {
		t.Fatalf("frame %+v", st.Updates[1].Frame)
	}
	if len(recv(t, raw, "raw")) == 0 {
		t.Fatal("raw callback got empty message")
	}
	if got := s.Status(); got.Main != Running || got.Connection != Connected || got.Auth != Authenticated || got.Data != DataActive {
		t.Fatalf("status %+v", got)
	}

	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if code := recv(t, closed, "server close"); code != websocket.StatusNormalClosure {
		t.Fatalf("server saw close code %d", code)
	}
	if got := s.Status(); got.Main != Stopped || got.Connection != Disconnected || got.Data != DataNone {
		t.Fatalf("status after stop %+v", got)
	}
	if err := s.Stop(ctx); err != nil {
		t.Fatalf("second stop: %v", err)
	}
}

func TestStartRejectsWhileRunning(t *testing.T) {
	srv := newWSServer(t, func(conn *websocket.Conn, _ int32) { conn.Read(context.Background()); conn.Read(context.Background()) })
	s, _ := NewLocal(Options{Addr: srv.URL})
	ctx := context.Background()
	if err := s.Start(ctx, Callbacks{}); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(ctx)
	var e *Error
	if err := s.Start(ctx, Callbacks{}); !errors.As(err, &e) || e.Code != CodeStreamAlreadyStarted {
		t.Fatalf("got %v", err)
	}
}

func TestStartFailsWhenUnreachable(t *testing.T) {
	s, _ := NewLocal(Options{Addr: "127.0.0.1:1", ConnectTimeout: time.Second})
	errs := make(chan *Error, 1)
	err := s.Start(context.Background(), Callbacks{Error: func(e *Error) { errs <- e }})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeConnectionFailed {
		t.Fatalf("got %v", err)
	}
	if recv(t, errs, "error callback").Code != CodeConnectionFailed {
		t.Fatal("error callback code")
	}
	if st := s.Status(); st.Main != Failed || st.MainError != e {
		t.Fatalf("status %+v", st)
	}
}

func TestStartTimesOutWhenRemoteNeverAuthenticates(t *testing.T) {
	srv := newWSServer(t, func(conn *websocket.Conn, _ int32) { conn.Read(context.Background()); conn.Read(context.Background()) })
	s, _ := NewRemote(Options{Addr: srv.URL, Token: "t", ConnectTimeout: 100 * time.Millisecond})
	err := s.Start(context.Background(), Callbacks{})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeConnectionTimeout {
		t.Fatalf("got %v", err)
	}
	if s.Status().Main != Failed {
		t.Fatalf("status %+v", s.Status())
	}
}

func TestReconnectsAfterAbnormalClose(t *testing.T) {
	srv := newWSServer(t, func(conn *websocket.Conn, n int32) {
		ctx := context.Background()
		conn.Read(ctx)
		if n == 1 {
			conn.Close(websocket.StatusInternalError, "boom")
			return
		}
		conn.Write(ctx, websocket.MessageBinary, inputState(t))
		conn.Read(ctx)
	})
	s, _ := NewLocal(Options{Addr: srv.URL, ReconnectDelay: 10 * time.Millisecond})
	data := make(chan *State, 1)
	statuses := make(chan Status, 64)
	ctx := context.Background()
	if err := s.Start(ctx, Callbacks{Data: func(st *State) { data <- st }, Status: func(st Status) { statuses <- st }}); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(ctx)
	recv(t, data, "data from second connection")
	if srv.accepts.Load() != 2 {
		t.Fatalf("accepts %d", srv.accepts.Load())
	}
	sawReconnecting := false
	for len(statuses) > 0 {
		if st := <-statuses; st.Connection == Reconnecting && st.ConnectionAttempts == 1 {
			sawReconnecting = true
		}
	}
	if !sawReconnecting {
		t.Fatal("never reported RECONNECTING with attempt 1")
	}
	if st := s.Status(); st.Main != Running || st.Connection != Connected || st.ConnectionAttempts != 0 {
		t.Fatalf("status %+v", st)
	}
}

func TestGivesUpAfterMaxReconnectAttempts(t *testing.T) {
	srv := newWSServer(t, func(conn *websocket.Conn, _ int32) {
		conn.Read(context.Background())
		conn.Close(websocket.StatusInternalError, "boom")
	})
	s, _ := NewLocal(Options{Addr: srv.URL, ReconnectDelay: 200 * time.Millisecond, MaxReconnectAttempts: 2})
	errs := make(chan *Error, 8)
	if err := s.Start(context.Background(), Callbacks{Error: func(e *Error) { errs <- e }}); err != nil {
		t.Fatal(err)
	}
	// Once the client is waiting to reconnect, take the server away so every
	// further dial fails and the attempt budget runs out.
	waitStatus(t, s, func(st Status) bool { return st.Connection == Reconnecting && st.ConnectionAttempts == 1 })
	srv.Close()
	var codes []ErrorCode
	for {
		e := recv(t, errs, "reconnect failed")
		codes = append(codes, e.Code)
		if e.Code == CodeReconnectFailed {
			break
		}
	}
	want := []ErrorCode{CodeConnectionFailed, CodeConnectionFailed, CodeReconnectFailed}
	if len(codes) != len(want) {
		t.Fatalf("errors %v want %v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("errors %v want %v", codes, want)
		}
	}
	waitStatus(t, s, func(st Status) bool { return st.Main == Failed && st.Connection == Disconnected })
	if srv.accepts.Load() != 1 {
		t.Fatalf("accepts %d", srv.accepts.Load())
	}
}

func TestNegativeMaxReconnectAttemptsFailsOnFirstDrop(t *testing.T) {
	srv := newWSServer(t, func(conn *websocket.Conn, _ int32) {
		conn.Read(context.Background())
		conn.Close(websocket.StatusInternalError, "boom")
	})
	s, _ := NewLocal(Options{Addr: srv.URL, ReconnectDelay: 10 * time.Millisecond, MaxReconnectAttempts: -1})
	if s.opts.MaxReconnectAttempts != 0 {
		t.Fatalf("MaxReconnectAttempts %d, want a negative value normalized to 0", s.opts.MaxReconnectAttempts)
	}
	errs := make(chan *Error, 8)
	if err := s.Start(context.Background(), Callbacks{Error: func(e *Error) { errs <- e }}); err != nil {
		t.Fatal(err)
	}
	if e := recv(t, errs, "reconnect failed"); e.Code != CodeReconnectFailed {
		t.Fatalf("first error %v, want %v", e.Code, CodeReconnectFailed)
	}
	waitStatus(t, s, func(st Status) bool { return st.Main == Failed && st.Connection == Disconnected })
	if srv.accepts.Load() != 1 {
		t.Fatalf("accepts %d, want 1: the stream must not redial", srv.accepts.Load())
	}
}

func TestDataGoesStaleWithoutMessages(t *testing.T) {
	srv := newWSServer(t, func(conn *websocket.Conn, _ int32) {
		ctx := context.Background()
		conn.Read(ctx)
		conn.Write(ctx, websocket.MessageBinary, inputState(t))
		conn.Read(ctx)
	})
	s, _ := NewLocal(Options{Addr: srv.URL, DataTimeout: 30 * time.Millisecond})
	ctx := context.Background()
	if err := s.Start(ctx, Callbacks{}); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(ctx)
	waitStatus(t, s, func(st Status) bool { return st.Data == DataActive })
	waitStatus(t, s, func(st Status) bool { return st.Data == DataStale })
}

func TestFatalDeviceErrorEndsStream(t *testing.T) {
	srv := newWSServer(t, func(conn *websocket.Conn, _ int32) {
		ctx := context.Background()
		conn.Read(ctx)
		b, _ := proto.Marshal(&pb.State{Error: &pb.Error{Cause: pb.Cause_RESOURCE_LIMIT, Severity: pb.Severity_FATAL}})
		conn.Write(ctx, websocket.MessageBinary, b)
		conn.Read(ctx)
	})
	s, _ := NewLocal(Options{Addr: srv.URL})
	errs := make(chan *Error, 1)
	if err := s.Start(context.Background(), Callbacks{Error: func(e *Error) { errs <- e }}); err != nil {
		t.Fatal(err)
	}
	e := recv(t, errs, "device error")
	if e.Code != CodeDeviceError || e.Data.(*pb.Error).GetCause() != pb.Cause_RESOURCE_LIMIT {
		t.Fatalf("%+v", e)
	}
	waitStatus(t, s, func(st Status) bool { return st.Main == Failed })
	if srv.accepts.Load() != 1 {
		t.Fatal("stream must not reconnect after a fatal device error")
	}
}

func TestRemoteAuthSubscribeAndEnvelope(t *testing.T) {
	msgs := make(chan string, 4)
	srv := newWSServer(t, func(conn *websocket.Conn, _ int32) {
		ctx := context.Background()
		for range 2 {
			_, m, err := conn.Read(ctx)
			if err != nil {
				return
			}
			msgs <- string(m)
		}
		conn.Write(ctx, websocket.MessageText, []byte(`{"type":"device.linked","device":{"id":"d1","hardware_id":"hw","name":"desk"}}`))
		env := `{"type":"protobuf","bar_id":"g1","state":"` + base64.StdEncoding.EncodeToString(inputState(t)) + `"}`
		conn.Write(ctx, websocket.MessageText, []byte(env))
		conn.Read(ctx)
	})
	s, err := NewRemote(Options{Addr: srv.URL, Token: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Subscribe(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	data := make(chan *State, 1)
	events := make(chan DeviceEvent, 1)
	err = s.Start(ctx, Callbacks{Data: func(st *State) { data <- st }, DeviceEvent: func(e DeviceEvent) { events <- e }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop(ctx)
	if got := recv(t, msgs, "token"); got != `{"token":"abc"}` {
		t.Fatalf("first message %q", got)
	}
	if got := recv(t, msgs, "subscribe"); got != `{"subscribe":["g1"]}` {
		t.Fatalf("second message %q", got)
	}
	if ev := recv(t, events, "device event"); ev.Type != "device.linked" || ev.Device.Name != "desk" {
		t.Fatalf("event %+v", ev)
	}
	if st := recv(t, data, "state"); st.BarID != "g1" || len(st.Updates) != 2 {
		t.Fatalf("state %+v", st)
	}
	if st := s.Status(); st.Main != Running || st.Auth != Authenticated {
		t.Fatalf("status %+v", st)
	}
}

func TestRemoteRefreshesTokenOnAuthClose(t *testing.T) {
	tokens := make(chan string, 2)
	srv := newWSServer(t, func(conn *websocket.Conn, n int32) {
		ctx := context.Background()
		_, m, err := conn.Read(ctx)
		if err != nil {
			return
		}
		tokens <- string(m)
		if n == 1 {
			conn.Close(authCloseCode, "expired")
			return
		}
		env := `{"bar_id":"g1","state":"` + base64.StdEncoding.EncodeToString(inputState(t)) + `"}`
		conn.Write(ctx, websocket.MessageText, []byte(env))
		conn.Read(ctx)
	})
	var refreshes atomic.Int32
	s, _ := NewRemote(Options{Addr: srv.URL, Token: "old", TokenProvider: func(context.Context) (string, error) {
		refreshes.Add(1)
		return "new", nil
	}})
	statuses := make(chan Status, 64)
	ctx := context.Background()
	if err := s.Start(ctx, Callbacks{Status: func(st Status) { statuses <- st }}); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(ctx)
	if got := recv(t, tokens, "old token"); got != `{"token":"old"}` {
		t.Fatalf("%q", got)
	}
	if got := recv(t, tokens, "new token"); got != `{"token":"new"}` {
		t.Fatalf("%q", got)
	}
	if refreshes.Load() != 1 {
		t.Fatalf("provider called %d times", refreshes.Load())
	}
	sawReauth := false
	for len(statuses) > 0 {
		if st := <-statuses; st.Auth == Reauthenticating && st.AuthAttempts == 1 {
			sawReauth = true
		}
	}
	if !sawReauth {
		t.Fatal("never reported REAUTHENTICATING")
	}
}

func TestRemoteFailsWithoutTokenProvider(t *testing.T) {
	srv := newWSServer(t, func(conn *websocket.Conn, _ int32) {
		conn.Read(context.Background())
		conn.Close(authCloseCode, "expired")
	})
	s, _ := NewRemote(Options{Addr: srv.URL, Token: "old"})
	err := s.Start(context.Background(), Callbacks{})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeAuthFailed {
		t.Fatalf("got %v", err)
	}
	if st := s.Status(); st.Main != Failed || st.Auth != AuthFailed {
		t.Fatalf("status %+v", st)
	}
}
