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

	// ConnectTimeout bounds each dial and each remote authentication handshake,
	// including handshakes after reconnecting.
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

// Callbacks receive stream events. Every field is optional. Run invokes them
// serially in its goroutine, so they must return promptly. Run checks its
// context between callbacks. External cancellation can race with a callback
// that is about to start. No callback fires after Run returns.
type Callbacks struct {
	// Ready is called once per Run when the stream is running: for a local
	// stream when the socket is open, for a remote stream after the first
	// authenticated message. It is not called again after a reconnect.
	Ready   func()
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

	mu      sync.Mutex
	running bool
	status  Status

	// commandMu orders token/subscription changes, connection publication, and writes.
	// No user callback runs while it is held.
	commandMu sync.Mutex
	token     string
	conn      *websocket.Conn
	subs      map[string]struct{}
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

// Run connects and blocks while the stream is active. It calls
// Callbacks.Ready once the stream is running: a local stream when the socket
// opens, a remote stream after its first authenticated state message. Run
// reconnects until the context is cancelled, a fatal error occurs, or a retry
// budget is used up. It returns ctx.Err() when cancellation stops it. An error
// already produced by the stream takes precedence. Only one call can run at a
// time; a second one returns CodeStreamAlreadyStarted.
func (s *Stream) Run(ctx context.Context, cb Callbacks) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.begin(); err != nil {
		return err
	}
	defer s.finish()
	em := &emitter{s: s, cb: cb}
	if cb.Status != nil {
		cb.Status(s.Status())
	}
	err := s.run(ctx, em)
	if ctxErr := ctx.Err(); ctxErr != nil {
		// run returns ctx.Err() directly for cancellation. A stream error may
		// wrap the same cause, but an already reported failure takes precedence.
		if err != nil && err != ctxErr {
			return err
		}
		em.report(func(st *Status) {
			st.Main, st.Connection, st.Auth, st.Data = Stopped, Disconnected, Unauthenticated, DataNone
			st.ConnectionAttempts, st.AuthAttempts = 0, 0
		})
		return ctxErr
	}
	return err
}

// run is the connect and reconnect loop. It returns whatever ended the loop;
// Run turns a cancellation into ctx.Err() and reports the Stopped status.
func (s *Stream) run(ctx context.Context, em *emitter) error {
	connectedOnce := false
	retries, authRetries := 0, 0
	for {
		em.patch(ctx, func(st *Status) { st.Connection = Connecting })
		conn, dialErr := s.dial(ctx)
		if dialErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			e := s.dialError(dialErr)
			em.fail(ctx, e)
			if !connectedOnce {
				return e
			}
			retries++
			if err := s.waitToReconnect(ctx, em, retries); err != nil {
				return err
			}
			continue
		}
		connectedOnce = true
		retries = 0
		s.onOpen(ctx, em, conn)
		res := s.serve(ctx, em, conn)
		s.commandMu.Lock()
		s.conn = nil
		s.commandMu.Unlock()
		if res.fatal != nil {
			return res.fatal
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if res.authenticated {
			authRetries = 0
		}

		closeCode := websocket.CloseStatus(res.err)
		switch {
		case s.remote && closeCode == authCloseCode:
			authRetries++
			if authRetries > s.opts.MaxAuthAttempts {
				e := newError(CodeAuthFailed, fmt.Sprintf("maximum authentication attempts (%d) reached", s.opts.MaxAuthAttempts))
				em.fail(ctx, e)
				return e
			}
			em.patch(ctx, func(st *Status) {
				st.Connection, st.Auth, st.Data = Reconnecting, Reauthenticating, DataNone
				st.AuthAttempts = authRetries
			})
			if err := ctx.Err(); err != nil {
				return err
			}
			if s.opts.TokenProvider == nil {
				e := newError(CodeAuthFailed, "token rejected and no TokenProvider configured")
				em.fail(ctx, e)
				return e
			}
			token, err := s.opts.TokenProvider(ctx)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if err != nil {
				e := &Error{Code: CodeAuthRefreshFailed, Message: "failed to refresh token: " + err.Error(), Cause: err}
				em.fail(ctx, e)
				return e
			}
			s.commandMu.Lock()
			s.token = token
			s.commandMu.Unlock()
		case closeCode == websocket.StatusNormalClosure:
			e := &Error{Code: CodeConnectionLost, Message: "stream closed by the server", Cause: res.err}
			em.fail(ctx, e)
			return e
		default:
			retries++
			if err := s.waitToReconnect(ctx, em, retries); err != nil {
				return err
			}
		}
	}
}

