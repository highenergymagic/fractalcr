package config

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func (c Config) InterfaceOrder() []string {
	names := Keys(c.AllInterfaces())
	rank := func(name string) int {
		_, kind, _, _ := VirtualInterface(name)
		if kind == "bond" {
			return 1
		}
		if kind == "vlan" {
			return 2
		}
		return 0
	}
	sort.SliceStable(names, func(i, j int) bool { return rank(names[i]) < rank(names[j]) })
	return names
}

func (c Config) InterfaceMTU(name string) int {
	for _, n := range c.AllInterfaces() {
		for _, member := range n.Members {
			if member == name {
				return n.MTU
			}
		}
	}
	return c.AllInterfaces()[name].MTU
}

var virtualName = regexp.MustCompile(`(?i)^(vlan|vl|port-channel|po)([0-9]+)$`)

func VirtualInterface(name string) (canonical, kind, kernel string, id int) {
	m := virtualName.FindStringSubmatch(name)
	if m == nil {
		return
	}
	id, err := strconv.Atoi(m[2])
	if err != nil || id < 1 {
		id = 0
		return
	}
	if strings.HasPrefix(strings.ToLower(m[1]), "v") {
		if id > 4094 {
			id = 0
			return
		}
		return fmt.Sprintf("Vlan%d", id), "vlan", fmt.Sprintf("fcrvl%d", id), id
	}
	if id > 1024 {
		id = 0
		return
	}
	return fmt.Sprintf("Port-channel%d", id), "bond", fmt.Sprintf("fcrbond%d", id), id
}

func (c Config) validateVirtualInterfaces() error {
	all := c.AllInterfaces()
	members := map[string]string{}
	for name, n := range all {
		if n == nil {
			continue
		}
		canonical, kind, _, _ := VirtualInterface(name)
		if kind == "" {
			if n.Parent != "" || n.BondMode != "" || len(n.Members) > 0 {
				return fmt.Errorf("virtual interface settings require a Vlan or Port-channel interface")
			}
			continue
		}
		if canonical != name {
			return fmt.Errorf("virtual interface name must be %s", canonical)
		}
		if kind == "vlan" {
			if n.BondMode != "" || len(n.Members) > 0 {
				return fmt.Errorf("VLAN interfaces cannot have bond settings")
			}
			if n.Parent != "" {
				parent := all[n.Parent]
				_, parentKind, _, _ := VirtualInterface(n.Parent)
				if parent == nil || parentKind == "vlan" || parent.IsManagement() || parent.Shutdown {
					return fmt.Errorf("VLAN parent must be an enabled container-role physical or Port-channel interface")
				}
				parentMTU := c.InterfaceMTU(n.Parent)
				if parentMTU == 0 {
					parentMTU = 1500
				}
				if n.MTU > parentMTU {
					return fmt.Errorf("VLAN MTU exceeds its parent interface MTU")
				}
			} else if n.Address != "" || n.DHCP || len(n.IPv6Addresses) > 0 || len(n.Routes) > 0 {
				return fmt.Errorf("configure the VLAN parent before addressing")
			}
		} else {
			if n.Parent != "" {
				return fmt.Errorf("Port-channel interfaces use member interfaces, not a parent")
			}
			switch n.BondMode {
			case "", "active-backup", "802.3ad", "balance-xor", "balance-rr", "broadcast", "balance-tlb", "balance-alb":
			default:
				return fmt.Errorf("invalid Port-channel mode")
			}
			for _, member := range n.Members {
				p := all[member]
				_, memberKind, _, _ := VirtualInterface(member)
				if p == nil || memberKind != "" || p.IsManagement() || p.Shutdown || p.Address != "" || p.DHCP || len(p.SecondaryAddresses) > 0 || len(p.IPv6Addresses) > 0 || len(p.Routes) > 0 {
					return fmt.Errorf("Port-channel member must be an enabled container-role physical interface without layer-3 settings")
				}
				bondMTU := n.MTU
				if bondMTU == 0 {
					bondMTU = 1500
				}
				if p.MTU != 0 && p.MTU != bondMTU {
					return fmt.Errorf("member %s MTU must match Port-channel %s", member, name)
				}
				if owner, ok := members[member]; ok {
					return fmt.Errorf("interface %s already belongs to %s", member, owner)
				}
				members[member] = name
			}
		}
	}
	for name, n := range all {
		if n != nil && n.Parent != "" {
			if owner := members[n.Parent]; owner != "" {
				return fmt.Errorf("VLAN %s must use Port-channel %s rather than its member", name, owner)
			}
		}
	}
	return nil
}

func (n Network) RenderVirtualCLI(line func(string, ...any)) {
	if n.Parent != "" {
		line(" parent %s", n.Parent)
	}
	if n.BondMode != "" {
		line(" channel-mode %s", n.BondMode)
	}
	for _, member := range n.Members {
		line(" member %s", member)
	}
}
