package busybar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestAccessTokenLifecycle(t *testing.T) {
	d := newFakeDevice(t)
	d.semver = "27.5.0"
	// Model the published API: only creation returns the full secret.
	tokens := map[string]AccessToken{}
	d.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("X-API-Token") != "local-secret" || r.Header.Get("Authorization") != "" {
			t.Error("wrong credential header")
		}
		if r.URL.Path == "/api/access/tokens" {
			switch r.Method {
			case http.MethodPost:
				var body struct {
					Name string `json:"name"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				token := AccessToken{ShortID: body.Name, DisplayID: body.Name + "…secret", Name: body.Name, CreatedAt: "1785812863582", LastUsedAt: "0", Token: body.Name + "full-secret"}
				writeJSON(w, 200, token)
				token.Token = ""
				tokens[token.ShortID] = token
			case http.MethodGet:
				list := []AccessToken{}
				for _, token := range tokens {
					list = append(list, token)
				}
				writeJSON(w, 200, AccessTokensInfo{Tokens: list})
			case http.MethodDelete:
				clear(tokens)
				writeJSON(w, 200, map[string]string{"result": "OK"})
			default:
				t.Errorf("method %s", r.Method)
			}
			return true
		}
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/access/tokens/") {
			id := strings.TrimPrefix(r.URL.Path, "/api/access/tokens/")
			if _, ok := tokens[id]; !ok {
				writeJSON(w, 404, map[string]string{"error": "token missing"})
				return true
			}
			delete(tokens, id)
			writeJSON(w, 200, map[string]string{"result": "OK"})
			return true
		}
		return false
	}
	c := d.client(t, Config{HTTPAccessPassword: "local-secret"})
	ctx := context.Background()
	first, err := c.SettingsAccessTokenCreate(ctx, "script01")
	if err != nil || first.Token != "script01full-secret" || first.CreatedAt != "1785812863582" {
		t.Fatalf("create: %+v %v", first, err)
	}
	second, err := c.SettingsAccessTokenCreate(ctx, "script02")
	if err != nil {
		t.Fatal(err)
	}
	list, err := c.SettingsAccessTokensGet(ctx)
	if err != nil || len(list.Tokens) != 2 {
		t.Fatalf("list: %+v %v", list, err)
	}
	for _, token := range list.Tokens {
		if token.Token != "" {
			t.Fatal("list exposed a secret")
		}
	}
	if err := c.SettingsAccessTokenRevoke(ctx, first.ShortID); err != nil {
		t.Fatal(err)
	}
	list, err = c.SettingsAccessTokensGet(ctx)
	if err != nil || len(list.Tokens) != 1 || list.Tokens[0].ShortID != second.ShortID {
		t.Fatalf("revoke: %+v %v", list, err)
	}
	err = c.SettingsAccessTokenRevoke(ctx, first.ShortID)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 {
		t.Fatalf("missing token: %v", err)
	}
	if err := c.SettingsAccessTokensDeleteAll(ctx); err != nil {
		t.Fatal(err)
	}
	list, err = c.SettingsAccessTokensGet(ctx)
	if err != nil || len(list.Tokens) != 0 {
		t.Fatalf("delete all: %+v %v", list, err)
	}
}
