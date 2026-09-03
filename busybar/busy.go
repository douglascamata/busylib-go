package busybar

import (
	"context"
	"net/http"
)

// BusySnapshotGet returns the running timer state.
func (c *Client) BusySnapshotGet(ctx context.Context) (*BusySnapshot, error) {
	var out BusySnapshot
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/busy/snapshot"}, &out)
}

// BusySnapshotSet replaces the running timer state.
func (c *Client) BusySnapshotSet(ctx context.Context, snapshot BusySnapshot) error {
	req, err := jsonRequest(http.MethodPut, "/busy/snapshot", snapshot)
	if err != nil {
		return err
	}
	return c.do(ctx, req, nil)
}

// BusyProfileGet returns the profile stored in a switch slot.
func (c *Client) BusyProfileGet(ctx context.Context, slot BusyProfileSlot) (*BusyProfile, error) {
	var out BusyProfile
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/busy/profiles/" + string(slot)}, &out)
}

// BusyProfileSet replaces the profile stored in a switch slot.
func (c *Client) BusyProfileSet(ctx context.Context, slot BusyProfileSlot, profile BusyProfile) error {
	req, err := jsonRequest(http.MethodPut, "/busy/profiles/"+string(slot), profile)
	if err != nil {
		return err
	}
	return c.do(ctx, req, nil)
}
