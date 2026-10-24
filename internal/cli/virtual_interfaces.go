package cli

import (
	"fmt"
	"fractalcr/internal/config"
	"slices"
)

func init() {
	modeCommands["interface"] = append(modeCommands["interface"], "parent <INTERFACE>", "no parent", "member <INTERFACE>", "no member <INTERFACE>", "channel-mode active-backup", "channel-mode 802.3ad", "channel-mode balance-xor", "channel-mode balance-rr", "channel-mode broadcast", "channel-mode balance-tlb", "channel-mode balance-alb", "no channel-mode")
}

func (s *Session) configureVirtualInterface(n *config.Network, words []string) (bool, error) {
	w := words
	remove := w[0] == "no"
	if remove {
		w = w[1:]
	}
	if w[0] != "parent" && w[0] != "member" && w[0] != "channel-mode" {
		return false, nil
	}
	_, kind, _, _ := config.VirtualInterface(n.Interface)
	if kind == "" {
		return true, fmt.Errorf("select a Vlan or Port-channel interface")
	}
	value := ""
	if len(w) > 1 {
		value = w[1]
	}
	if name, _, _, _ := config.VirtualInterface(value); name != "" {
		value = name
	}
	if (w[0] == "parent" && !remove || w[0] == "member") && s.InterfaceName != nil {
		resolved, err := s.InterfaceName(value)
		if err != nil {
			return true, err
		}
		value = resolved
	}
	switch w[0] {
	case "parent":
		if kind != "vlan" {
			return true, fmt.Errorf("parent applies to VLAN interfaces")
		}
		if remove {
			n.Parent = ""
		} else {
			n.Parent = value
		}
	case "channel-mode":
		if kind != "bond" {
			return true, fmt.Errorf("channel-mode applies to Port-channel interfaces")
		}
		if remove {
			n.BondMode = ""
		} else {
			n.BondMode = value
		}
	case "member":
		if kind != "bond" {
			return true, fmt.Errorf("members apply to Port-channel interfaces")
		}
		if remove {
			n.Members = slices.DeleteFunc(n.Members, func(v string) bool { return v == value })
		} else if !slices.Contains(n.Members, value) {
			n.Members = append(n.Members, value)
		}
	}
	return true, nil
}
