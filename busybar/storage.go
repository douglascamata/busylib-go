package busybar

import (
	"context"
	"net/http"
	"net/url"
)

type StorageEntryType string

const (
	StorageFile StorageEntryType = "file"
	StorageDir  StorageEntryType = "dir"
)

type StorageEntry struct {
	Type StorageEntryType `json:"type"`
	Name string           `json:"name"`
	// Size in bytes. Only set for files.
	Size int64 `json:"size,omitempty"`
}

type StorageList struct {
	List []StorageEntry `json:"list"`
}

type StorageStatus struct {
	UsedBytes  int64 `json:"used_bytes"`
	FreeBytes  int64 `json:"free_bytes"`
	TotalBytes int64 `json:"total_bytes"`
}

// StorageWrite creates or overwrites a file.
func (c *Client) StorageWrite(ctx context.Context, path string, data []byte) error {
	return c.do(ctx, request{
		method:      http.MethodPost,
		path:        "/storage/write",
		query:       url.Values{"path": {path}},
		body:        data,
		contentType: "application/octet-stream",
	}, nil)
}

// StorageAppend appends bytes to a file, creating it if needed.
// Data is sent unchanged. Requires API 27.5.0 or newer.
func (c *Client) StorageAppend(ctx context.Context, path string, data []byte) error {
	return c.do(ctx, request{
		method:      http.MethodPost,
		path:        "/storage/write",
		query:       url.Values{"path": {path}, "append": {"1"}},
		body:        data,
		contentType: "application/octet-stream",
	}, nil)
}

// StorageRead returns the content of a file.
func (c *Client) StorageRead(ctx context.Context, path string) ([]byte, error) {
	var out []byte
	err := c.do(ctx, request{method: http.MethodGet, path: "/storage/read", query: url.Values{"path": {path}}}, &out)
	return out, err
}

// StorageListGet lists a directory.
func (c *Client) StorageListGet(ctx context.Context, path string) (*StorageList, error) {
	var out StorageList
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/storage/list", query: url.Values{"path": {path}}}, &out)
}

// StorageRemove deletes a file or directory.
func (c *Client) StorageRemove(ctx context.Context, path string) error {
	return c.do(ctx, request{method: http.MethodDelete, path: "/storage/remove", query: url.Values{"path": {path}}}, nil)
}

// StorageMkdir creates a directory.
func (c *Client) StorageMkdir(ctx context.Context, path string) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/storage/mkdir", query: url.Values{"path": {path}}}, nil)
}

// StorageRename moves a file or directory.
func (c *Client) StorageRename(ctx context.Context, path, newPath string) error {
	return c.do(ctx, request{
		method: http.MethodPost,
		path:   "/storage/rename",
		query:  url.Values{"path": {path}, "new_path": {newPath}},
	}, nil)
}

// StorageStatusGet returns space usage.
func (c *Client) StorageStatusGet(ctx context.Context) (*StorageStatus, error) {
	var out StorageStatus
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/storage/status"}, &out)
}
