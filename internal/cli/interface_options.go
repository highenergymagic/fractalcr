package cli

import (
	"fmt"
	"fractalcr/internal/config"
	"net"
	"slices"
	"strconv"
	"strings"
)

func init() {
	execCommands = append(execCommands, Command{"show ipv6 route", "IPv6 routing table", false}, Command{"show ipv6 interface brief", "IPv6 interface address summary", false})
	modeCommands["interface"] = append(modeCommands["interface"], "description <TEXT...>", "no description", "mtu <576-65535>", "no mtu",
		"ip address <IPv4/PREFIX> <NETMASK>", "ip address <IPv4/PREFIX> <NETMASK> secondary",
		"no ip address <IPv4/PREFIX>", "no ip address <IPv4/PREFIX> <NETMASK>", "no ip address <IPv4/PREFIX> <NETMASK> secondary",
		"ip address <IPv4/PREFIX> secondary", "no ip address <IPv4/PREFIX> secondary",
		"ipv6 address <PREFIX>", "no ipv6 address <PREFIX>", "ipv6 gateway <ADDRESS>", "no ipv6 gateway")
}

func normalizeInterfaceAddress(words []string) ([]string, error) {
	offset := 0
	if words[0] == "no" {
		offset = 1
	}
	if len(words) < offset+4 || words[offset] != "ip" || words[offset+1] != "address" || words[offset+3] == "secondary" {
		return words, nil
	}
	ip := net.ParseIP(words[offset+2]).To4()
	mask := net.ParseIP(words[offset+3]).To4()
	if ip == nil || mask == nil {
		return nil, fmt.Errorf("Invalid IP address or subnet mask")
	}
	ones, bits := net.IPMask(mask).Size()
	if bits != 32 {
		return nil, fmt.Errorf("Bad mask %s", words[offset+3])
	}
	result := append([]string(nil), words[:offset+2]...)
	result = append(result, fmt.Sprintf("%s/%d", ip, ones))
	return append(result, words[offset+4:]...), nil
}

func configureInterfaceOptions(n *config.Network, words []string) bool {
	w := words
	remove := w[0] == "no"
	if remove {
		w = w[1:]
	}
	set := func(p *[]string, value string) {
		if remove {
			*p = slices.DeleteFunc(*p, func(s string) bool { return s == value })
		} else if !slices.Contains(*p, value) {
			*p = append(*p, value)
		}
	}
	switch w[0] {
	case "description":
		if remove {
			n.Description = ""
		} else {
			n.Description = strings.Join(w[1:], " ")
		}
	case "mtu":
		if remove {
			n.MTU = 0
		} else {
			n.MTU, _ = strconv.Atoi(w[1])
		}
	case "ipv6":
		if w[1] == "address" {
			set(&n.IPv6Addresses, w[2])
		} else if remove {
			n.IPv6Gateway = ""
		} else {
			n.IPv6Gateway = w[2]
		}
	case "ip":
		if len(w) != 4 || w[1] != "address" || w[3] != "secondary" {
			return false
		}
		set(&n.SecondaryAddresses, w[2])
	default:
		return false
	}
	return true
}
