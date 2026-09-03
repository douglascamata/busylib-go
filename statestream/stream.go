// Package statestream receives real-time BUSY Bar state updates over WebSocket.
//
// A local stream connects to the device itself and decodes binary protobuf
// frames. A remote stream connects to the BUSY cloud, authenticates with a
// token and receives the same protobuf payloads inside a JSON envelope.
package statestream

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/douglascamata/busylib-go/statestream/pb"
)

const (
	DefaultLocalAddr            = "10.0.4.20"
	DefaultConnectTimeout       = 5 * time.Second
	DefaultDataTimeout          = 15 * time.Second
	DefaultMaxReconnectAttempts = 5
	DefaultMaxAuthAttempts      = 5
	DefaultReconnectDelay       = 500 * time.Millisecond

	authCloseCode websocket.StatusCode = 3000
	writeTimeout                       = 5 * time.Second
)

var errFatalDeviceError = errors.New("fatal device error")

// Options configures a Stream. Zero values select the defaults above.
type Options struct {
	// Addr is an IP, host, or URL. Schemes http(s):// become ws(s)://; without
	// a scheme ws:// is used. Local streams default to DefaultLocalAddr and
	// append /api/status/ws when the URL has no path. Remote streams require it.
	Addr string
	// HTTPAccessPassword is sent as the x-api-token query parameter (local only).
	HTTPAccessPassword string
	// Token authenticates a remote stream.
	Token string
	// TokenProvider returns a fresh token when the server rejects the current
	// one (close code 3000). Remote only.
	TokenProvider func(context.Context) (string, error)

	ConnectTimeout time.Duration
	DataTimeout    time.Duration
	ReconnectDelay time.Duration
	// MaxReconnectAttempts and MaxAuthAttempts count retries after the first
	// failure. Zero selects the default; a negative value disables retries, so
	// the stream fails on the first drop or the first rejected token.
	MaxReconnectAttempts int
	MaxAuthAttempts      int
	// HTTPClient is used for the WebSocket handshake. nil means http.DefaultClient.
	HTTPClient *http.Client
}

// Callbacks receive stream events. Every field is optional. The stream calls
// one callback at a time, so callbacks must not block for long. A callback may
// call Stop and Start.
type Callbacks struct {
	Data    func(*State)
	RawData func([]byte)
	Error   func(*Error)
	Status  func(Status)
	// DeviceEvent reports device.linked, device.name-updated and
	// device.unlinked events from the cloud. Remote only.
	DeviceEvent func(DeviceEvent)
}

type RemoteDevice struct {
	ID         string `json:"id"`
	HardwareID string `json:"hardware_id"`
	Name       string `json:"name"`
}

type DeviceEvent struct {
	Type   string       `json:"type"`
	Device RemoteDevice `json:"device"`
}

// Stream is one WebSocket state subscription. Create it with NewLocal or NewRemote.
type Stream struct {
	remote bool
	url    string
	opts   Options

	mu         sync.Mutex
	status     Status
	token      string
	subs       map[string]struct{}
	current    *streamRun
	generation uint64

	callbackMu      sync.Mutex
	callbackQueue   []callbackEvent
	callbackWorker  bool
	callbackRunning *streamRun
}

// streamRun owns all state that belongs to one call to Start.
type streamRun struct {
	generation uint64
	cancel     context.CancelFunc
	done       chan struct{}
	cb         Callbacks
	conn       *websocket.Conn
	dataTimer  *time.Timer
}

type callbackEvent struct {
	run    *streamRun
	final  bool
	invoke func(Callbacks)
	done   chan struct{}
}

// NewLocal creates a stream to a device on the local network.
func NewLocal(opts Options) (*Stream, error) {
	if opts.Addr == "" {
		opts.Addr = DefaultLocalAddr
	}
	u, err := url.Parse(resolveProtocol(opts.Addr))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("statestream: invalid address %q", opts.Addr)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/api/status/ws"
	}
	if opts.HTTPAccessPassword != "" {
		q := u.Query()
		q.Set("x-api-token", opts.HTTPAccessPassword)
		u.RawQuery = q.Encode()
	}
	return newStream(false, u.String(), opts), nil
}

