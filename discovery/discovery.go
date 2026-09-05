// Package discovery finds BUSY Bar devices on the local network using mDNS.
// It follows busylib-py's _http._tcp.local. service and busybar- instance naming.
package discovery

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/enbility/zeroconf/v3"
)

// DefaultTimeout is the discovery window used by busylib-py.
const DefaultTimeout = 1500 * time.Millisecond

// Options configures a single discovery scan.
type Options struct {
	// Timeout is the scan duration. Zero means DefaultTimeout.
	Timeout time.Duration
	// Interfaces limits the scan to these network interfaces. Empty means all
	// active multicast interfaces, including USB-Ethernet.
	Interfaces []net.Interface
}

// Discover collects devices for the scan duration, then returns them sorted by
// ID, with unique addresses sorted by IP. No devices is a successful empty scan.
// Cancellation or a caller deadline returns partial results and ctx.Err().
// The normal scan timeout is not an error. All discovery sockets are closed
// before return. Devices seen during the scan are retained even if they leave.
func Discover(ctx context.Context, opts Options) ([]Device, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.Timeout < 0 {
		return nil, fmt.Errorf("discovery: timeout must not be negative")
	}
	if opts.Timeout == 0 {
		opts.Timeout = DefaultTimeout
	}
	scanCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	entries := make(chan *zeroconf.ServiceEntry)
	removed := make(chan *zeroconf.ServiceEntry)
	done := make(chan error, 1)
	go func() {
		done <- zeroconf.Browse(scanCtx, "_http._tcp", "local.", entries, removed,
			zeroconf.SelectIfaces(opts.Interfaces))
	}()

	devices := make(deviceSet)
	for {
		select {
		case entry, ok := <-entries:
			if !ok {
				entries = nil
				continue
			}
			devices.record(entry)
		case _, ok := <-removed:
			// Python returns everything seen within the scan window.
			if !ok {
				removed = nil
			}
		case err := <-done:
			result := devices.list()
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			if err != nil {
				return result, fmt.Errorf("discovery: browse mDNS: %w", err)
			}
			return result, nil
		}
	}
}

// Only Discover's receiving goroutine owns this map and its device slices.
type deviceSet map[string]*Device

func (devices deviceSet) record(entry *zeroconf.ServiceEntry) {
	instance, _, _ := strings.Cut(entry.Instance, ".")
	id, ok := strings.CutPrefix(instance, "busybar-")
	if !ok {
		return
	}
	device := devices[id]
	if device == nil {
		name := "BUSY Bar"
		for _, field := range entry.Text {
			if value, ok := strings.CutPrefix(field, "name="); ok && value != "" {
				name = strings.ToValidUTF8(value, "\uFFFD")
				break
			}
		}
		device = &Device{ID: id, Name: name}
		devices[id] = device
	}
	for _, raw := range entry.AddrIPv4 {
		// AddrIPv4 contains validated DNS A records. Unmap net.IP's 16-byte form.
		ip, _ := netip.AddrFromSlice(raw)
		ip = ip.Unmap()
		if slices.ContainsFunc(device.Addresses, func(a Address) bool { return a.IP == ip }) {
			continue
		}
		affinity := WiFi
		if strings.HasPrefix(ip.String(), "10.0.4.") {
			affinity = USB
		}
		device.Addresses = append(device.Addresses, Address{IP: ip, Affinity: affinity})
	}
}

func (devices deviceSet) list() []Device {
	result := make([]Device, 0, len(devices))
	for _, device := range devices {
		slices.SortFunc(device.Addresses, func(a, b Address) int { return a.IP.Compare(b.IP) })
		result = append(result, *device)
	}
	slices.SortFunc(result, func(a, b Device) int { return strings.Compare(a.ID, b.ID) })
	return result
}
