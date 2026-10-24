package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func (n Network) ValidateOptions() error {
	if n.MTU != 0 && (n.MTU < 576 || n.MTU > 65535) {
		return fmt.Errorf("interface MTU must be 576..65535")
	}
	if len(n.IPv6Addresses) > 0 && n.MTU > 0 && n.MTU < 1280 {
		return fmt.Errorf("IPv6 requires an MTU of at least 1280")
	}
	if len(n.Description) > 240 || strings.ContainsAny(n.Description, "\x00\r\n") {
		return fmt.Errorf("invalid interface description")
	}
	seen := map[string]bool{n.Address: true}
	for _, list := range []struct {
		addresses []string
		ipv6      bool
	}{{n.SecondaryAddresses, false}, {n.IPv6Addresses, true}} {
		for _, address := range list.addresses {
			ip, _, err := net.ParseCIDR(address)
			if err != nil || (ip.To4() == nil) != list.ipv6 || seen[address] {
				return fmt.Errorf("invalid or duplicate interface address")
			}
			seen[address] = true
		}
	}
	if n.DHCP && len(n.SecondaryAddresses) > 0 {
		return fmt.Errorf("secondary IPv4 addresses require static addressing")
	}
	if n.IPv6Gateway != "" {
		ip := net.ParseIP(n.IPv6Gateway)
		if ip == nil || ip.To4() != nil || !n.IsManagement() {
			return fmt.Errorf("IPv6 default gateway requires an IPv6 address on the management interface")
		}
	}
	return nil
}

func (n Network) RenderOptionsCLI(line func(string, ...any)) {
	if n.Description != "" {
		line(" description %s", strconv.Quote(n.Description))
	}
	if n.MTU != 0 {
		line(" mtu %d", n.MTU)
	}
	for _, address := range n.SecondaryAddresses {
		line(" ip address %s secondary", address)
	}
	for _, address := range n.IPv6Addresses {
		line(" ipv6 address %s", address)
	}
	if n.IPv6Gateway != "" {
		line(" ipv6 gateway %s", n.IPv6Gateway)
	}
}
