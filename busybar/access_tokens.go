package busybar

import (
	"context"
	"net/http"
	"net/url"
)

// AccessToken is a local device credential, separate from a cloud bearer token.
// CreatedAt and LastUsedAt are Unix millisecond timestamps encoded as strings.
type AccessToken struct {
	ShortID    string `json:"short_id"`
	DisplayID  string `json:"display_id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
	// Token is disclosed only when created. Listings omit the secret.
	Token string `json:"token,omitempty"`
}

type AccessTokensInfo struct {
	Tokens []AccessToken `json:"tokens"`
}

// SettingsAccessTokensGet lists local credentials. Requires API 27.5.0 (firmware 1.2.3).
func (c *Client) SettingsAccessTokensGet(ctx context.Context) (*AccessTokensInfo, error) {
	var out AccessTokensInfo
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/access/tokens"}, &out)
}

// SettingsAccessTokenCreate creates a local credential. Save Token from the
// result; the device will not return it again. Requires API 27.5.0.
func (c *Client) SettingsAccessTokenCreate(ctx context.Context, name string) (*AccessToken, error) {
	req, err := jsonRequest(http.MethodPost, "/access/tokens", struct {
		Name string `json:"name"`
	}{name})
	if err != nil {
		return nil, err
	}
	var out AccessToken
	return &out, c.do(ctx, req, &out)
}

// SettingsAccessTokenRevoke revokes one local credential by ShortID. Requires API 27.5.0.
func (c *Client) SettingsAccessTokenRevoke(ctx context.Context, shortID string) error {
	return c.do(ctx, request{method: http.MethodDelete, path: "/access/tokens/" + url.PathEscape(shortID)}, nil)
}

// SettingsAccessTokensDeleteAll revokes every local credential. Requires API 27.5.0.
func (c *Client) SettingsAccessTokensDeleteAll(ctx context.Context) error {
	return c.do(ctx, request{method: http.MethodDelete, path: "/access/tokens"}, nil)
}
