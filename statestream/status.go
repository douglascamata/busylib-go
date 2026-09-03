package statestream

import "time"

// Lifecycle is the high-level state of a Stream.
type Lifecycle string

const (
	Idle     Lifecycle = "IDLE"
	Starting Lifecycle = "STARTING"
	Running  Lifecycle = "RUNNING"
	Stopped  Lifecycle = "STOPPED"
	Failed   Lifecycle = "FAILED"
)

// ConnectionState is the WebSocket connection state.
type ConnectionState string

const (
	Disconnected ConnectionState = "DISCONNECTED"
	Connecting   ConnectionState = "CONNECTING"
	Connected    ConnectionState = "CONNECTED"
	Reconnecting ConnectionState = "RECONNECTING"
)

// AuthState is the authentication progress. Local streams jump straight to
// Authenticated when the socket opens.
type AuthState string

const (
	Unauthenticated  AuthState = "UNAUTHENTICATED"
	Authenticating   AuthState = "AUTHENTICATING"
	Authenticated    AuthState = "AUTHENTICATED"
	Reauthenticating AuthState = "REAUTHENTICATING"
	AuthFailed       AuthState = "FAILED"
)

// DataState tells whether messages are flowing. Stale means the socket is
// still open but nothing arrived for Options.DataTimeout.
type DataState string

const (
	DataNone   DataState = "NONE"
	DataActive DataState = "ACTIVE"
	DataStale  DataState = "STALE"
)

// Status is a snapshot of every state component. Callbacks.Status receives a
// new snapshot after each change.
type Status struct {
	Main      Lifecycle
	MainError *Error

	Connection         ConnectionState
	ConnectionAttempts int
	ConnectionError    *Error

	Auth         AuthState
	AuthAttempts int
	AuthError    *Error

	Data         DataState
	LastActivity time.Time
}

func initialStatus() Status {
	return Status{Main: Idle, Connection: Disconnected, Auth: Unauthenticated, Data: DataNone}
}
