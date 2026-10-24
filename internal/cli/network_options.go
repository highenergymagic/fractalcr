package cli

import "fractalcr/internal/config"

func init() {
	modeCommands["network"] = append(modeCommands["network"], "driver <DRIVER>")
	modeCommands["network"] = append(modeCommands["network"], "ipv6 enable", "no ipv6 enable", "option <KEY> <VALUE>", "no option <KEY>", "label <KEY> <VALUE>", "no label <KEY>",
		"ipam driver <DRIVER>", "no ipam driver", "ipam option <KEY> <VALUE>", "no ipam option <KEY>",
		"ipam pool <PREFIX>", "no ipam pool <PREFIX>", "ipam pool <PREFIX> gateway <ADDRESS>", "ipam pool <PREFIX> range <PREFIX>", "ipam pool <PREFIX> reserve <NAME> <ADDRESS>",
		"no ipam pool <PREFIX> gateway", "no ipam pool <PREFIX> range", "no ipam pool <PREFIX> reserve <NAME>")
}

func configureNetworkOptions(n *config.DockerNetwork, words []string) bool {
	w := words
	remove := w[0] == "no"
	if remove {
		w = w[1:]
	}
	set := func(m *map[string]string, args []string) {
		if remove {
			delete(*m, args[0])
		} else {
			if *m == nil {
				*m = map[string]string{}
			}
			(*m)[args[0]] = args[1]
		}
	}
	switch w[0] {
	case "ipv6":
		n.IPv6 = !remove
	case "option":
		set(&n.Options, w[1:])
	case "label":
		set(&n.Labels, w[1:])
	case "ipam":
		switch w[1] {
		case "driver":
			if remove {
				n.IPAMDriver = ""
			} else {
				n.IPAMDriver = w[2]
			}
		case "option":
			set(&n.IPAMOptions, w[2:])
		case "pool":
			if remove && len(w) == 3 {
				delete(n.Pools, w[2])
				break
			}
			if n.Pools == nil {
				n.Pools = map[string]config.IPAMPool{}
			}
			p := n.Pools[w[2]]
			if len(w) > 3 {
				value := ""
				if !remove {
					value = w[4]
				}
				switch w[3] {
				case "gateway":
					p.Gateway = value
				case "range":
					p.Range = value
				case "reserve":
					set(&p.Aux, w[4:])
				}
			}
			n.Pools[w[2]] = p
		}
	default:
		return false
	}
	return true
}
