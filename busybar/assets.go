package busybar

import (
	"context"
	"net/http"
	"net/url"
)

// AssetsUpload stores a file in the application's asset directory.
func (c *Client) AssetsUpload(ctx context.Context, params AssetsUploadParams) error {
	return c.do(ctx, request{
		method:      http.MethodPost,
		path:        "/assets/upload",
		query:       url.Values{"application_name": {params.ApplicationName}, "file": {params.File}},
		body:        params.Data,
		contentType: "application/octet-stream",
	}, nil)
}

// AssetsDelete removes all assets of an application.
func (c *Client) AssetsDelete(ctx context.Context, applicationName string) error {
	return c.do(ctx, request{
		method: http.MethodDelete,
		path:   "/assets/upload",
		query:  url.Values{"application_name": {applicationName}},
	}, nil)
}
