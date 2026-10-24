package config

import (
	"fmt"
	"net"
)

type StaticRoute struct {
	Gateway string `json:"gateway"`
	Metric  uint32 `json:"metric"`
}

func (n Network) ValidateRoutes() error {
	for prefix, route := range n.Routes {
		ip, network, err := net.ParseCIDR(prefix)
		if err != nil || !ip.Equal(network.IP) {
			return fmt.Errorf("static route destination must be a network prefix")
		}
		if route.Gateway != "direct" {
			gateway := net.ParseIP(route.Gateway)
			if gateway == nil || (gateway.To4() == nil) != (ip.To4() == nil) || gateway.IsUnspecified() || gateway.IsMulticast() {
				return fmt.Errorf("static route gateway must match the destination address family")
			}
		}
	}
	return nil
}

func (n Network) RenderRoutesCLI(line func(string, ...any)) {
	for _, prefix := range Keys(n.Routes) {
		route := n.Routes[prefix]
		family := "ip"
		ip, _, _ := net.ParseCIDR(prefix)
		if ip.To4() == nil {
			family = "ipv6"
		}
		line("%s route %s %s %s metric %d", family, prefix, n.Interface, route.Gateway, route.Metric)
	}
}
