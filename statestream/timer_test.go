package statestream

import (
	"bytes"
	"compress/gzip"
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/douglascamata/busylib-go/statestream/pb"
)

// The firmware timer serializer uses the same document as GET /busy/snapshot.
const timerJSON = `{"snapshot_timestamp_ms":1000000,"snapshot":{"type":"SIMPLE","card_id":"00000000-0000-0000-0000-000000000000","is_paused":false,"time_left_ms":60000,"busy_bar_settings":{"theme":"busy","show_work_phase_only":false,"trigger_smart_home":true}}}`

func TestDecodeTimer(t *testing.T) {
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	w.Write([]byte(timerJSON))
	w.Close()
	for _, tc := range []struct {
		name        string
		compression pb.Compression
		data        []byte
		wantErr     bool
	}{
		{"plain", pb.Compression_PLAIN, []byte(timerJSON), false},
		{"gzip", pb.Compression_GZIP, compressed.Bytes(), false},
		{"bad gzip", pb.Compression_GZIP, []byte(timerJSON), true},
		{"unknown compression", 99, []byte(timerJSON), true},
		{"bad JSON", pb.Compression_PLAIN, []byte("{"), true},
		{"null", pb.Compression_PLAIN, []byte("null"), true},
		{"missing JSON", pb.Compression_PLAIN, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := DecodeTimer(&pb.Timer{Json: &pb.Json{Compression: tc.compression, Data: tc.data}})
			if tc.wantErr {
				if err == nil || snapshot != nil {
					t.Fatal("invalid timer accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			state, err := snapshot.StateAt(time.UnixMilli(1020000))
			if err != nil || *state.TimeLeftMs != 40000 || !snapshot.Snapshot.BusyBarSettings.TriggerSmartHome {
				t.Fatalf("state %+v; error %v", state, err)
			}
		})
	}
}

func TestTimerStreamContinuesAfterMalformedUpdate(t *testing.T) {
	srv := newWSServer(t, func(conn *websocket.Conn, _ int32) {
		ctx := context.Background()
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
		for _, data := range []string{"{", timerJSON} {
			wire, err := proto.Marshal(&pb.State{Updates: []*pb.StateUpdate{
				{State: &pb.StateUpdate_Timer{Timer: &pb.Timer{Json: &pb.Json{Data: []byte(data)}}}},
				{State: &pb.StateUpdate_DeviceName{DeviceName: &pb.DeviceName{Name: "desk"}}},
			}})
			if err != nil {
				t.Error(err)
				return
			}
			conn.Write(ctx, websocket.MessageBinary, wire)
		}
		conn.Read(ctx)
	})
	s, err := NewLocal(Options{Addr: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	failures := make(chan *Error, 2)
	states := make(chan *State, 2)
	cancel, done := runAsync(s, Callbacks{Error: func(e *Error) { failures <- e }, Data: func(st *State) { states <- st }})
	defer cancelAndWait(t, cancel, done)
	if e := recv(t, failures, "timer error"); e.Code != CodeDecodeError || e.Unwrap() == nil {
		t.Fatalf("error %+v", e)
	}
	first := recv(t, states, "malformed timer")
	if first.Updates[0].Timer != nil || first.Updates[1].GetDeviceName().GetName() != "desk" {
		t.Fatal("bad timer affected another update")
	}
	next := recv(t, states, "valid timer")
	if next.Updates[0].Kind != KindTimer || next.Updates[0].Timer == nil {
		t.Fatal("typed timer missing")
	}
	state, err := next.Updates[0].Timer.StateAt(time.UnixMilli(1040000))
	if err != nil || *state.TimeLeftMs != 20000 {
		t.Fatalf("timer %+v; error %v", state, err)
	}
}
