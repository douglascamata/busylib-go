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
// serially in its goroutine. They must return promptly. A callback can cancel
// Run's context.
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

	mu      sync.Mutex
	running bool
	status  Status
	token   string
	conn    *websocket.Conn
	subs    map[string]struct{}
}

type readResult struct {
	typ  websocket.MessageType
	data []byte
	err  error
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

// Run connects and blocks while the stream is active. A local stream is ready
// when the socket opens. A remote stream is ready after its first authenticated
// state message. Run reconnects until the context is cancelled, a fatal error
// occurs, or a retry budget is used up. It returns ctx.Err() after cancellation.
// Only one call can run at a time.
func (s *Stream) Run(ctx context.Context, cb Callbacks) error {
	if err := s.begin(); err != nil {
		return err
	}
	defer s.finish()
	if cb.Status != nil {
		cb.Status(s.Status())
	}

	connectedOnce := false
	ready := false
	retries, authRetries := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			s.markStopped(cb)
			return err
		}

		s.patch(cb, func(st *Status) { st.Connection = Connecting })
		conn, dialErr := s.dial(ctx)
		if dialErr != nil {
			if err := ctx.Err(); err != nil {
				s.markStopped(cb)
				return err
			}
			e := newError(CodeConnectionFailed, "websocket connection error: "+dialErr.Error())
			s.fail(cb, e)
			if !connectedOnce {
				return e
			}
			if e := s.waitToReconnect(ctx, cb, &retries); e != nil {
				return e
			}
			continue
		}
		if err := ctx.Err(); err != nil {
			_ = conn.Close(websocket.StatusNormalClosure, "")
			s.markStopped(cb)
			return err
		}
		s.mu.Lock()
		s.conn = conn
		s.mu.Unlock()

		connectedOnce = true
		retries = 0
		s.onOpen(ctx, cb, conn)
		if err := ctx.Err(); err != nil {
			s.markStopped(cb)
			return err
		}
		closeCode, fatalErr, authenticated := s.serve(ctx, cb, conn, s.remote && !ready)
		s.mu.Lock()
		s.conn = nil
		s.mu.Unlock()
		if authenticated {
			ready = true
			authRetries = 0
		}
		if err := ctx.Err(); err != nil {
			s.markStopped(cb)
			return err
		}
		if fatalErr != nil {
			return fatalErr
		}

		switch {
		case s.remote && closeCode == authCloseCode:
			authRetries++
			if authRetries > s.opts.MaxAuthAttempts {
				e := newError(CodeAuthFailed, fmt.Sprintf("maximum authentication attempts (%d) reached", s.opts.MaxAuthAttempts))
				s.fail(cb, e)
				return e
			}
			s.patch(cb, func(st *Status) { st.Auth = Reauthenticating; st.AuthAttempts = authRetries })
			if err := ctx.Err(); err != nil {
				s.markStopped(cb)
				return err
			}
			if s.opts.TokenProvider == nil {
				e := newError(CodeAuthFailed, "token rejected and no TokenProvider configured")
				s.fail(cb, e)
				return e
			}
			token, err := s.opts.TokenProvider(ctx)
			if ctxErr := ctx.Err(); ctxErr != nil {
				s.markStopped(cb)
				return ctxErr
			}
			if err != nil {
				e := newError(CodeAuthRefreshFailed, "failed to refresh token: "+err.Error())
				s.fail(cb, e)
				return e
			}
			s.mu.Lock()
			s.token = token
			s.mu.Unlock()
		case closeCode == websocket.StatusNormalClosure:
			e := newError(CodeConnectionLost, "stream closed by the server")
			s.fail(cb, e)
			return e
		default:
			if e := s.waitToReconnect(ctx, cb, &retries); e != nil {
				return e
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

func (s *Stream) finish() {
	s.mu.Lock()
	conn := s.conn
	s.conn = nil
	s.running = false
	s.mu.Unlock()
	if conn != nil {
		_ = conn.CloseNow()
	}
}

func (s *Stream) markStopped(cb Callbacks) {
	s.patch(cb, func(st *Status) {
		st.Main, st.Connection, st.Auth, st.Data = Stopped, Disconnected, Unauthenticated, DataNone
		st.ConnectionAttempts, st.AuthAttempts = 0, 0
	})
}

func (s *Stream) waitToReconnect(ctx context.Context, cb Callbacks, retries *int) *Error {
	if ctx.Err() != nil {
		return nil
	}
	(*retries)++
	if *retries > s.opts.MaxReconnectAttempts {
		e := newError(CodeReconnectFailed, fmt.Sprintf("maximum reconnection attempts (%d) reached", s.opts.MaxReconnectAttempts))
		s.fail(cb, e)
		return e
	}
	s.patch(cb, func(st *Status) { st.Connection = Reconnecting; st.ConnectionAttempts = *retries })
	timer := time.NewTimer(s.opts.ReconnectDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return nil
	}
}

// SetToken replaces the remote token. When connected, it sends the token at once.
func (s *Stream) SetToken(ctx context.Context, token string) error {
	s.mu.Lock()
	changed := s.token != token
	s.token = token
	conn := s.conn
	s.mu.Unlock()
	if !s.remote || conn == nil || !changed {
		return nil
	}
	return writeJSON(ctx, conn, map[string]string{"token": token})
}

// Subscribe asks the cloud for updates of one device. Subscriptions survive
// reconnects. Remote only.
func (s *Stream) Subscribe(ctx context.Context, guid string) error {
	s.mu.Lock()
	_, had := s.subs[guid]
	s.subs[guid] = struct{}{}
	conn := s.conn
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
	conn := s.conn
	s.mu.Unlock()
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

func (s *Stream) onOpen(ctx context.Context, cb Callbacks, conn *websocket.Conn) {
	s.patch(cb, func(st *Status) { st.Connection = Connected; st.ConnectionAttempts = 0 })
	if ctx.Err() != nil {
		return
	}
	if !s.remote {
		_ = writeJSON(ctx, conn, map[string]bool{"enable": true})
		if ctx.Err() != nil {
			return
		}
		s.patch(cb, func(st *Status) { st.Auth = Authenticated; st.Main = Running })
		return
	}
	s.mu.Lock()
	token := s.token
	subs := make([]string, 0, len(s.subs))
	for guid := range s.subs {
		subs = append(subs, guid)
	}
	s.mu.Unlock()
	if token != "" {
		s.patch(cb, func(st *Status) { st.Auth = Authenticating })
		if ctx.Err() != nil {
			return
		}
		_ = writeJSON(ctx, conn, map[string]string{"token": token})
	}
	if len(subs) > 0 {
		_ = writeJSON(ctx, conn, map[string][]string{"subscribe": subs})
	}
}

// serve owns callback delivery and timers for one connection. The reader
// goroutine only performs WebSocket reads and sends their results here.
func (s *Stream) serve(ctx context.Context, cb Callbacks, conn *websocket.Conn, waitForAuth bool) (websocket.StatusCode, *Error, bool) {
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()
	reads := make(chan readResult, 1)
	go readMessages(readCtx, conn, reads)

	var dataTimer *time.Timer
	var dataC <-chan time.Time
	defer func() {
		if dataTimer != nil {
			dataTimer.Stop()
		}
	}()

	var authTimer *time.Timer
	var authC <-chan time.Time
	if waitForAuth {
		authTimer = time.NewTimer(s.opts.ConnectTimeout)
		authC = authTimer.C
		defer authTimer.Stop()
	}
	authenticated := false

	for {
		select {
		case <-ctx.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return -1, nil, authenticated
		case result := <-reads:
			if result.err != nil {
				return websocket.CloseStatus(result.err), nil, authenticated
			}
			if dataTimer == nil {
				dataTimer = time.NewTimer(s.opts.DataTimeout)
			} else {
				if !dataTimer.Stop() {
					select {
					case <-dataTimer.C:
					default:
					}
				}
				dataTimer.Reset(s.opts.DataTimeout)
			}
			dataC = dataTimer.C
			s.touchData(cb)
			if ctx.Err() != nil {
				continue
			}
			if cb.RawData != nil {
				cb.RawData(result.data)
			}
			if ctx.Err() != nil {
				continue
			}
			fatal, didAuthenticate := s.handleMessage(ctx, cb, result.typ, result.data, authenticated)
			if didAuthenticate && !authenticated {
				authenticated = true
				if authTimer != nil {
					authTimer.Stop()
					authC = nil
				}
			}
			if fatal != nil {
				_ = conn.Close(websocket.StatusInternalError, "fatal device error")
				return -1, fatal, authenticated
			}
		case <-dataC:
			dataC = nil
			s.patch(cb, func(st *Status) {
				if st.Data == DataActive {
					st.Data = DataStale
				}
			})
		case <-authC:
			e := newError(CodeConnectionTimeout, fmt.Sprintf("connection timed out after %s", s.opts.ConnectTimeout))
			s.fail(cb, e)
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return -1, e, authenticated
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
func (s *Stream) handleMessage(ctx context.Context, cb Callbacks, typ websocket.MessageType, data []byte, authReported bool) (*Error, bool) {
	payload, barID := data, ""
	authenticated := false
	if typ == websocket.MessageText {
		var env struct {
			Type    string          `json:"type"`
			BarID   string          `json:"bar_id"`
			BarIDCC string          `json:"barId"`
			State   json.RawMessage `json:"state"`
			Device  RemoteDevice    `json:"device"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			s.emitError(cb, &Error{Code: CodeDecodeError, Message: "decode error: " + err.Error(), Data: data})
			return nil, false
		}
		if strings.HasPrefix(env.Type, "device.") {
			if cb.DeviceEvent != nil {
				cb.DeviceEvent(DeviceEvent{Type: env.Type, Device: env.Device})
			}
			return nil, false
		}
		barID = env.BarID
		if barID == "" {
			barID = env.BarIDCC
		}
		var err error
		if payload, err = decodeEnvelopeState(env.State); err != nil {
			s.emitError(cb, &Error{Code: CodeDecodeError, Message: "decode error: " + err.Error(), Data: data})
			return nil, false
		}
		if !authReported {
			s.patch(cb, func(st *Status) { st.Auth = Authenticated; st.Main = Running })
		}
		authenticated = true
	}
	if payload == nil || ctx.Err() != nil {
		return nil, authenticated
	}
	var raw pb.State
	if err := proto.Unmarshal(payload, &raw); err != nil {
		s.emitError(cb, &Error{Code: CodeDecodeError, Message: "decode error: " + err.Error(), Data: data})
		return nil, authenticated
	}
	if raw.Error != nil {
		e := &Error{
			Code:    CodeDeviceError,
			Message: fmt.Sprintf("device reported %s: %s", raw.Error.GetSeverity(), raw.Error.GetCause()),
			Data:    raw.Error,
		}
		fatal := raw.Error.GetSeverity() == pb.Severity_FATAL
		s.patch(cb, func(st *Status) {
			applyError(st, e)
			if fatal {
				st.Main = Failed
			}
		})
		s.emitError(cb, e)
		switch {
		case fatal:
			return e, authenticated
		case raw.Error.GetSeverity() == pb.Severity_ERROR:
			return nil, authenticated
		}
	}
	state := processState(&raw, barID, func(err error) {
		s.emitError(cb, &Error{Code: CodeFrameProcessError, Message: err.Error()})
	})
	if cb.Data != nil && ctx.Err() == nil {
		cb.Data(state)
	}
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

func (s *Stream) patch(cb Callbacks, fn func(*Status)) {
	s.mu.Lock()
	fn(&s.status)
	snapshot := s.status
	s.mu.Unlock()
	if cb.Status != nil {
		cb.Status(snapshot)
	}
}

func (s *Stream) emitError(cb Callbacks, e *Error) {
	if cb.Error != nil {
		cb.Error(e)
	}
}

func (s *Stream) fail(cb Callbacks, e *Error) {
	s.patch(cb, func(st *Status) { applyError(st, e) })
	s.emitError(cb, e)
}

func applyError(st *Status, e *Error) {
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
}

// touchData marks data active. The connection loop owns the stale timer.
func (s *Stream) touchData(cb Callbacks) {
	s.mu.Lock()
	changed := s.status.Data != DataActive
	s.status.Data, s.status.LastActivity = DataActive, time.Now()
	snapshot := s.status
	s.mu.Unlock()
	if changed && cb.Status != nil {
		cb.Status(snapshot)
	}
}
