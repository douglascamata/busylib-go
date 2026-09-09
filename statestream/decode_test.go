package statestream

import (
	"bytes"
	"compress/zlib"
	"os"
	"testing"

	"github.com/douglascamata/busylib-go/busybar"
	"github.com/douglascamata/busylib-go/statestream/pb"
)

func deflate(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write(b)
	w.Close()
	return buf.Bytes()
}

func TestDecodeFrame(t *testing.T) {
	rgb := bytes.Repeat([]byte{1, 2, 3}, 72*16)
	rgbRLE := append(bytes.Repeat([]byte{127, 1, 2, 3}, 9), 9, 1, 2, 3)
	cases := []struct {
		name             string
		format           pb.PixelFormat
		plain, rle, want []byte
	}{
		{"rgb888", pb.PixelFormat_RGB888, rgb, rgbRLE, bytes.Repeat([]byte{3, 2, 1, 255}, 72*16)},
		{"l8", pb.PixelFormat_L8, bytes.Repeat([]byte{17}, 72*16), append(bytes.Repeat([]byte{127, 17}, 9), 9, 17), bytes.Repeat([]byte{17, 17, 17, 255}, 72*16)},
		{"l4", pb.PixelFormat_L4, bytes.Repeat([]byte{0x21, 0x43}, 72*16/4), append(bytes.Repeat([]byte{127, 0x21, 0x43}, 2), 34, 0x21, 0x43), bytes.Repeat([]byte{17, 17, 17, 255, 34, 34, 34, 255, 51, 51, 51, 255, 68, 68, 68, 255}, 72*16/4)},
	}
	for _, tc := range cases {
		for _, encoding := range []pb.Encoding{pb.Encoding_PLAIN, pb.Encoding_RUN_LENGTH, pb.Encoding_DEFLATE, pb.Encoding_DEFLATE_RUN_LENGTH} {
			t.Run(tc.name+"/"+encoding.String(), func(t *testing.T) {
				data := tc.plain
				if encoding == pb.Encoding_RUN_LENGTH || encoding == pb.Encoding_DEFLATE_RUN_LENGTH {
					data = tc.rle
				}
				if encoding == pb.Encoding_DEFLATE || encoding == pb.Encoding_DEFLATE_RUN_LENGTH {
					data = deflate(t, data)
				}
				got, err := DecodeFrame(&pb.Frame{Width: 72, Height: 16, PixelFormat: tc.format, Encoding: encoding, Data: data})
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, tc.want) {
					t.Fatalf("decoded image differs: got %d bytes, want %d", len(got), len(tc.want))
				}
			})
		}
	}
}

func TestDecodeFrameRejectsMalformedFrames(t *testing.T) {
	cases := []struct {
		name   string
		modify func(*pb.Frame)
	}{
		{"overflow dimensions", func(f *pb.Frame) { f.Width = ^uint32(0); f.Height = ^uint32(0) }},
		{"zero dimensions", func(f *pb.Frame) { f.Width = 0 }},
		{"partial frame", func(f *pb.Frame) { f.Width = 2; f.Height = 1 }},
		{"wrong display dimensions", func(f *pb.Frame) { f.Screen = pb.Screen_BACK }},
		{"unknown screen", func(f *pb.Frame) { f.Screen = 99 }},
		{"unknown format", func(f *pb.Frame) { f.PixelFormat = 99 }},
		{"unknown encoding", func(f *pb.Frame) { f.Encoding = 99 }},
		{"oversized payload", func(f *pb.Frame) { f.Data = make([]byte, 16385) }},
		{"short plain", func(f *pb.Frame) { f.Data = f.Data[:len(f.Data)-1] }},
		{"long plain", func(f *pb.Frame) { f.Data = append(f.Data, 0) }},
		{"bad deflate", func(f *pb.Frame) { f.Encoding = pb.Encoding_DEFLATE; f.Data = []byte{1, 2, 3} }},
		{"short deflate", func(f *pb.Frame) { f.Encoding = pb.Encoding_DEFLATE; f.Data = deflate(t, []byte{1, 2, 3}) }},
		{"deflate expansion", func(f *pb.Frame) { f.Encoding = pb.Encoding_DEFLATE; f.Data = deflate(t, make([]byte, 1000000)) }},
		{"deflate rle expansion", func(f *pb.Frame) {
			f.Encoding = pb.Encoding_DEFLATE_RUN_LENGTH
			f.Data = deflate(t, make([]byte, 1000000))
		}},
		{"rle expansion", func(f *pb.Frame) {
			f.Encoding = pb.Encoding_RUN_LENGTH
			f.Data = bytes.Repeat([]byte{127, 1, 2, 3}, 10)
		}},
		{"short rle", func(f *pb.Frame) { f.Encoding = pb.Encoding_RUN_LENGTH; f.Data = []byte{1, 1, 2, 3} }},
		{"truncated repeat", func(f *pb.Frame) { f.Encoding = pb.Encoding_RUN_LENGTH; f.Data = []byte{1, 1, 2} }},
		{"truncated literal", func(f *pb.Frame) { f.Encoding = pb.Encoding_RUN_LENGTH; f.Data = []byte{0x82, 1, 2, 3} }},
		{"zero count", func(f *pb.Frame) { f.Encoding = pb.Encoding_RUN_LENGTH; f.Data = []byte{0} }},
		{"trailing rle", func(f *pb.Frame) {
			f.Encoding = pb.Encoding_RUN_LENGTH
			f.Data = append(append(bytes.Repeat([]byte{127, 1, 2, 3}, 9), 9, 1, 2, 3), 1, 1, 2, 3)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &pb.Frame{Width: 72, Height: 16, Data: make([]byte, 72*16*3)}
			tc.modify(f)
			if got, err := DecodeFrame(f); err == nil || got != nil {
				t.Fatalf("got %d decoded bytes, error %v", len(got), err)
			}
		})
	}
}

