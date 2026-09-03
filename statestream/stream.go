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

	ConnectTimeout       time.Duration
	DataTimeout          time.Duration
	MaxReconnectAttempts int
	ReconnectDelay       time.Duration
	MaxAuthAttempts      int
	// HTTPClient is used for the WebSocket handshake. nil means http.DefaultClient.
	HTTPClient *http.Client
}

// Callbacks receive stream events. Every field is optional. Callbacks run on
// the stream's goroutine, so they must not block for long.
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

	mu        sync.Mutex
	status    Status
	cb        Callbacks
	token     string
	conn      *websocket.Conn
	cancel    context.CancelFunc
	done      chan struct{}
	stopping  bool
	dataTimer *time.Timer
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
	s.cb = cb
	s.stopping = false
	runCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	done := make(chan struct{})
	s.done = done
	s.mu.Unlock()

	s.patch(func(st *Status) { st.Main = Starting; st.MainError = nil })

	ready := make(chan error, 1)
	go s.run(runCtx, ready, done)

	timer := time.NewTimer(s.opts.ConnectTimeout)
	defer timer.Stop()
	select {
	case err := <-ready:
		if err != nil {
			_ = s.shutdown(ctx)
		}
		return err
	case <-timer.C:
		err := newError(CodeConnectionTimeout, fmt.Sprintf("connection timed out after %s", s.opts.ConnectTimeout))
		s.mapErrorToStatus(err)
		s.emitError(err)
		_ = s.shutdown(ctx)
		return err
	case <-ctx.Done():
		_ = s.shutdown(context.Background())
		s.patch(func(st *Status) { st.Main = Stopped })
		return ctx.Err()
	}
}

// Stop closes the connection gracefully and clears the callbacks. It returns
// an Error with CodeConnectionLost when the WebSocket did not close cleanly.
func (s *Stream) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.status.Main == Idle || s.status.Main == Stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopping = true
	if s.dataTimer != nil {
		s.dataTimer.Stop()
	}
	s.mu.Unlock()
	s.patch(func(st *Status) {
		st.Main, st.Connection, st.Auth, st.Data = Stopped, Disconnected, Unauthenticated, DataNone
		st.ConnectionAttempts, st.AuthAttempts = 0, 0
	})
	err := s.shutdown(ctx)
	s.mu.Lock()
	s.cb = Callbacks{}
	s.mu.Unlock()
	return err
}

// shutdown closes the socket, ends the run goroutine and waits for it.
func (s *Stream) shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.stopping = true
	conn, cancel, done := s.conn, s.cancel, s.done
	if s.dataTimer != nil {
		s.dataTimer.Stop()
	}
	s.mu.Unlock()
	var closeErr error
	if conn != nil {
		closeErr = conn.Close(websocket.StatusNormalClosure, "")
	}
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
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
	conn := s.conn
	s.mu.Unlock()
	if !s.remote || conn == nil || !changed {
		return nil
	}
	s.patch(func(st *Status) { st.Auth = Authenticating })
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

