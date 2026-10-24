package cli

import (
	"fmt"
	"fractalcr/internal/config"
	"net"
	"strconv"
)

func init() {
	for _, family := range []string{"ip", "ipv6"} {
		configCommands = append(configCommands,
			Command{family + " route <PREFIX> <INTERFACE> <GATEWAY>", "Configure a static route", false},
			Command{family + " route <PREFIX> <INTERFACE> <GATEWAY> metric <0-4294967295>", "Configure a static route and metric", false},
			Command{"no " + family + " route <PREFIX> <INTERFACE>", "Remove a static route", false})
	}
}

func (s *Session) staticRoute(words []string) (bool, error) {
	w := words
	remove := w[0] == "no"
	if remove {
		w = w[1:]
	}
	if len(w) < 4 || (w[0] != "ip" && w[0] != "ipv6") || w[1] != "route" {
		return false, nil
	}
	name := w[3]
	if s.InterfaceName != nil {
		resolved, err := s.InterfaceName(name)
		if err != nil {
			return true, err
		}
		name = resolved
	}
	n := s.Candidate.AllInterfaces()[name]
	if n == nil {
		return true, fmt.Errorf("configure interface %s before adding routes", name)
	}
	ip, _, err := net.ParseCIDR(w[2])
	if err != nil || (ip.To4() == nil) != (w[0] == "ipv6") {
		return true, fmt.Errorf("route prefix does not match the address family")
	}
	if remove {
		delete(n.Routes, w[2])
		return true, nil
	}
	metric := uint64(100)
	if len(w) == 7 {
		metric, err = strconv.ParseUint(w[6], 10, 32)
		if err != nil {
			return true, err
		}
	}
	if n.Routes == nil {
		n.Routes = map[string]config.StaticRoute{}
	}
	n.Routes[w[2]] = config.StaticRoute{Gateway: w[4], Metric: uint32(metric)}
	return true, nil
}
