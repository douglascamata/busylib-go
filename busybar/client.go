// Package busybar is a typed client for the BUSY Bar HTTP API.
//
// Every method takes a context. The client applies Config.Timeout to a call
// only when the context has no deadline of its own, so a caller can override
// the default per call with context.WithTimeout.
package busybar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	DefaultDeviceURL = "http://10.0.4.20"
	DefaultProxyURL  = "https://api.busy.app"
	DefaultTimeout   = 3 * time.Second
)

var proxyHostRe = regexp.MustCompile(`(?i)^https?://api(?:\.(?:dev|test|stage))?\.busy\.app$`)

// Config describes how to reach a BUSY Bar.
type Config struct {
	// Addr is an IP address, a host name, or a full URL. Without a scheme,
	// http:// is assumed, except for the BUSY proxy host which defaults to
	// https://. Empty means the USB-Ethernet device address, or the BUSY
	// proxy when Token is set.
	Addr string
	// Token is the bearer token for the BUSY proxy (https://api.busy.app).
	Token string
	// HTTPAccessPassword is the optional HTTP access password of the device.
	HTTPAccessPassword string
	// Timeout is the default per-request timeout. Zero means DefaultTimeout;
	// a negative value disables the default.
	Timeout time.Duration
	// HTTPClient is used for all requests. nil means http.DefaultClient.
	HTTPClient *http.Client
}

// Client talks to one BUSY Bar, directly or through the BUSY proxy.
type Client struct {
	addr    string
	baseURL string
	http    *http.Client
	timeout time.Duration

	mu        sync.RWMutex
	token     string
	apiKey    string
	apiSemver string
	versionMu sync.Mutex
}

// New builds a client. It returns an error when Addr is not a valid address,
// or when Addr points at the BUSY proxy and Token is empty.
func New(cfg Config) (*Client, error) {
	addr, err := resolveAddr(cfg)
	if err != nil {
		return nil, err
	}
	prefix := "/api"
	if proxyHostRe.MatchString(addr) {
		prefix = "/busybar"
	}
	c := &Client{
		addr:    addr,
		baseURL: addr + prefix,
		http:    cfg.HTTPClient,
		timeout: cfg.Timeout,
		token:   cfg.Token,
		apiKey:  cfg.HTTPAccessPassword,
	}
	if c.http == nil {
		c.http = http.DefaultClient
	}
	if c.timeout == 0 {
		c.timeout = DefaultTimeout
	}
	return c, nil
}

func resolveAddr(cfg Config) (string, error) {
	if cfg.Addr == "" {
		if cfg.Token == "" {
			return DefaultDeviceURL, nil
		}
		return DefaultProxyURL, nil
	}
	addr := strings.TrimSpace(cfg.Addr)
	explicitScheme := strings.HasPrefix(strings.ToLower(addr), "http://") || strings.HasPrefix(strings.ToLower(addr), "https://")
	if !explicitScheme {
		addr = "http://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("busybar: invalid address %q", cfg.Addr)
	}
	origin := strings.ToLower(u.Scheme) + "://" + u.Host
	if !explicitScheme && proxyHostRe.MatchString(origin) {
		origin = "https://" + u.Host
	}
	if proxyHostRe.MatchString(origin) && cfg.Token == "" {
		return "", errors.New("busybar: token is required for the BUSY proxy")
	}
	return origin, nil
}

// Addr returns the normalized origin the client talks to, e.g. http://10.0.4.20.
func (c *Client) Addr() string { return c.addr }

// APISemver returns the API version reported by the device, or "" before the
// first successful request.
func (c *Client) APISemver() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.apiSemver
}

// SetToken replaces the bearer token used for all following requests.
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

// SetHTTPAccessPassword replaces the HTTP access password (X-API-Token header).
func (c *Client) SetHTTPAccessPassword(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apiKey = key
}

// HTTPError is returned for any non-2xx response.
type HTTPError struct {
	StatusCode int
	Status     string
	Body       []byte
	// Message is the "error" or "message" field of a JSON body, the plain text
	// body, or the HTTP status line.
	Message string
}

func (e *HTTPError) Error() string { return e.Message }

func newHTTPError(res *http.Response) *HTTPError {
	body, _ := io.ReadAll(res.Body)
	e := &HTTPError{StatusCode: res.StatusCode, Status: res.Status, Body: body}
	if strings.Contains(res.Header.Get("Content-Type"), "application/json") {
		var parsed struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &parsed) == nil {
			e.Message = parsed.Error
			if e.Message == "" {
				e.Message = parsed.Message
			}
		}
	} else {
		e.Message = strings.TrimSpace(string(body))
	}
	if e.Message == "" {
		e.Message = "HTTP " + res.Status
	}
	return e
}

type request struct {
	method      string
	path        string
	query       url.Values
	body        []byte
	contentType string
}

func jsonRequest(method, path string, v any) (request, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return request{}, err
	}
	return request{method: method, path: path, body: body, contentType: "application/json"}, nil
}

// do sends a request and decodes the response into out. out may be nil, a
// *[]byte for the raw body, or a pointer to a JSON-decodable value.
func (c *Client) do(ctx context.Context, req request, out any) error {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	isVersion := req.path == "/version"
	if !isVersion {
		if err := c.ensureVersion(ctx); err != nil {
			return err
		}
	}
	res, err := c.send(ctx, req)
	if err != nil {
		return err
	}
	if res.StatusCode == http.StatusMethodNotAllowed && !isVersion {
		// The device rejects a stale X-API-Sem-Ver with 405. Refresh and retry once.
		_ = res.Body.Close()
		c.mu.Lock()
		c.apiSemver = ""
		c.mu.Unlock()
		if err := c.ensureVersion(ctx); err != nil {
			return err
		}
		if res, err = c.send(ctx, req); err != nil {
			return err
		}
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return newHTTPError(res)
	}
	switch v := out.(type) {
	case nil:
		return nil
	case *[]byte:
		*v, err = io.ReadAll(res.Body)
		return err
	default:
		return json.NewDecoder(res.Body).Decode(out)
	}
}

func (c *Client) send(ctx context.Context, req request) (*http.Response, error) {
	u := c.baseURL + req.path
	if len(req.query) > 0 {
		u += "?" + req.query.Encode()
	}
	var body io.Reader
	if req.body != nil {
		body = bytes.NewReader(req.body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.method, u, body)
	if err != nil {
		return nil, err
	}
	if req.contentType != "" {
		httpReq.Header.Set("Content-Type", req.contentType)
	}
	c.mu.RLock()
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	if req.path != "/version" {
		if c.apiSemver != "" {
			httpReq.Header.Set("X-API-Sem-Ver", c.apiSemver)
		}
		if c.apiKey != "" {
			httpReq.Header.Set("X-API-Token", c.apiKey)
		}
	}
	c.mu.RUnlock()
	return c.http.Do(httpReq)
}

// ensureVersion fetches the API version once and caches it. Concurrent callers
// wait for the same fetch instead of each hitting /version.
func (c *Client) ensureVersion(ctx context.Context) error {
	c.versionMu.Lock()
	defer c.versionMu.Unlock()
	if c.APISemver() != "" {
		return nil
	}
	v, err := c.SystemVersionGet(ctx)
	if err != nil {
		return err
	}
	if v.APISemver == "" {
		return errors.New("busybar: device returned an empty API version")
	}
	c.mu.Lock()
	c.apiSemver = v.APISemver
	c.mu.Unlock()
	return nil
}