func TestDecodeFrameDeflateRLEAllowsLiteralOverhead(t *testing.T) {
	// A valid RLE stream can be larger than the raw image: one opcode per pixel.
	data := bytes.Repeat([]byte{0x81, 1, 2, 3}, 72*16)
	got, err := DecodeFrame(&pb.Frame{Width: 72, Height: 16, Encoding: pb.Encoding_DEFLATE_RUN_LENGTH, Data: deflate(t, data)})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bytes.Repeat([]byte{3, 2, 1, 255}, 72*16)) {
		t.Fatal("decoded pixels differ")
	}
}

func TestDecodeFrameFirmwareBackL4(t *testing.T) {
	data, err := os.ReadFile("testdata/back-l4.rle")
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeFrame(&pb.Frame{Screen: pb.Screen_BACK, Width: 160, Height: 80, PixelFormat: pb.PixelFormat_L4, Encoding: pb.Encoding_RUN_LENGTH, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Repeat([]byte{17, 17, 17, 255, 34, 34, 34, 255, 51, 51, 51, 255, 68, 68, 68, 255}, 3200)
	if !bytes.Equal(got, want) {
		t.Fatal("decoded image differs from the firmware encoder's input")
	}
}

func TestProcessStateSetsKindAndFrame(t *testing.T) {
	raw := &pb.State{Timestamp: 42, Updates: []*pb.StateUpdate{
		{State: &pb.StateUpdate_Input{Input: &pb.InputEvent{Event: &pb.InputEvent_EncoderEvent{EncoderEvent: &pb.EncoderEvent{Delta: -1}}}}},
		{State: &pb.StateUpdate_Frame{Frame: &pb.Frame{Screen: pb.Screen_BACK, Width: 160, Height: 80, PixelFormat: pb.PixelFormat_L4, Data: bytes.Repeat([]byte{0x0F}, 6400)}}},
		{State: &pb.StateUpdate_Power{Power: &pb.Power{State: &pb.Power_Known{Known: &pb.PowerState{BatteryStatus: pb.BatteryStatus_CHARGING}}}}},
		{},
	}}
	st := processState(raw, "bar-1", func(err *Error) { t.Error(err) })
	if st.Timestamp != 42 || st.BarID != "bar-1" || len(st.Updates) != 4 {
		t.Fatalf("%+v", st)
	}
	if st.Updates[0].Kind != KindInput || st.Updates[0].GetInput().GetEncoderEvent().GetDelta() != -1 {
		t.Fatalf("input update %+v", st.Updates[0])
	}
	f := st.Updates[1].Frame
	if st.Updates[1].Kind != KindFrame || f == nil || f.Width != 160 || f.Screen != 1 || !bytes.Equal(f.RGBA, bytes.Repeat([]byte{255, 255, 255, 255, 0, 0, 0, 255}, 6400)) {
		t.Fatalf("frame update %+v", f)
	}
	if img := f.Image(); img.Bounds().Dx() != 160 {
		t.Fatalf("image %v", img.Bounds())
	}
	if st.Updates[2].Kind != KindPower || BatteryStatus(st.Updates[2].GetPower().GetKnown().GetBatteryStatus()) != busybar.PowerCharging {
		t.Fatalf("power update %+v", st.Updates[2])
	}
	if st.Updates[3].Kind != "" {
		t.Fatalf("empty update kind %q", st.Updates[3].Kind)
	}
}

func TestEnumConversions(t *testing.T) {
	if BleStatus(pb.ServiceStatus_ADVERTISING) != busybar.BleEnabled || BleStatus(pb.ServiceStatus_READY) != busybar.BleEnabled {
		t.Fatal("ble READY and ADVERTISING both map to enabled")
	}
	if WifiSecurity(pb.WifiSecurity_WPA_WPA2) != busybar.WifiWPAWPA2 || WifiSecurity(pb.WifiSecurity(99)) != "" {
		t.Fatal("wifi security mapping")
	}
	if UpdateAction(pb.UpdateAction_INSTALLATION_PREPARE) != busybar.UpdateActionPrepare {
		t.Fatal("update action mapping")
	}
	if CheckErrorReason(pb.CheckError_IDLE) != CheckErrorIdle || CheckEvent(pb.CheckEvent_STOP) != busybar.CheckEventStop {
		t.Fatal("check mapping")
	}
	if MatterStatus(pb.MatterCommissioningStatus_COMPLETED_SUCCESSFULLY) != busybar.PairingCompletedSuccessfully {
		t.Fatal("matter mapping")
	}
	if IPMethod(pb.IpConfigurationMethod_STATIC) != busybar.WifiStatic || IPProtocol(pb.IpProtocol_IPV6) != busybar.WifiIPv6 {
		t.Fatal("ip mapping")
	}
}
