package statestream

// ErrorCode is a machine-readable reason attached to every Error.
type ErrorCode string

const (
	CodeConnectionFailed     ErrorCode = "CONNECTION_FAILED"
	CodeReconnectFailed      ErrorCode = "RECONNECT_FAILED"
	CodeConnectionLost       ErrorCode = "CONNECTION_LOST"
	CodeConnectionTimeout    ErrorCode = "CONNECTION_TIMEOUT"
	CodeAuthFailed           ErrorCode = "AUTH_FAILED"
	CodeAuthRefreshFailed    ErrorCode = "AUTH_REFRESH_FAILED"
	CodeDeviceError          ErrorCode = "DEVICE_ERROR"
	CodeDecodeError          ErrorCode = "DECODE_ERROR"
	CodeFrameProcessError    ErrorCode = "FRAME_PROCESS_ERROR"
	CodeStreamAlreadyStarted ErrorCode = "STREAM_ALREADY_STARTED"
)

// Error is reported through Callbacks.Error and returned by Run.
type Error struct {
	Code    ErrorCode
	Message string
	// Cause is the underlying transport, token-provider, or decoding error.
	Cause error
	// Data carries context for some codes: the protobuf error for
	// CodeDeviceError, the raw message for CodeDecodeError.
	Data any
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

func (e *Error) Unwrap() error { return e.Cause }

func newError(code ErrorCode, msg string) *Error { return &Error{Code: code, Message: msg} }
