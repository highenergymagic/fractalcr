package cli

import (
	"fmt"
	"slices"
	"strconv"
)

func init() {
	configCommands = append(configCommands, Command{"system container-engine", "Configure the container engine", false})
	modeCommands["engine"] = []string{"registry mirror <URL>", "no registry mirror <URL>", "registry insecure <REGISTRY>", "no registry insecure <REGISTRY>",
		"address-pool <PREFIX> size <1-30>", "no address-pool <PREFIX>", "logging driver <DRIVER>", "no logging driver", "logging option <KEY> <VALUE>", "no logging option <KEY>",
		"live-restore", "no live-restore", "debug", "no debug", "ipv6 enable", "no ipv6 enable", "ipv6 fixed-cidr <PREFIX>", "no ipv6 fixed-cidr",
		"bridge address <IPv4/PREFIX>", "no bridge address", "mtu <576-65535>", "no mtu", "max-concurrent-downloads <1-1024>", "no max-concurrent-downloads",
		"max-concurrent-uploads <1-1024>", "no max-concurrent-uploads", "shutdown-timeout <0-86400>", "no shutdown-timeout",
		"dns server <ADDRESS>", "no dns server <ADDRESS>", "dns search <DOMAIN>", "no dns search <DOMAIN>", "dns option <VALUE>", "no dns option <VALUE>", "label <KEY> <VALUE>", "no label <KEY>", "storage backend overlay2", "storage backend blockfile"}
}

func (s *Session) engine(w []string) (string, error) {
	e := &s.Candidate.Engine
	remove := w[0] == "no"
	if remove {
		w = w[1:]
	}
	setlist := func(p *[]string, v string) {
		if remove {
			*p = slices.DeleteFunc(*p, func(s string) bool { return s == v })
		} else if !slices.Contains(*p, v) {
			*p = append(*p, v)
		}
	}
	setmap := func(p *map[string]string, args []string) {
		if remove {
			delete(*p, args[0])
		} else {
			if *p == nil {
				*p = map[string]string{}
			}
			(*p)[args[0]] = args[1]
		}
	}
	value := ""
	if !remove {
		value = w[len(w)-1]
	}
	number, _ := strconv.Atoi(value)
	switch w[0] {
	case "storage":
		e.StorageBackend = w[2]
	case "registry":
		if w[1] == "mirror" {
			setlist(&e.RegistryMirrors, w[2])
		} else {
			setlist(&e.InsecureRegistries, w[2])
		}
	case "address-pool":
		if remove {
			delete(e.AddressPools, w[1])
		} else {
			if e.AddressPools == nil {
				e.AddressPools = map[string]int{}
			}
			e.AddressPools[w[1]] = number
		}
	case "logging":
		if w[1] == "driver" {
			e.LogDriver = value
		} else {
			setmap(&e.LogOptions, w[2:])
		}
	case "live-restore":
		e.LiveRestore = !remove
	case "debug":
		e.Debug = !remove
	case "ipv6":
		if w[1] == "enable" {
			e.IPv6 = !remove
		} else {
			e.FixedCIDRv6 = value
		}
	case "bridge":
		e.BridgeAddress = value
	case "mtu":
		e.MTU = number
	case "max-concurrent-downloads":
		e.MaxDownloads = number
	case "max-concurrent-uploads":
		e.MaxUploads = number
	case "shutdown-timeout":
		if remove {
			e.ShutdownTimeout = nil
		} else {
			e.ShutdownTimeout = &number
		}
	case "dns":
		switch w[1] {
		case "server":
			setlist(&e.DNS, w[2])
		case "search":
			setlist(&e.DNSSearch, w[2])
		case "option":
			setlist(&e.DNSOptions, w[2])
		}
	case "label":
		setmap(&e.Labels, w[1:])
	default:
		return "", fmt.Errorf("invalid engine setting")
	}
	return "", nil
}
