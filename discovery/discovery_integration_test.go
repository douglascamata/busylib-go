package discovery_test

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/douglascamata/busylib-go/busybar"
	"github.com/douglascamata/busylib-go/discovery"
	"github.com/enbility/zeroconf/v3"
)

// This test sends real multicast packets.
func TestDiscoverLocalAdvertisement(t *testing.T) {
	id := fmt.Sprintf("busylib-go-test-%d", time.Now().UnixNano())
	server, err := zeroconf.RegisterProxy("busybar-"+id, "_http._tcp", "local.", 80,
		id+".local.", []string{"192.0.2.10", "10.0.4.20", "2001:db8::1"},
		[]string{"name=Discovery Test Bar"}, nil, zeroconf.TTL(5))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown()

	devices, err := discovery.Discover(context.Background(), discovery.Options{Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range devices {
		if device.ID != id {
			continue
		}
		if device.Name != "Discovery Test Bar" || len(device.Addresses) != 2 {
			t.Fatalf("device = %+v", device)
		}
		usb, ok := device.Address(discovery.Any)
		if !ok || usb != netip.MustParseAddr("10.0.4.20") {
			t.Fatalf("preferred address = %v, %v", usb, ok)
		}
		wifi, ok := device.Address(discovery.WiFi)
		if !ok || wifi != netip.MustParseAddr("192.0.2.10") {
			t.Fatalf("Wi-Fi address = %v, %v", wifi, ok)
		}
		bar, err := busybar.New(busybar.Config{Addr: wifi.String()})
		if err != nil {
			t.Fatal(err)
		}
		if bar.Addr() != "http://192.0.2.10" {
			t.Fatalf("client address = %q", bar.Addr())
		}
		return
	}
	t.Fatalf("local advertisement %q was not discovered; got %+v", id, devices)
}

func TestDiscoverCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := discovery.Discover(ctx, discovery.Options{Timeout: time.Minute})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("cancellation took %v", elapsed)
	}
}