func (s *Stream) begin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return newError(CodeStreamAlreadyStarted, "stream is already running")
	}
	s.running = true
	s.status = initialStatus()
	s.status.Main = Starting
	return nil
}

// finish releases the Run. run clears the connection itself; the close here
// only matters when a callback panic unwinds past that point.
func (s *Stream) finish() {
	s.commandMu.Lock()
	if s.conn != nil {
		_ = s.conn.CloseNow()
		s.conn = nil
	}
	s.commandMu.Unlock()
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

func (s *Stream) dialError(err error) *Error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Code: CodeConnectionTimeout, Message: fmt.Sprintf("connection timed out after %s", s.opts.ConnectTimeout), Cause: err}
	}
	return &Error{Code: CodeConnectionFailed, Message: "websocket connection error: " + err.Error(), Cause: err}
}

// waitToReconnect reports attempt and sleeps for the reconnect delay. It
// returns CodeReconnectFailed when the budget is used up.
func (s *Stream) waitToReconnect(ctx context.Context, em *emitter, attempt int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if attempt > s.opts.MaxReconnectAttempts {
		e := newError(CodeReconnectFailed, fmt.Sprintf("maximum reconnection attempts (%d) reached", s.opts.MaxReconnectAttempts))
		em.fail(ctx, e)
		return e
	}
	em.patch(ctx, func(st *Status) {
		st.Connection, st.Data = Reconnecting, DataNone
		st.ConnectionAttempts = attempt
	})
	select {
	case <-time.After(s.opts.ReconnectDelay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SetToken replaces the remote token. When connected, it sends the token at once.
func (s *Stream) SetToken(ctx context.Context, token string) error {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	changed := s.token != token
	s.token = token
	conn := s.conn
	if !s.remote || conn == nil || !changed {
		return nil
	}
	return writeJSON(ctx, conn, map[string]string{"token": token})
}

// Subscribe asks the cloud for updates of one device. Subscriptions survive
// reconnects. Remote only.
func (s *Stream) Subscribe(ctx context.Context, guid string) error {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	_, had := s.subs[guid]
	s.subs[guid] = struct{}{}
	conn := s.conn
	if had || conn == nil || !s.remote {
		return nil
	}
	return writeJSON(ctx, conn, map[string][]string{"subscribe": {guid}})
}

// Unsubscribe stops updates of one device. Remote only.
func (s *Stream) Unsubscribe(ctx context.Context, guid string) error {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	_, had := s.subs[guid]
	delete(s.subs, guid)
	conn := s.conn
	if !had || conn == nil || !s.remote {
		return nil
	}
	return writeJSON(ctx, conn, map[string][]string{"unsubscribe": {guid}})
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

func (s *Stream) onOpen(ctx context.Context, em *emitter, conn *websocket.Conn) {
	s.commandMu.Lock()
	s.conn = conn
	hasToken := s.token != ""
	if !s.remote {
		_ = writeJSON(ctx, conn, map[string]bool{"enable": true})
	} else {
		if hasToken {
			_ = writeJSON(ctx, conn, map[string]string{"token": s.token})
		}
		if len(s.subs) > 0 {
			subs := make([]string, 0, len(s.subs))
			for guid := range s.subs {
				subs = append(subs, guid)
			}
			_ = writeJSON(ctx, conn, map[string][]string{"subscribe": subs})
		}
	}
	s.commandMu.Unlock()

	em.patch(ctx, func(st *Status) { st.Connection = Connected; st.ConnectionAttempts = 0 })
	if !s.remote {
		em.running(ctx, func(st *Status) { st.Auth = Authenticated })
	} else if hasToken {
		em.patch(ctx, func(st *Status) { st.Auth = Authenticating })
	}
}

type readResult struct {
	typ  websocket.MessageType
	data []byte
	err  error
}

// connResult describes how one connection ended. err preserves the read error,
// including the server close code and reason.
type connResult struct {
	err           error
	fatal         *Error
	authenticated bool
}

// serve delivers one connection's messages until it ends. The reader
// goroutine only performs WebSocket reads and hands the results over here,
// so every callback and timer runs on the Run goroutine.
func (s *Stream) serve(ctx context.Context, em *emitter, conn *websocket.Conn) connResult {
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()
	reads := make(chan readResult, 1)
	go readMessages(readCtx, conn, reads)

	// A stopped timer never fires, so the first message arms it.
	dataTimer := time.NewTimer(s.opts.DataTimeout)
	dataTimer.Stop()
	defer dataTimer.Stop()

	var authC <-chan time.Time
	if s.remote {
		authTimer := time.NewTimer(s.opts.ConnectTimeout)
		defer authTimer.Stop()
		authC = authTimer.C
	}
	authenticated := false

	for {
		select {
		case <-ctx.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return connResult{authenticated: authenticated}
		case r := <-reads:
			if r.err != nil {
				return connResult{err: r.err, authenticated: authenticated}
			}
			dataTimer.Reset(s.opts.DataTimeout)
			s.touchData(ctx, em)
			em.raw(ctx, r.data)
			fatal, didAuthenticate := s.handleMessage(ctx, em, r.typ, r.data, authenticated)
			if didAuthenticate {
				authenticated = true
				authC = nil
			}
			if fatal != nil {
				_ = conn.Close(websocket.StatusInternalError, "fatal device error")
				return connResult{fatal: fatal, authenticated: authenticated}
			}
		case <-dataTimer.C:
			em.patch(ctx, func(st *Status) {
				if st.Data == DataActive {
					st.Data = DataStale
				}
			})
		case <-authC:
			e := &Error{Code: CodeConnectionTimeout, Message: fmt.Sprintf("connection timed out after %s", s.opts.ConnectTimeout), Cause: context.DeadlineExceeded}
			em.fail(ctx, e)
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return connResult{fatal: e, authenticated: authenticated}
		}
	}
}

func readMessages(ctx context.Context, conn *websocket.Conn, results chan<- readResult) {
	for {
		typ, data, err := conn.Read(context.Background())
		select {
		case results <- readResult{typ: typ, data: data, err: err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}

// handleMessage decodes one message. It returns a fatal device error and
// whether this message completed remote authentication.
func (s *Stream) handleMessage(ctx context.Context, em *emitter, typ websocket.MessageType, data []byte, authReported bool) (fatal *Error, authenticated bool) {
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
			em.error(ctx, &Error{Code: CodeDecodeError, Message: "decode error: " + err.Error(), Cause: err, Data: data})
			return nil, false
		}
		if strings.HasPrefix(env.Type, "device.") {
			em.device(ctx, DeviceEvent{Type: env.Type, Device: env.Device})
			return nil, false
		}
		barID = env.BarID
		if barID == "" {
			barID = env.BarIDCC
		}
		var err error
		if payload, err = decodeEnvelopeState(env.State); err != nil {
			em.error(ctx, &Error{Code: CodeDecodeError, Message: "decode error: " + err.Error(), Cause: err, Data: data})
			return nil, false
		}
		if !authReported {
			em.running(ctx, func(st *Status) { st.Auth = Authenticated })
		}
		authenticated = true
	}
	if payload == nil {
		return nil, authenticated
	}
	var raw pb.State
	if err := proto.Unmarshal(payload, &raw); err != nil {
		em.error(ctx, &Error{Code: CodeDecodeError, Message: "decode error: " + err.Error(), Cause: err, Data: data})
		return nil, authenticated
	}
	if raw.Error != nil {
		e := &Error{
			Code:    CodeDeviceError,
			Message: fmt.Sprintf("device reported %s: %s", raw.Error.GetSeverity(), raw.Error.GetCause()),
			Data:    raw.Error,
		}
		fatal := raw.Error.GetSeverity() == pb.Severity_FATAL
		updateStatus := func(st *Status) {
			applyError(st, e)
			if fatal {
				st.Main = Failed
				st.Connection, st.Data = Disconnected, DataNone
			}
		}
		if fatal {
			em.report(updateStatus)
		} else {
			em.patch(ctx, updateStatus)
		}
		em.error(ctx, e)
		switch {
		case fatal:
			return e, authenticated
		case raw.Error.GetSeverity() == pb.Severity_ERROR:
			return nil, authenticated
		}
	}
	state := processState(&raw, barID, func(err *Error) {
		em.error(ctx, err)
	})
	em.data(ctx, state)
	return nil, authenticated
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

// touchData marks data active. Only a change is reported, not every message.
func (s *Stream) touchData(ctx context.Context, em *emitter) {
	if ctx.Err() != nil {
		return
	}
	s.mu.Lock()
	changed := s.status.Data != DataActive
	s.status.Data, s.status.LastActivity = DataActive, time.Now()
	snapshot := s.status
	s.mu.Unlock()
	if changed && em.cb.Status != nil {
		em.cb.Status(snapshot)
	}
}

func applyError(st *Status, e *Error) {
	switch e.Code {
	case CodeConnectionFailed, CodeConnectionLost, CodeReconnectFailed, CodeConnectionTimeout:
		st.Connection, st.ConnectionError = Disconnected, e
		st.Main, st.MainError = Failed, e
		st.Data = DataNone
	case CodeAuthFailed, CodeAuthRefreshFailed:
		st.Auth, st.AuthError = AuthFailed, e
		st.Main, st.MainError = Failed, e
		st.Connection, st.Data = Disconnected, DataNone
	case CodeDeviceError, CodeDecodeError:
		st.MainError = e
	}
}

// emitter delivers callbacks for one Run. report always changes the status
// and calls Status, even after cancellation, so an error the stream has
// already produced is reflected. patch is dropped after cancellation.
type emitter struct {
	s     *Stream
	cb    Callbacks
	ready bool
}

func (e *emitter) report(fn func(*Status)) {
	e.s.mu.Lock()
	fn(&e.s.status)
	snapshot := e.s.status
	e.s.mu.Unlock()
	if e.cb.Status != nil {
		e.cb.Status(snapshot)
	}
}

func (e *emitter) patch(ctx context.Context, fn func(*Status)) {
	if ctx.Err() == nil {
		e.report(fn)
	}
}

// running marks the stream running and calls Ready the first time per Run.
func (e *emitter) running(ctx context.Context, fn func(*Status)) {
	e.patch(ctx, func(st *Status) { fn(st); st.Main = Running })
	if e.ready || ctx.Err() != nil {
		return
	}
	e.ready = true
	if e.cb.Ready != nil {
		e.cb.Ready()
	}
}

func (e *emitter) fail(ctx context.Context, err *Error) {
	e.report(func(st *Status) { applyError(st, err) })
	e.error(ctx, err)
}

func (e *emitter) error(ctx context.Context, err *Error) {
	if ctx.Err() == nil && e.cb.Error != nil {
		e.cb.Error(err)
	}
}

func (e *emitter) data(ctx context.Context, st *State) {
	if ctx.Err() == nil && e.cb.Data != nil {
		e.cb.Data(st)
	}
}

func (e *emitter) raw(ctx context.Context, b []byte) {
	if ctx.Err() == nil && e.cb.RawData != nil {
		e.cb.RawData(b)
	}
}

func (e *emitter) device(ctx context.Context, ev DeviceEvent) {
	if ctx.Err() == nil && e.cb.DeviceEvent != nil {
		e.cb.DeviceEvent(ev)
	}
}
