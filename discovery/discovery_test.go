package discovery

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"reflect"
	"testing"
	"time"

	"github.com/enbility/zeroconf/v3"
)

func TestDeviceSetMergesAddresses(t *testing.T) {
	devices := make(deviceSet)
	devices.record(&zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{Instance: "printer"},
		Text:          []string{"name=Not a BUSY Bar"}, AddrIPv4: []net.IP{net.ParseIP("192.0.2.1")},
	})
	devices.record(&zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{Instance: "busybar-bb"},
		Text:          []string{"name="}, AddrIPv6: []net.IP{net.ParseIP("2001:db8::1")},
	})
	wifi := net.ParseIP("192.0.2.10")
	devices.record(&zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{Instance: "busybar-aa"},
		Text:          []string{"other=value", "name=Desk Bar"}, AddrIPv4: []net.IP{wifi},
	})
	devices.record(&zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{Instance: "busybar-aa"},
		// Like Python, retain the first name while merging later addresses.
		Text:     []string{"name=Later name"},
		AddrIPv4: []net.IP{net.ParseIP("10.0.4.20"), net.ParseIP("192.0.2.10"), net.IP{10, 0, 5, 2}},
	})
	// Returned addresses must not retain mutable DNS packet storage.
	wifi[len(wifi)-1] = 99
	want := []Device{
		{ID: "aa", Name: "Desk Bar", Addresses: []Address{
			{IP: netip.MustParseAddr("10.0.4.20"), Affinity: USB},
			{IP: netip.MustParseAddr("10.0.5.2"), Affinity: WiFi},
			{IP: netip.MustParseAddr("192.0.2.10"), Affinity: WiFi},
		}},
		{ID: "bb", Name: "BUSY Bar"},
	}
	if got := devices.list(); !reflect.DeepEqual(got, want) {
		t.Fatalf("devices = %+v, want %+v", got, want)
	}
}

func TestDeviceNames(t *testing.T) {
	for _, tc := range []struct {
		txt  []string
		want string
	}{
		{nil, "BUSY Bar"},
		{[]string{"name="}, "BUSY Bar"},
		{[]string{"name=Office=Upstairs"}, "Office=Upstairs"},
		{[]string{"name=Desk\xff"}, "Desk\uFFFD"},
	} {
		devices := make(deviceSet)
		devices.record(&zeroconf.ServiceEntry{ServiceRecord: zeroconf.ServiceRecord{Instance: "busybar-123"}, Text: tc.txt})
		if got := devices.list()[0].Name; got != tc.want {
			t.Fatalf("name = %q, want %q", got, tc.want)
		}
	}
}

func TestDeviceAddress(t *testing.T) {
	wifi := Address{IP: netip.MustParseAddr("192.0.2.10"), Affinity: WiFi}
	usb := Address{IP: netip.MustParseAddr("10.0.4.20"), Affinity: USB}
	for _, tc := range []struct {
		addresses []Address
		affinity  Affinity
		want      netip.Addr
		ok        bool
	}{
		{[]Address{wifi, usb}, Any, usb.IP, true},
		{[]Address{wifi, usb}, WiFi, wifi.IP, true},
		{[]Address{wifi}, Any, wifi.IP, true},
		{[]Address{wifi}, USB, netip.Addr{}, false},
		{nil, Any, netip.Addr{}, false},
	} {
		ip, ok := (Device{Addresses: tc.addresses}).Address(tc.affinity)
		if ip != tc.want || ok != tc.ok {
			t.Fatalf("Address(%q) = %v, %v; want %v, %v", tc.affinity, ip, ok, tc.want, tc.ok)
		}
	}
}

func TestDiscoverCanceledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	devices, err := Discover(ctx, Options{})
	if !errors.Is(err, context.Canceled) || len(devices) != 0 {
		t.Fatalf("got %v, %v", devices, err)
	}
}

func TestDiscoverInvalidTimeout(t *testing.T) {
	if _, err := Discover(context.Background(), Options{Timeout: -time.Second}); err == nil {
		t.Fatal("negative timeout accepted")
	}
}
