package appliance

import (
	"fmt"
	"fractalcr/internal/config"
	"net"
	"syscall"
)

func staticRoutePayload(index int, prefix string, route config.StaticRoute) ([]byte, error) {
	ip, network, err := net.ParseCIDR(prefix)
	if err != nil {
		return nil, err
	}
	bits, _ := network.Mask.Size()
	payload := make([]byte, 12)
	payload[0] = syscall.AF_INET
	address := ip.To4()
	if address == nil {
		payload[0] = syscall.AF_INET6
		address = ip.To16()
	}
	payload[1] = byte(bits)
	payload[4] = syscall.RT_TABLE_MAIN
	payload[5] = syscall.RTPROT_STATIC
	payload[7] = syscall.RTN_UNICAST
	payload = append(payload, nlAttribute(syscall.RTA_DST, address)...)
	if route.Gateway == "direct" {
		payload[6] = syscall.RT_SCOPE_LINK
	} else {
		gateway := net.ParseIP(route.Gateway)
		if gateway == nil {
			return nil, fmt.Errorf("invalid gateway")
		}
		value := gateway.To4()
		if value == nil {
			value = gateway.To16()
		}
		payload = append(payload, nlAttribute(syscall.RTA_GATEWAY, value)...)
	}
	payload = append(payload, nlAttribute(syscall.RTA_OIF, nlUint(uint32(index)))...)
	payload = append(payload, nlAttribute(syscall.RTA_PRIORITY, nlUint(route.Metric))...)
	return payload, nil
}

func staticRoutes(n config.Network, ipv6, remove bool) error {
	if len(n.Routes) == 0 {
		return nil
	}
	iface, err := resolveLink(n.Interface)
	if err != nil {
		return err
	}
	for _, prefix := range config.Keys(n.Routes) {
		ip, _, _ := net.ParseCIDR(prefix)
		if (ip.To4() == nil) != ipv6 {
			continue
		}
		payload, err := staticRoutePayload(iface.Index, prefix, n.Routes[prefix])
		if err != nil {
			return err
		}
		kind, flags := uint16(syscall.RTM_NEWROUTE), uint16(syscall.NLM_F_CREATE|syscall.NLM_F_REPLACE)
		if remove {
			kind = syscall.RTM_DELROUTE
			flags = 0
		}
		if err = routeRequest(kind, flags, payload); err != nil && !(remove && err == syscall.ESRCH) {
			return fmt.Errorf("route %s: %w", prefix, err)
		}
	}
	return nil
}
