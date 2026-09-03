package statestream

import (
	"bytes"
	"compress/zlib"
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
	// 4x1 BGR: one literal pixel (B1 G2 R3) then (9,9,9) repeated 3 times.
	rleBGR := []byte{0x81, 1, 2, 3, 0x03, 9, 9, 9}
	wantBGR := []byte{3, 2, 1, 255, 9, 9, 9, 255, 9, 9, 9, 255, 9, 9, 9, 255}
	cases := []struct {
		name  string
		frame *pb.Frame
		want  []byte
	}{
		{"rle rgb888", &pb.Frame{Width: 4, Height: 1, Encoding: pb.Encoding_RUN_LENGTH, PixelFormat: pb.PixelFormat_RGB888, Data: rleBGR}, wantBGR},
		{"plain l8", &pb.Frame{Width: 2, Height: 1, PixelFormat: pb.PixelFormat_L8, Data: []byte{0, 255}}, []byte{0, 0, 0, 255, 255, 255, 255, 255}},
		{"deflate l4", &pb.Frame{Width: 2, Height: 1, Encoding: pb.Encoding_DEFLATE, PixelFormat: pb.PixelFormat_L4, Data: deflate(t, []byte{0xF0})}, []byte{0, 0, 0, 255, 255, 255, 255, 255}},
		{"deflate+rle rgb888", &pb.Frame{Width: 4, Height: 1, Encoding: pb.Encoding_DEFLATE_RUN_LENGTH, PixelFormat: pb.PixelFormat_RGB888, Data: deflate(t, rleBGR)}, wantBGR},
		{"no dimensions", &pb.Frame{Data: []byte{1}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeFrame(tc.frame)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestDecodeFrameReportsBadDeflate(t *testing.T) {
	_, err := DecodeFrame(&pb.Frame{Width: 1, Height: 1, Encoding: pb.Encoding_DEFLATE, Data: []byte{1, 2, 3}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestProcessStateSetsKindAndFrame(t *testing.T) {
	raw := &pb.State{Timestamp: 42, Updates: []*pb.StateUpdate{
		{State: &pb.StateUpdate_Input{Input: &pb.InputEvent{Event: &pb.InputEvent_EncoderEvent{EncoderEvent: &pb.EncoderEvent{Delta: -1}}}}},
		{State: &pb.StateUpdate_Frame{Frame: &pb.Frame{Screen: pb.Screen_BACK, Width: 2, Height: 1, PixelFormat: pb.PixelFormat_L4, Data: []byte{0x0F}}}},
		{State: &pb.StateUpdate_Power{Power: &pb.Power{State: &pb.Power_Known{Known: &pb.PowerState{BatteryStatus: pb.BatteryStatus_CHARGING}}}}},
		{},
	}}
	st := processState(raw, "bar-1", func(err error) { t.Error(err) })
	if st.Timestamp != 42 || st.BarID != "bar-1" || len(st.Updates) != 4 {
		t.Fatalf("%+v", st)
	}
	if st.Updates[0].Kind != KindInput || st.Updates[0].GetInput().GetEncoderEvent().GetDelta() != -1 {
		t.Fatalf("input update %+v", st.Updates[0])
	}
	f := st.Updates[1].Frame
	if st.Updates[1].Kind != KindFrame || f == nil || f.Width != 2 || f.Screen != 1 || !bytes.Equal(f.RGBA, []byte{255, 255, 255, 255, 0, 0, 0, 255}) {
		t.Fatalf("frame update %+v", f)
	}
	if img := f.Image(); img.Bounds().Dx() != 2 {
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
