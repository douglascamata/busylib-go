package statestream

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"

	"github.com/douglascamata/busylib-go/busybar"
	"github.com/douglascamata/busylib-go/statestream/pb"
)

// DecodeTimer decodes the JSON snapshot carried by a protobuf timer update.
// Protobuf has already decoded the bytes; no base64 conversion is needed.
func DecodeTimer(timer *pb.Timer) (*busybar.BusySnapshot, error) {
	envelope := timer.GetJson()
	data := envelope.GetData()
	switch envelope.GetCompression() {
	case pb.Compression_PLAIN:
	case pb.Compression_GZIP:
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("statestream: timer gzip: %w", err)
		}
		data, err = io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			return nil, fmt.Errorf("statestream: timer gzip: %w", err)
		}
	default:
		return nil, fmt.Errorf("statestream: unknown timer compression %d", envelope.GetCompression())
	}
	var snapshot *busybar.BusySnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("statestream: timer JSON: %w", err)
	}
	if snapshot == nil {
		return nil, fmt.Errorf("statestream: timer JSON is null")
	}
	return snapshot, nil
}
