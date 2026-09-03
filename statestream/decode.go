package statestream

import (
	"fmt"
	"image"

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

// DecodeFrame decompresses a protobuf frame and converts it to RGBA. It
// returns nil, nil for a frame without dimensions.
func DecodeFrame(f *pb.Frame) ([]byte, error) {
	w, h := int(f.GetWidth()), int(f.GetHeight())
	if w == 0 || h == 0 {
		return nil, nil
	}
	data := f.GetData()
	blockSize := 1
	if f.GetPixelFormat() == pb.PixelFormat_RGB888 {
		blockSize = 3
	}
	var err error
	switch f.GetEncoding() {
	case pb.Encoding_RUN_LENGTH:
		data = frame.DecompressRLE(data, blockSize)
	case pb.Encoding_DEFLATE:
		data, err = frame.Inflate(data)
	case pb.Encoding_DEFLATE_RUN_LENGTH:
		if data, err = frame.Inflate(data); err == nil {
			data = frame.DecompressRLE(data, blockSize)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("statestream: frame decompression failed: %w", err)
	}
	switch f.GetPixelFormat() {
	case pb.PixelFormat_L4:
		return frame.L4ToRGBA(data, w, h), nil
	case pb.PixelFormat_L8:
		return frame.L8ToRGBA(data, w, h), nil
	case pb.PixelFormat_RGB888:
		return frame.BGRToRGBA(data, w, h), nil
	default:
		return make([]byte, w*h*4), nil
	}
}
