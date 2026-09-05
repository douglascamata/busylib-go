package discovery

import "net/netip"

// Affinity identifies how an advertised address reaches the device.
type Affinity string

const (
	// Any prefers USB, then Wi-Fi.
	Any  Affinity = ""
	USB  Affinity = "over_usb"
	WiFi Affinity = "over_wifi"
)

// Address is an advertised IPv4 address. Like busylib-py, addresses in
// 10.0.4.0/24 are classified as USB; all others are classified as Wi-Fi.
type Address struct {
	IP       netip.Addr
	Affinity Affinity
}

// Device is one discovered BUSY Bar, with addresses merged across interfaces.
// ID is the service instance name without its "busybar-" prefix.
type Device struct {
	ID        string
	Name      string
	Addresses []Address
}

// Address selects an address with the requested affinity. Any prefers USB.
// It returns false if no matching IPv4 address was discovered.
func (d Device) Address(affinity Affinity) (netip.Addr, bool) {
	if affinity == Any {
		if ip, ok := d.Address(USB); ok {
			return ip, true
		}
		return d.Address(WiFi)
	}
	for _, address := range d.Addresses {
		if address.Affinity == affinity {
			return address.IP, true
		}
	}
	return netip.Addr{}, false
}
