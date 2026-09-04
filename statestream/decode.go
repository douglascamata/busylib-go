package statestream

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"io"

	"github.com/douglascamata/busylib-go/frame"
	"github.com/douglascamata/busylib-go/statestream/pb"
)

// UpdateKind names the module a StateUpdate belongs to. Values are the
// snake_case oneof field names of BSB_State.StateUpdate.
type UpdateKind string

const (
	KindDeviceName      UpdateKind = "device_name"
	KindPower           UpdateKind = "power"
	KindBrightness      UpdateKind = "brightness"
	KindAudioVolume     UpdateKind = "audio_volume"
	KindWifi            UpdateKind = "wifi"
	KindUpdateState     UpdateKind = "update_state"
	KindUpdateCheck     UpdateKind = "update_check"
	KindTimezone        UpdateKind = "timezone"
	KindMatter          UpdateKind = "matter"
	KindFrame           UpdateKind = "frame"
	KindInput           UpdateKind = "input"
	KindTimer           UpdateKind = "timer"
	KindBle             UpdateKind = "ble"
	KindAutoUpdateState UpdateKind = "auto_update_state"
	KindTimerProfiles   UpdateKind = "timer_profiles"
)

// Frame is a display frame decompressed and converted to RGBA.
type Frame struct {
	Screen frame.Display
	Width  int
	Height int
	RGBA   []byte
}

// Image wraps the frame in an image.RGBA without copying.
func (f *Frame) Image() *image.RGBA { return frame.ToImage(f.RGBA, f.Width, f.Height) }

// Update is one state change. The embedded protobuf message exposes the
// payload through its getters, e.g. u.GetPower() or u.GetInput().
type Update struct {
	Kind UpdateKind
	*pb.StateUpdate
	// Frame is set when Kind is KindFrame and the frame decoded successfully.
	Frame *Frame
}

// State is one decoded message from the device.
type State struct {
	Timestamp uint64
	Updates   []Update
	// Error is set when the device reports a stream error.
	Error *pb.Error
	// BarID identifies the device in remote mode; empty for local streams.
	BarID string
	Raw   *pb.State
}

func processState(raw *pb.State, barID string, onFrameErr func(error)) *State {
	st := &State{Timestamp: raw.GetTimestamp(), Error: raw.GetError(), BarID: barID, Raw: raw}
	for _, u := range raw.GetUpdates() {
		up := Update{Kind: kindOf(u), StateUpdate: u}
		if f := u.GetFrame(); f != nil && len(f.GetData()) > 0 {
			rgba, err := DecodeFrame(f)
			switch {
			case err != nil:
				onFrameErr(err)
			case rgba != nil:
				up.Frame = &Frame{Screen: frame.Display(f.GetScreen()), Width: int(f.GetWidth()), Height: int(f.GetHeight()), RGBA: rgba}
			}
		}
		st.Updates = append(st.Updates, up)
	}
	return st
}

func kindOf(u *pb.StateUpdate) UpdateKind {
	m := u.ProtoReflect()
	fd := m.WhichOneof(m.Descriptor().Oneofs().ByName("state"))
	if fd == nil {
		return ""
	}
	return UpdateKind(fd.Name())
}

// DecodeFrame validates a complete display frame, decompresses it, and converts
// it to RGBA. Invalid geometry, formats, encodings, and payloads return an error.
func DecodeFrame(f *pb.Frame) ([]byte, error) {
	if f.GetScreen() != pb.Screen_FRONT && f.GetScreen() != pb.Screen_BACK {
		return nil, fmt.Errorf("statestream: unknown frame screen %d", f.GetScreen())
	}
	w, h := frame.Dimensions(frame.Display(f.GetScreen()))
	if f.GetWidth() != uint32(w) || f.GetHeight() != uint32(h) {
		return nil, fmt.Errorf("statestream: frame dimensions %dx%d, want %dx%d", f.GetWidth(), f.GetHeight(), w, h)
	}
	size, blockSize := w*h, 1
	switch f.GetPixelFormat() {
	case pb.PixelFormat_RGB888:
		size *= 3
		blockSize = 3
	case pb.PixelFormat_L4:
		size /= 2
		blockSize = 2
	case pb.PixelFormat_L8:
	default:
		return nil, fmt.Errorf("statestream: unknown frame pixel format %d", f.GetPixelFormat())
	}
	data := f.GetData()
	// The protocol's frame.options declares a 16 KiB data limit.
	if len(data) > 16384 {
		return nil, fmt.Errorf("statestream: frame payload exceeds 16384 bytes")
	}
	switch f.GetEncoding() {
	case pb.Encoding_PLAIN, pb.Encoding_RUN_LENGTH:
	case pb.Encoding_DEFLATE, pb.Encoding_DEFLATE_RUN_LENGTH:
		limit := size
		if f.GetEncoding() == pb.Encoding_DEFLATE_RUN_LENGTH {
			// Each nonempty RLE opcode represents at least one block.
			limit += size / blockSize
		}
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("statestream: frame decompression failed: %w", err)
		}
		data, err = io.ReadAll(io.LimitReader(r, int64(limit)+1))
		_ = r.Close()
		if err != nil {
			return nil, fmt.Errorf("statestream: frame decompression failed: %w", err)
		}
		if len(data) > limit {
			return nil, fmt.Errorf("statestream: inflated frame exceeds %d bytes", limit)
		}
	default:
		return nil, fmt.Errorf("statestream: unknown frame encoding %d", f.GetEncoding())
	}
	if f.GetEncoding() == pb.Encoding_RUN_LENGTH || f.GetEncoding() == pb.Encoding_DEFLATE_RUN_LENGTH {
		// Validate the complete RLE stream before the helper allocates its output.
		decodedSize := 0
		for i := 0; i < len(data); {
			opcode := data[i]
			i++
			count := int(opcode & 0x7f)
			if count == 0 {
				return nil, fmt.Errorf("statestream: zero-length RLE block")
			}
			encodedSize := blockSize
			if opcode&0x80 != 0 {
				encodedSize *= count
			}
			if encodedSize > len(data)-i {
				return nil, fmt.Errorf("statestream: truncated RLE block")
			}
			decodedSize += count * blockSize
			if decodedSize > size {
				return nil, fmt.Errorf("statestream: RLE frame exceeds %d bytes", size)
			}
			i += encodedSize
		}
		if decodedSize != size {
			return nil, fmt.Errorf("statestream: decoded frame has %d bytes, want %d", decodedSize, size)
		}
		data = frame.DecompressRLE(data, blockSize)
	} else if len(data) != size {
		return nil, fmt.Errorf("statestream: decoded frame has %d bytes, want %d", len(data), size)
	}
	switch f.GetPixelFormat() {
	case pb.PixelFormat_L4:
		return frame.L4ToRGBA(data, w, h), nil
	case pb.PixelFormat_L8:
		return frame.L8ToRGBA(data, w, h), nil
	default: // RGB888, checked above.
		return frame.BGRToRGBA(data, w, h), nil
	}
}