// NewRemote creates a stream to the BUSY cloud.
func NewRemote(opts Options) (*Stream, error) {
	if opts.Addr == "" {
		return nil, errors.New("statestream: remote address is required")
	}
	u, err := url.Parse(resolveProtocol(opts.Addr))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("statestream: invalid address %q", opts.Addr)
	}
	return newStream(true, u.String(), opts), nil
}

func newStream(remote bool, wsURL string, opts Options) *Stream {
	setDefault(&opts.ConnectTimeout, DefaultConnectTimeout)
	setDefault(&opts.DataTimeout, DefaultDataTimeout)
	setDefault(&opts.ReconnectDelay, DefaultReconnectDelay)
	setDefault(&opts.MaxReconnectAttempts, DefaultMaxReconnectAttempts)
	setDefault(&opts.MaxAuthAttempts, DefaultMaxAuthAttempts)
	opts.MaxReconnectAttempts = max(opts.MaxReconnectAttempts, 0)
	opts.MaxAuthAttempts = max(opts.MaxAuthAttempts, 0)
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}
	return &Stream{remote: remote, url: wsURL, opts: opts, status: initialStatus(), token: opts.Token, subs: map[string]struct{}{}}
}

func setDefault[T int | time.Duration](v *T, def T) {
	if *v == 0 {
		*v = def
	}
}

func resolveProtocol(addr string) string {
	addr = strings.TrimSpace(addr)
	lower := strings.ToLower(addr)
	switch {
	case strings.HasPrefix(lower, "https://"):
		return "wss://" + addr[len("https://"):]
	case strings.HasPrefix(lower, "http://"):
		return "ws://" + addr[len("http://"):]
	case strings.HasPrefix(lower, "wss://"), strings.HasPrefix(lower, "ws://"):
		return addr
	}
	return "ws://" + addr
}

// URL returns the WebSocket URL the stream connects to.
func (s *Stream) URL() string { return s.url }

