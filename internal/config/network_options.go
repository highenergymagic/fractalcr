package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func (n DockerNetwork) validateOptions() error {
	if n.IPAMDriver != "" && !DriverName.MatchString(n.IPAMDriver) {
		return fmt.Errorf("invalid IPAM driver")
	}
	for _, m := range []map[string]string{n.Options, n.Labels, n.IPAMOptions} {
		for k, v := range m {
			if k == "" || strings.ContainsAny(k, "=\x00\r\n") || strings.ContainsAny(v, "\x00\r\n") {
				return fmt.Errorf("invalid network option")
			}
		}
	}
	if _, ok := n.Options["parent"]; ok {
		return fmt.Errorf("use parent INTERFACE to select the parent")
	}
	for subnet, pool := range n.Pools {
		ip, cidr, err := net.ParseCIDR(subnet)
		if err != nil || !ip.Equal(cidr.IP) {
			return fmt.Errorf("IPAM pool must be a network prefix")
		}
		if ip.To4() == nil && !n.IPv6 {
			return fmt.Errorf("enable IPv6 before adding an IPv6 pool")
		}
		if subnet == n.Subnet {
			return fmt.Errorf("duplicate IPAM subnet")
		}
		if pool.Gateway != "" && (net.ParseIP(pool.Gateway) == nil || !cidr.Contains(net.ParseIP(pool.Gateway))) {
			return fmt.Errorf("IPAM gateway must belong to its pool")
		}
		if pool.Range != "" {
			rip, r, e := net.ParseCIDR(pool.Range)
			if e != nil || !rip.Equal(r.IP) {
				return fmt.Errorf("invalid IPAM allocation range")
			}
			a, ab := cidr.Mask.Size()
			b, bb := r.Mask.Size()
			if ab != bb || b < a || !cidr.Contains(r.IP) {
				return fmt.Errorf("IPAM allocation range must belong to its pool")
			}
		}
		for name, address := range pool.Aux {
			if !Name.MatchString(name) || net.ParseIP(address) == nil || !cidr.Contains(net.ParseIP(address)) {
				return fmt.Errorf("invalid IPAM reserved address")
			}
		}
	}
	return nil
}

func (n DockerNetwork) RenderOptionsCLI(line func(string, ...any)) {
	if n.IPv6 {
		line(" ipv6 enable")
	}
	if n.IPAMDriver != "" {
		line(" ipam driver %s", strconv.Quote(n.IPAMDriver))
	}
	for _, item := range []struct {
		key    string
		values map[string]string
	}{{"option", n.Options}, {"label", n.Labels}, {"ipam option", n.IPAMOptions}} {
		for _, k := range Keys(item.values) {
			line(" %s %s %s", item.key, strconv.Quote(k), strconv.Quote(item.values[k]))
		}
	}
	for _, subnet := range Keys(n.Pools) {
		p := n.Pools[subnet]
		line(" ipam pool %s", subnet)
		if p.Gateway != "" {
			line(" ipam pool %s gateway %s", subnet, p.Gateway)
		}
		if p.Range != "" {
			line(" ipam pool %s range %s", subnet, p.Range)
		}
		for _, name := range Keys(p.Aux) {
			line(" ipam pool %s reserve %s %s", subnet, name, p.Aux[name])
		}
	}
}

func (n DockerNetwork) IPAMAPI() map[string]any {
	pools := []map[string]any{}
	if n.Subnet != "" {
		pools = append(pools, map[string]any{"Subnet": n.Subnet, "Gateway": n.Gateway})
	}
	for _, subnet := range Keys(n.Pools) {
		p := n.Pools[subnet]
		pools = append(pools, map[string]any{"Subnet": subnet, "Gateway": p.Gateway, "IPRange": p.Range, "AuxiliaryAddresses": p.Aux})
	}
	result := map[string]any{"Config": pools, "Options": n.IPAMOptions}
	if n.IPAMDriver != "" {
		result["Driver"] = n.IPAMDriver
	}
	return result
}
