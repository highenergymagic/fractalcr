package cli

import (
	"fmt"
	"fractalcr/internal/config"
	"slices"
	"strconv"
)

func init() {
	modeCommands["container"] = append(modeCommands["container"],
		"network attach <NAME>", "no network attach <NAME>", "network attach <NAME> alias <NAME>", "no network attach <NAME> alias <NAME>",
		"network attach <NAME> ipv4 <ADDRESS>", "no network attach <NAME> ipv4", "network attach <NAME> ipv6 <ADDRESS>", "no network attach <NAME> ipv6",
		"gpu count <1-1048576>", "gpu all", "gpu device <ID>", "no gpu", "no gpu device <ID>",
		"bind <PATH> <PATH>", "bind <PATH> <PATH> read-only", "bind <PATH> <PATH> propagation <MODE>", "bind <PATH> <PATH> read-only propagation <MODE>", "no bind <PATH>",
		"security option <VALUE>", "no security option <VALUE>", "namespace pid <MODE>", "namespace ipc <MODE>", "namespace uts <MODE>", "no namespace pid", "no namespace ipc", "no namespace uts",
		"no publish <PORT-MAPPING>", "no volume <PATH>")
}

func configureAttachments(c *config.Container, words []string) (bool, error) {
	w := words
	remove := w[0] == "no"
	if remove {
		w = w[1:]
	}
	switch w[0] {
	case "network":
		if len(w) < 3 || w[1] != "attach" {
			return false, nil
		}
		name := w[2]
		if remove && len(w) == 3 {
			delete(c.Networks, name)
			break
		}
		if c.Networks == nil {
			c.Networks = map[string]config.Endpoint{}
		}
		e := c.Networks[name]
		if len(w) > 3 {
			value := ""
			if !remove || w[3] == "alias" {
				value = w[4]
			}
			switch w[3] {
			case "alias":
				if remove {
					e.Aliases = slices.DeleteFunc(e.Aliases, func(s string) bool { return s == value })
				} else if !slices.Contains(e.Aliases, value) {
					e.Aliases = append(e.Aliases, value)
				}
			case "ipv4":
				e.IPv4 = value
			case "ipv6":
				e.IPv6 = value
			}
		}
		c.Networks[name] = e
	case "gpu":
		if remove && len(w) == 1 {
			c.GPUCount = 0
			c.GPUDevices = nil
			break
		}
		switch w[1] {
		case "all":
			c.GPUCount = -1
			c.GPUDevices = nil
		case "count":
			c.GPUCount, _ = strconv.Atoi(w[2])
			c.GPUDevices = nil
		case "device":
			if remove {
				c.GPUDevices = slices.DeleteFunc(c.GPUDevices, func(s string) bool { return s == w[2] })
			} else {
				c.GPUCount = 0
				if !slices.Contains(c.GPUDevices, w[2]) {
					c.GPUDevices = append(c.GPUDevices, w[2])
				}
			}
		}
	case "bind":
		if remove {
			delete(c.Binds, w[1])
			break
		}
		m := config.BindMount{Source: w[1]}
		for i := 3; i < len(w); i++ {
			if w[i] == "read-only" {
				m.ReadOnly = true
			} else if w[i] == "propagation" {
				i++
				m.Propagation = w[i]
			}
		}
		if c.Binds == nil {
			c.Binds = map[string]config.BindMount{}
		}
		c.Binds[w[2]] = m
	case "security":
		if len(w) < 3 || w[1] != "option" {
			return false, nil
		}
		if remove {
			c.SecurityOptions = slices.DeleteFunc(c.SecurityOptions, func(s string) bool { return s == w[2] })
		} else if !slices.Contains(c.SecurityOptions, w[2]) {
			c.SecurityOptions = append(c.SecurityOptions, w[2])
		}
	case "namespace":
		value := ""
		if !remove {
			value = w[2]
		}
		switch w[1] {
		case "pid":
			c.PIDMode = value
		case "ipc":
			c.IPCMode = value
		case "uts":
			c.UTSMode = value
		default:
			return true, fmt.Errorf("unknown namespace")
		}
	case "publish":
		if !remove || len(w) != 2 {
			return false, nil
		}
		c.Publish = slices.DeleteFunc(c.Publish, func(s string) bool { return s == w[1] })
	case "volume":
		if !remove || len(w) != 2 {
			return false, nil
		}
		c.Volumes = slices.DeleteFunc(c.Volumes, func(v config.Volume) bool { return v.Target == w[1] })
	default:
		return false, nil
	}
	return true, nil
}