// Status returns the current status snapshot.
func (s *Stream) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Start connects and returns once the stream is running: for a local stream
// when the socket is open, for a remote stream when the first message after
// authentication arrives. It returns an Error with CodeStreamAlreadyStarted
// when the stream is starting or running, CodeConnectionTimeout when nothing
// happened within Options.ConnectTimeout, or the connection error. After a
// failed Start nothing keeps running. After success the stream reconnects on
// its own until Stop is called or the reconnect budget is used up.
func (s *Stream) Start(ctx context.Context, cb Callbacks) error {
	s.mu.Lock()
	if s.status.Main == Starting || s.status.Main == Running {
		s.mu.Unlock()
		return newError(CodeStreamAlreadyStarted, "stream is already running; call Stop first")
	}
	if s.current != nil {
		s.current.cancel()
		if s.current.dataTimer != nil {
			s.current.dataTimer.Stop()
		}
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.generation++
	run := &streamRun{
		generation: s.generation,
		cancel:     cancel,
		done:       make(chan struct{}),
		cb:         cb,
	}
	s.current = run
	s.status.Main, s.status.MainError = Starting, nil
	s.queueStatusLocked(run, s.status)
	s.mu.Unlock()

	ready := make(chan error, 1)
	go s.run(runCtx, run, ready)

	timer := time.NewTimer(s.opts.ConnectTimeout)
	defer timer.Stop()
	select {
	case err := <-ready:
		if err != nil {
			_ = s.shutdown(ctx, run)
		}
		return err
	case <-timer.C:
		err := newError(CodeConnectionTimeout, fmt.Sprintf("connection timed out after %s", s.opts.ConnectTimeout))
		s.mapErrorToStatus(run, err)
		s.emitError(run, err)
		_ = s.shutdown(ctx, run)
		return err
	case <-ctx.Done():
		_ = s.stopRun(context.Background(), run)
		return ctx.Err()
	}
}

// Stop closes the connection gracefully and clears the callbacks. It returns
// an Error with CodeConnectionLost when the WebSocket did not close cleanly.
//
// Stop waits for the stream goroutine to exit. A callback that is already
// executing can finish. No later callback from that run fires.
func (s *Stream) Stop(ctx context.Context) error {
	s.mu.Lock()
	run := s.current
	if run == nil {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	return s.stopRun(ctx, run)
}

func (s *Stream) stopRun(ctx context.Context, run *streamRun) error {
	s.mu.Lock()
	stopped := false
	if s.current == run {
		stopped = true
		run.cancel()
		if run.dataTimer != nil {
			run.dataTimer.Stop()
		}
		s.current = nil
		s.status.Main, s.status.Connection, s.status.Auth, s.status.Data = Stopped, Disconnected, Unauthenticated, DataNone
		s.status.ConnectionAttempts, s.status.AuthAttempts = 0, 0
	}
	s.mu.Unlock()
	s.cancelCallbacks(run)
	err := s.shutdown(ctx, run)
	if stopped {
		s.deliverStopped(run)
	}
	return err
}

// shutdown cancels one run, closes its socket, and waits for it to exit.
func (s *Stream) shutdown(ctx context.Context, run *streamRun) error {
	run.cancel()
	s.mu.Lock()
	conn := run.conn
	if run.dataTimer != nil {
		run.dataTimer.Stop()
	}
	s.mu.Unlock()
	var closeErr error
	if conn != nil {
		closeErr = conn.Close(websocket.StatusNormalClosure, "")
	}
	select {
	case <-run.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
		return newError(CodeConnectionLost, "websocket did not close cleanly: "+closeErr.Error())
	}
	return nil
}

// SetToken replaces the remote token. When connected, it re-authenticates at once.
func (s *Stream) SetToken(ctx context.Context, token string) error {
	s.mu.Lock()
	changed := s.token != token
	s.token = token
	run := s.current
	var conn *websocket.Conn
	if run != nil {
		conn = run.conn
	}
	s.mu.Unlock()
	if !s.remote || conn == nil || !changed {
		return nil
	}
	s.patch(run, func(st *Status) { st.Auth = Authenticating })
	return writeJSON(ctx, conn, map[string]string{"token": token})
}

// Subscribe asks the cloud for updates of one device. Subscriptions survive
// reconnects. Remote only.
func (s *Stream) Subscribe(ctx context.Context, guid string) error {
	s.mu.Lock()
	_, had := s.subs[guid]
	s.subs[guid] = struct{}{}
	var conn *websocket.Conn
	if s.current != nil {
		conn = s.current.conn
	}
	s.mu.Unlock()
	if had || conn == nil || !s.remote {
		return nil
	}
	return writeJSON(ctx, conn, map[string][]string{"subscribe": {guid}})
}

// Unsubscribe stops updates of one device. Remote only.
func (s *Stream) Unsubscribe(ctx context.Context, guid string) error {
	s.mu.Lock()
	_, had := s.subs[guid]
	delete(s.subs, guid)
	var conn *websocket.Conn
	if s.current != nil {
		conn = s.current.conn
	}
	s.mu.Unlock()
	if !had || conn == nil || !s.remote {
		return nil
	}
	return writeJSON(ctx, conn, map[string][]string{"unsubscribe": {guid}})
}

func (s *Stream) run(ctx context.Context, run *streamRun, ready chan<- error) {
	var once sync.Once
	signal := func(err error) { once.Do(func() { ready <- err }) }
	defer func() {
		if ctx.Err() != nil {
			signal(ctx.Err())
		}
		close(run.done)
	}()

	connectedOnce := false
	retries, authRetries := 0, 0
	for {
		s.patch(run, func(st *Status) { st.Connection = Connecting })
		conn, dialErr := s.dial(ctx)
		closeCode := websocket.StatusCode(-1)
		var readErr error
		if dialErr == nil {
			if !s.adopt(ctx, run, conn) {
				_ = conn.Close(websocket.StatusNormalClosure, "")
				return
			}
			connectedOnce = true
			retries, authRetries = 0, 0
			s.onOpen(ctx, run, conn, signal)
			closeCode, readErr = s.readLoop(run, conn, signal)
			s.clearConn(run, conn)
		} else {
			if ctx.Err() != nil {
				return
			}
			e := newError(CodeConnectionFailed, "websocket connection error: "+dialErr.Error())
			s.mapErrorToStatus(run, e)
			s.emitError(run, e)
			if !connectedOnce {
				signal(e)
				return
			}
		}
		if ctx.Err() != nil || errors.Is(readErr, errFatalDeviceError) {
			return
		}

		switch {
		case s.remote && closeCode == authCloseCode:
			authRetries++
			if authRetries > s.opts.MaxAuthAttempts {
				e := newError(CodeAuthFailed, fmt.Sprintf("maximum authentication attempts (%d) reached", s.opts.MaxAuthAttempts))
				s.mapErrorToStatus(run, e)
				s.emitError(run, e)
				signal(e)
				return
			}
			s.patch(run, func(st *Status) { st.Auth = Reauthenticating; st.AuthAttempts = authRetries })
			if s.opts.TokenProvider == nil {
				e := newError(CodeAuthFailed, "token rejected and no TokenProvider configured")
				s.mapErrorToStatus(run, e)
				s.emitError(run, e)
				signal(e)
				return
			}
			token, err := s.opts.TokenProvider(ctx)
			if err != nil {
				e := newError(CodeAuthRefreshFailed, "failed to refresh token: "+err.Error())
				s.mapErrorToStatus(run, e)
				s.emitError(run, e)
				signal(e)
				return
			}
			s.mu.Lock()
			if s.current == run {
				s.token = token
			}
			s.mu.Unlock()
		case closeCode == websocket.StatusNormalClosure:
			s.patch(run, func(st *Status) { st.Connection = Disconnected })
			e := newError(CodeConnectionLost, "stream closed by the server")
			s.mapErrorToStatus(run, e)
			s.emitError(run, e)
			signal(e)
			return
		default:
			retries++
			if retries > s.opts.MaxReconnectAttempts {
				e := newError(CodeReconnectFailed, fmt.Sprintf("maximum reconnection attempts (%d) reached", s.opts.MaxReconnectAttempts))
				s.mapErrorToStatus(run, e)
				s.emitError(run, e)
				signal(e)
				return
			}
			s.patch(run, func(st *Status) { st.Connection = Reconnecting; st.ConnectionAttempts = retries })
			select {
			case <-time.After(s.opts.ReconnectDelay):
			case <-ctx.Done():
				return
			}
		}
	}
}

func (s *Stream) dial(ctx context.Context) (*websocket.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, s.opts.ConnectTimeout)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, s.url, &websocket.DialOptions{HTTPClient: s.opts.HTTPClient})
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(1 << 20)
	return conn, nil
}

