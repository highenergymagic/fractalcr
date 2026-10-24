package config

import (
	"fmt"
	"net"
	"strings"
)

type NTP struct {
	Servers []string `json:"servers,omitempty"`
}
type DockerNetwork struct {
	IPv6        bool                `json:"ipv6,omitempty"`
	IPAMDriver  string              `json:"ipam_driver,omitempty"`
	IPAMOptions map[string]string   `json:"ipam_options,omitempty"`
	Pools       map[string]IPAMPool `json:"pools,omitempty"`
	Options     map[string]string   `json:"options,omitempty"`
	Labels      map[string]string   `json:"labels,omitempty"`
	Driver      string              `json:"driver"`
	Subnet      string              `json:"subnet,omitempty"`
	Gateway     string              `json:"gateway,omitempty"`
	Parent      string              `json:"parent,omitempty"`
	Internal    bool                `json:"internal,omitempty"`
	Attachable  bool                `json:"attachable,omitempty"`
}

type IPAMPool struct {
	Gateway string            `json:"gateway,omitempty"`
	Range   string            `json:"range,omitempty"`
	Aux     map[string]string `json:"aux,omitempty"`
}

// AllInterfaces retains compatibility with version-1 single-interface configs.
func (c Config) AllInterfaces() map[string]*Network {
	out := map[string]*Network{}
	if c.Network != nil {
		out[c.Network.Interface] = c.Network
	}
	for k, v := range c.Interfaces {
		out[k] = v
	}
	return out
}
func (n Network) IsManagement() bool { return n.Role == "" || n.Role == "management" }
func (c *Config) EditInterface(name string) *Network {
	if c.Network != nil && c.Network.Interface == name {
		return c.Network
	}
	if c.Network == nil && len(c.Interfaces) == 0 {
		c.Network = &Network{Interface: name}
		return c.Network
	}
	if c.Interfaces == nil {
		c.Interfaces = map[string]*Network{}
	}
	if c.Interfaces[name] == nil {
		c.Interfaces[name] = &Network{Interface: name, Role: "container"}
	}
	return c.Interfaces[name]
}
func (c Config) validateNetworking() error {
	management := 0
	routes := map[string]bool{}
	if c.Network != nil {
		if _, ok := c.Interfaces[c.Network.Interface]; ok {
			return fmt.Errorf("duplicate interface")
		}
	}
	for name, n := range c.AllInterfaces() {
		if n == nil || n.Interface != name || !Interface.MatchString(name) || name == "lo" {
			return fmt.Errorf("invalid interface")
		}
		if n.Role != "" && n.Role != "management" && n.Role != "container" {
			return fmt.Errorf("invalid interface role")
		}
		for prefix, route := range n.Routes {
			key := fmt.Sprintf("%s/%d", prefix, route.Metric)
			if routes[key] {
				return fmt.Errorf("duplicate static route prefix and metric across interfaces")
			}
			routes[key] = true
		}
		if n.IsManagement() {
			management++
		}
		if n.DHCP && (n.Address != "" || n.Gateway != "" || n.DNS != "") {
			return fmt.Errorf("DHCP cannot be combined with static address, gateway or DNS")
		}
		if !n.IsManagement() && (n.Gateway != "" || n.DNS != "") {
			return fmt.Errorf("default gateway and DNS belong to the management interface")
		}
	}
	if management > 1 {
		return fmt.Errorf("select one management interface")
	}
	if len(c.NTP.Servers) > 8 {
		return fmt.Errorf("at most eight NTP servers")
	}
	for _, s := range c.NTP.Servers {
		if !validServer(s) {
			return fmt.Errorf("invalid NTP server")
		}
	}
	for name, n := range c.Networks {
		if err := n.validateOptions(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !Name.MatchString(name) || name == "bridge" || name == "host" || name == "none" {
			return fmt.Errorf("invalid or reserved Docker network name")
		}
		switch n.Driver {
		case "bridge", "overlay", "macvlan", "ipvlan":
		default:
			if !DriverName.MatchString(n.Driver) {
				return fmt.Errorf("invalid Docker network driver")
			}
		}
		if n.Driver == "macvlan" || n.Driver == "ipvlan" {
			parent := c.AllInterfaces()[n.Parent]
			if n.Parent != "" && (parent == nil || parent.IsManagement() || parent.Shutdown) {
				return fmt.Errorf("network %s requires an enabled container-role parent interface", name)
			}
		} else if n.Parent != "" {
			return fmt.Errorf("parent is only valid for macvlan or ipvlan")
		}
		if n.Subnet != "" {
			ip, sub, e := net.ParseCIDR(n.Subnet)
			if e != nil || ip.To4() == nil || !ip.Equal(sub.IP) {
				return fmt.Errorf("network subnet must be an IPv4 network/prefix")
			}
			if n.Gateway != "" {
				gw := net.ParseIP(n.Gateway)
				if gw.To4() == nil || !sub.Contains(gw) {
					return fmt.Errorf("network gateway must be within its subnet")
				}
			}
		} else if n.Gateway != "" {
			return fmt.Errorf("network gateway requires a subnet")
		}
	}
	return nil
}
func validServer(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}
