package busybar

import (
	"context"
	"net/http"
	"net/url"
)

// TimeGet returns the device time.
func (c *Client) TimeGet(ctx context.Context) (*TimestampInfo, error) {
	var out TimestampInfo
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/time"}, &out)
}

// TimeTimestampSet sets the device time from an ISO 8601 timestamp with timezone.
func (c *Client) TimeTimestampSet(ctx context.Context, timestamp string) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/time/timestamp", query: url.Values{"timestamp": {timestamp}}}, nil)
}

// TimeTimezoneGet returns the configured timezone.
func (c *Client) TimeTimezoneGet(ctx context.Context) (*TimezoneInfo, error) {
	var out TimezoneInfo
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/time/timezone"}, &out)
}

// TimeTimezoneSet selects a timezone by name.
func (c *Client) TimeTimezoneSet(ctx context.Context, timezone string) error {
	return c.do(ctx, request{method: http.MethodPost, path: "/time/timezone", query: url.Values{"timezone": {timezone}}}, nil)
}

// TimeTzListGet lists the timezones the device knows.
func (c *Client) TimeTzListGet(ctx context.Context) (*TimezoneList, error) {
	var out TimezoneList
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/time/tzlist"}, &out)
}