func (s *Stream) onOpen(ctx context.Context, run *streamRun, conn *websocket.Conn, signal func(error)) {
	s.patch(run, func(st *Status) { st.Connection = Connected; st.ConnectionAttempts = 0 })
	if !s.remote {
		_ = writeJSON(ctx, conn, map[string]bool{"enable": true})
		s.patch(run, func(st *Status) { st.Auth = Authenticated; st.Main = Running })
		signal(nil)
		return
	}
	s.mu.Lock()
	token := s.token
	subs := make([]string, 0, len(s.subs))
	for g := range s.subs {
		subs = append(subs, g)
	}
	s.mu.Unlock()
	if token != "" {
		s.patch(run, func(st *Status) { st.Auth = Authenticating })
		_ = writeJSON(ctx, conn, map[string]string{"token": token})
	}
	if len(subs) > 0 {
		_ = writeJSON(ctx, conn, map[string][]string{"subscribe": subs})
	}
}

// readLoop delivers messages until the connection ends. It returns the close
// code (or -1 when the connection did not end with a close frame). The read
// has no cancellable context on purpose: cancelling one force-closes the
// socket without a close handshake. shutdown closes the socket instead, and
// that wakes the read.
func (s *Stream) readLoop(run *streamRun, conn *websocket.Conn, signal func(error)) (websocket.StatusCode, error) {
	authReported := false
	for {
		typ, data, err := conn.Read(context.Background())
		if err != nil {
			return websocket.CloseStatus(err), err
		}
		s.touchData(run)
		s.emit(run, func(cb Callbacks) {
			if cb.RawData != nil {
				cb.RawData(data)
			}
		})
		if s.handleMessage(run, typ, data, &authReported, signal) {
			_ = conn.Close(websocket.StatusInternalError, "fatal device error")
			return -1, errFatalDeviceError
		}
	}
}