func (s *Stream) run(ctx context.Context, ready chan<- error, done chan struct{}) {
	defer close(done)
	var once sync.Once
	signal := func(err error) { once.Do(func() { ready <- err }) }

	connectedOnce := false
	retries, authRetries := 0, 0
	for {
		s.patch(func(st *Status) { st.Connection = Connecting })
		conn, dialErr := s.dial(ctx)
		closeCode := websocket.StatusCode(-1)
		var readErr error
		if dialErr == nil {
			connectedOnce = true
			retries, authRetries = 0, 0
			s.onOpen(ctx, conn, signal)
			closeCode, readErr = s.readLoop(ctx, conn, signal)
			s.setConn(nil)
		} else {
			e := newError(CodeConnectionFailed, "websocket connection error: "+dialErr.Error())
			s.mapErrorToStatus(e)
			s.emitError(e)
			if !connectedOnce {
				signal(e)
				return
			}
		}
		if s.isStopping() || errors.Is(readErr, errFatalDeviceError) {
			return
		}

		switch {
		case s.remote && closeCode == authCloseCode:
			authRetries++
			if authRetries > s.opts.MaxAuthAttempts {
				e := newError(CodeAuthFailed, fmt.Sprintf("maximum authentication attempts (%d) reached", s.opts.MaxAuthAttempts))
				s.mapErrorToStatus(e)
				s.emitError(e)
				signal(e)
				return
			}
			s.patch(func(st *Status) { st.Auth = Reauthenticating; st.AuthAttempts = authRetries })
			if s.opts.TokenProvider == nil {
				e := newError(CodeAuthFailed, "token rejected and no TokenProvider configured")
				s.mapErrorToStatus(e)
				s.emitError(e)
				signal(e)
				return
			}
			token, err := s.opts.TokenProvider(ctx)
			if err != nil {
				e := newError(CodeAuthRefreshFailed, "failed to refresh token: "+err.Error())
				s.mapErrorToStatus(e)
				s.emitError(e)
				signal(e)
				return
			}
			s.mu.Lock()
			s.token = token
			s.mu.Unlock()
		case closeCode == websocket.StatusNormalClosure:
			s.patch(func(st *Status) { st.Connection = Disconnected })
			e := newError(CodeConnectionLost, "stream closed by the server")
			s.mapErrorToStatus(e)
			s.emitError(e)
			signal(e)
			return
		default:
			retries++
			if retries > s.opts.MaxReconnectAttempts {
				e := newError(CodeReconnectFailed, fmt.Sprintf("maximum reconnection attempts (%d) reached", s.opts.MaxReconnectAttempts))
				s.mapErrorToStatus(e)
				s.emitError(e)
				signal(e)
				return
			}
			s.patch(func(st *Status) { st.Connection = Reconnecting; st.ConnectionAttempts = retries })
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

func (s *Stream) onOpen(ctx context.Context, conn *websocket.Conn, signal func(error)) {
	s.setConn(conn)
	s.patch(func(st *Status) { st.Connection = Connected; st.ConnectionAttempts = 0 })
	if !s.remote {
		_ = writeJSON(ctx, conn, map[string]bool{"enable": true})
		s.patch(func(st *Status) { st.Auth = Authenticated; st.Main = Running })
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
		s.patch(func(st *Status) { st.Auth = Authenticating })
		_ = writeJSON(ctx, conn, map[string]string{"token": token})
	}
	if len(subs) > 0 {
		_ = writeJSON(ctx, conn, map[string][]string{"subscribe": subs})
	}
}

// readLoop delivers messages until the connection ends. It returns the close
// code (or -1 when the connection did not end with a close frame).
func (s *Stream) readLoop(ctx context.Context, conn *websocket.Conn, signal func(error)) (websocket.StatusCode, error) {
	authReported := false
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return websocket.CloseStatus(err), err
		}
		s.touchData()
		if cb := s.callbacks(); cb.RawData != nil {
			cb.RawData(data)
		}
		if s.handleMessage(typ, data, &authReported, signal) {
			_ = conn.Close(websocket.StatusInternalError, "fatal device error")
			return -1, errFatalDeviceError
		}
	}
}

// handleMessage decodes one message and reports true for a fatal device error.
func (s *Stream) handleMessage(typ websocket.MessageType, data []byte, authReported *bool, signal func(error)) bool {
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
			s.emitError(&Error{Code: CodeDecodeError, Message: "decode error: " + err.Error(), Data: data})
			return false
		}
		if strings.HasPrefix(env.Type, "device.") {
			if cb := s.callbacks(); cb.DeviceEvent != nil {
				cb.DeviceEvent(DeviceEvent{Type: env.Type, Device: env.Device})
			}
			return false
		}
		barID = env.BarID
		if barID == "" {
			barID = env.BarIDCC
		}
		var err error
		if payload, err = decodeEnvelopeState(env.State); err != nil {
			s.emitError(&Error{Code: CodeDecodeError, Message: "decode error: " + err.Error(), Data: data})
			return false
		}
		if s.remote && !*authReported {
			*authReported = true
			s.patch(func(st *Status) { st.Auth = Authenticated; st.Main = Running })
			signal(nil)
		}
	}
	if payload == nil {
		return false
	}
	var raw pb.State
	if err := proto.Unmarshal(payload, &raw); err != nil {
		s.emitError(&Error{Code: CodeDecodeError, Message: "decode error: " + err.Error(), Data: data})
		return false
	}
	if raw.Error != nil {
		e := &Error{
			Code:    CodeDeviceError,
			Message: fmt.Sprintf("device reported %s: %s", raw.Error.GetSeverity(), raw.Error.GetCause()),
			Data:    raw.Error,
		}
		s.mapErrorToStatus(e)
		s.emitError(e)
		switch raw.Error.GetSeverity() {
		case pb.Severity_FATAL:
			s.patch(func(st *Status) { st.Main = Failed })
			signal(e)
			return true
		case pb.Severity_ERROR:
			return false
		}
	}
	state := processState(&raw, barID, func(err error) {
		s.emitError(&Error{Code: CodeFrameProcessError, Message: err.Error()})
	})
	if cb := s.callbacks(); cb.Data != nil {
		cb.Data(state)
	}
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

func (s *Stream) setConn(conn *websocket.Conn) {
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()
}

func (s *Stream) isStopping() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopping
}

func (s *Stream) callbacks() Callbacks {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cb
}

// patch mutates the status under the lock and reports the new snapshot.
func (s *Stream) patch(fn func(*Status)) {
	s.mu.Lock()
	fn(&s.status)
	snap, cb := s.status, s.cb
	s.mu.Unlock()
	if cb.Status != nil {
		cb.Status(snap)
	}
}

func (s *Stream) emitError(e *Error) {
	if cb := s.callbacks(); cb.Error != nil {
		cb.Error(e)
	}
}

func (s *Stream) mapErrorToStatus(e *Error) {
	s.patch(func(st *Status) {
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
func (s *Stream) touchData() {
	s.mu.Lock()
	changed := s.status.Data != DataActive
	s.status.Data, s.status.LastActivity = DataActive, time.Now()
	if s.dataTimer != nil {
		s.dataTimer.Stop()
	}
	s.dataTimer = time.AfterFunc(s.opts.DataTimeout, func() {
		s.patch(func(st *Status) {
			if st.Data == DataActive {
				st.Data = DataStale
			}
		})
	})
	snap, cb := s.status, s.cb
	s.mu.Unlock()
	if changed && cb.Status != nil {
		cb.Status(snap)
	}
}