// handleMessage decodes one message and reports true for a fatal device error.
func (s *Stream) handleMessage(run *streamRun, typ websocket.MessageType, data []byte, authReported *bool, signal func(error)) bool {
	payload, barID := data, ""
	if typ == websocket.MessageText {
		var env struct {
			Type    string          `json:"type"`
			BarID   string          `json:"bar_id"`
			BarIDCC string          `json:"barId"`
			State   json.RawMessage `json:"state"`
			Device  RemoteDevice    `json:"device"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			s.emitError(run, &Error{Code: CodeDecodeError, Message: "decode error: " + err.Error(), Data: data})
			return false
		}
		if strings.HasPrefix(env.Type, "device.") {
			s.emit(run, func(cb Callbacks) {
				if cb.DeviceEvent != nil {
					cb.DeviceEvent(DeviceEvent{Type: env.Type, Device: env.Device})
				}
			})
			return false
		}
		barID = env.BarID
		if barID == "" {
			barID = env.BarIDCC
		}
		var err error
		if payload, err = decodeEnvelopeState(env.State); err != nil {
			s.emitError(run, &Error{Code: CodeDecodeError, Message: "decode error: " + err.Error(), Data: data})
			return false
		}
		if s.remote && !*authReported {
			*authReported = true
			s.patch(run, func(st *Status) { st.Auth = Authenticated; st.Main = Running })
			signal(nil)
		}
	}
	if payload == nil {
		return false
	}
	var raw pb.State
	if err := proto.Unmarshal(payload, &raw); err != nil {
		s.emitError(run, &Error{Code: CodeDecodeError, Message: "decode error: " + err.Error(), Data: data})
		return false
	}
	if raw.Error != nil {
		e := &Error{
			Code:    CodeDeviceError,
			Message: fmt.Sprintf("device reported %s: %s", raw.Error.GetSeverity(), raw.Error.GetCause()),
			Data:    raw.Error,
		}
		s.mapErrorToStatus(run, e)
		fatal := raw.Error.GetSeverity() == pb.Severity_FATAL
		if fatal {
			s.patch(run, func(st *Status) { st.Main = Failed })
		}
		s.emitError(run, e)
		switch {
		case fatal:
			signal(e)
			return true
		case raw.Error.GetSeverity() == pb.Severity_ERROR:
			return false
		}
	}
	state := processState(&raw, barID, func(err error) {
		s.emitError(run, &Error{Code: CodeFrameProcessError, Message: err.Error()})
	})
	s.emit(run, func(cb Callbacks) {
		if cb.Data != nil {
			cb.Data(state)
		}
	})
	return false
}

// decodeEnvelopeState accepts the cloud's state field as base64 text or as a
// JSON byte array.
func decodeEnvelopeState(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var b64 string
	if err := json.Unmarshal(raw, &b64); err == nil {
		return base64.StdEncoding.DecodeString(b64)
	}
	var nums []byte
	var ints []int
	if err := json.Unmarshal(raw, &ints); err != nil {
		return nil, err
	}
	for _, n := range ints {
		nums = append(nums, byte(n))
	}
	return nums, nil
}

func writeJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(wctx, websocket.MessageText, b)
}

// adopt stores a freshly dialed connection. It reports false when shutdown
// already ran, in which case the caller owns closing conn. The check and the
// store happen under one lock, so exactly one side closes the socket.
func (s *Stream) adopt(ctx context.Context, run *streamRun, conn *websocket.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || s.current != run {
		return false
	}
	run.conn = conn
	return true
}

func (s *Stream) clearConn(run *streamRun, conn *websocket.Conn) {
	s.mu.Lock()
	if run.conn == conn {
		run.conn = nil
	}
	s.mu.Unlock()
}

// emit adds one callback to the stream's serialized callback queue.
func (s *Stream) emit(run *streamRun, invoke func(Callbacks)) {
	s.mu.Lock()
	if s.current == run {
		s.queueCallbackLocked(callbackEvent{run: run, invoke: invoke})
	}
	s.mu.Unlock()
}

func (s *Stream) queueStatusLocked(run *streamRun, status Status) {
	if run.cb.Status == nil {
		return
	}
	s.queueCallbackLocked(callbackEvent{
		run:    run,
		invoke: func(cb Callbacks) { cb.Status(status) },
	})
}

func (s *Stream) queueCallbackLocked(event callbackEvent) {
	s.callbackMu.Lock()
	s.callbackQueue = append(s.callbackQueue, event)
	if !s.callbackWorker {
		s.callbackWorker = true
		go s.runCallbacks()
	}
	s.callbackMu.Unlock()
}

func (s *Stream) runCallbacks() {
	for {
		s.callbackMu.Lock()
		if len(s.callbackQueue) == 0 {
			s.callbackWorker = false
			s.callbackMu.Unlock()
			return
		}
		event := s.callbackQueue[0]
		s.callbackQueue = s.callbackQueue[1:]
		s.callbackRunning = event.run
		s.callbackMu.Unlock()

		s.mu.Lock()
		active := s.current == event.run
		if event.final {
			active = s.current == nil && s.generation == event.run.generation && s.status.Main == Stopped
		}
		s.mu.Unlock()
		if active {
			event.invoke(event.run.cb)
		}
		if event.done != nil {
			close(event.done)
		}

		s.callbackMu.Lock()
		s.callbackRunning = nil
		s.callbackMu.Unlock()
	}
}

func (s *Stream) cancelCallbacks(run *streamRun) {
	s.callbackMu.Lock()
	kept := s.callbackQueue[:0]
	for _, event := range s.callbackQueue {
		if event.run != run {
			kept = append(kept, event)
		}
	}
	s.callbackQueue = kept
	s.callbackMu.Unlock()
}

// deliverStopped reports the final status unless a callback from this run is
// executing or a newer run has started.
func (s *Stream) deliverStopped(run *streamRun) {
	s.mu.Lock()
	if s.current != nil || s.generation != run.generation || run.cb.Status == nil {
		s.mu.Unlock()
		return
	}
	s.callbackMu.Lock()
	if s.callbackRunning == run {
		s.callbackMu.Unlock()
		s.mu.Unlock()
		return
	}
	status := s.status
	done := make(chan struct{})
	s.callbackQueue = append(s.callbackQueue, callbackEvent{
		run:    run,
		final:  true,
		invoke: func(cb Callbacks) { cb.Status(status) },
		done:   done,
	})
	if !s.callbackWorker {
		s.callbackWorker = true
		go s.runCallbacks()
	}
	s.callbackMu.Unlock()
	s.mu.Unlock()
	<-done
}

// patch changes the status only when run is still current.
func (s *Stream) patch(run *streamRun, fn func(*Status)) {
	s.mu.Lock()
	if s.current != run {
		s.mu.Unlock()
		return
	}
	fn(&s.status)
	s.queueStatusLocked(run, s.status)
	s.mu.Unlock()
}

func (s *Stream) emitError(run *streamRun, e *Error) {
	s.emit(run, func(cb Callbacks) {
		if cb.Error != nil {
			cb.Error(e)
		}
	})
}

func (s *Stream) mapErrorToStatus(run *streamRun, e *Error) {
	s.patch(run, func(st *Status) {
		switch e.Code {
		case CodeConnectionFailed, CodeConnectionLost, CodeReconnectFailed, CodeConnectionTimeout:
			st.Connection, st.ConnectionError = Disconnected, e
			st.Main, st.MainError = Failed, e
		case CodeAuthFailed, CodeAuthRefreshFailed:
			st.Auth, st.AuthError = AuthFailed, e
			st.Main, st.MainError = Failed, e
		case CodeDeviceError, CodeDecodeError:
			st.MainError = e
		}
	})
}

// touchData marks data as active and arms the stale timer. The status
// callback fires only when the state changes, not on every message.
func (s *Stream) touchData(run *streamRun) {
	s.mu.Lock()
	if s.current != run {
		s.mu.Unlock()
		return
	}
	changed := s.status.Data != DataActive
	s.status.Data, s.status.LastActivity = DataActive, time.Now()
	if run.dataTimer != nil {
		run.dataTimer.Stop()
	}
	var timer *time.Timer
	timer = time.AfterFunc(s.opts.DataTimeout, func() {
		s.mu.Lock()
		if s.current == run && run.dataTimer == timer && s.status.Data == DataActive {
			s.status.Data = DataStale
			s.queueStatusLocked(run, s.status)
		}
		s.mu.Unlock()
	})
	run.dataTimer = timer
	if changed {
		s.queueStatusLocked(run, s.status)
	}
	s.mu.Unlock()
}
