package cli

import (
	"fmt"
	"fractalcr/internal/config"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var cpuDecimal = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,9})?$`)

func init() {
	modeCommands["container"] = append(modeCommands["container"],
		"user <VALUE>", "guest-hostname <NAME>", "working-directory <PATH>", "entrypoint <ARG...>",
		"label <KEY> <VALUE>", "cpus <CPUS>", "cpu-shares <2-262144>", "cpu-set <CPU-LIST>",
		"memory-reservation <MEMORY>", "memory-swap <MEMORY>", "memory-swap unlimited", "memory-shared <MEMORY>",
		"init", "tty", "stdin-open", "privileged", "stop-signal <SIGNAL>", "stop-timeout <SECONDS>",
		"logging driver <DRIVER>", "logging option <KEY> <VALUE>",
		"capability add <CAPABILITY>", "capability drop <CAPABILITY>",
		"dns server <ADDRESS>", "dns search <DOMAIN>", "dns option <VALUE>", "host <NAME> <ADDRESS>",
		"sysctl <KEY> <VALUE>", "tmpfs <PATH> <OPTIONS>", "device <PATH> <PATH> <PERMISSIONS>",
		"ulimit <NAME> <SOFT> <HARD>", "healthcheck command <ARG...>", "healthcheck disable",
		"healthcheck interval <DURATION>", "healthcheck timeout <DURATION>", "healthcheck start-period <DURATION>",
		"healthcheck start-interval <DURATION>", "healthcheck retries <1-1000000>",
		"no label <KEY>", "no logging option <KEY>", "no capability add <CAPABILITY>", "no capability drop <CAPABILITY>",
		"no dns server <ADDRESS>", "no dns search <DOMAIN>", "no dns option <VALUE>",
		"no host <NAME>", "no sysctl <KEY>", "no tmpfs <PATH>", "no device <PATH>", "no ulimit <NAME>")
	for _, key := range []string{"user", "guest-hostname", "working-directory", "entrypoint", "cpus", "cpu-shares", "cpu-set", "memory", "memory-reservation", "memory-swap", "memory-shared", "pids-limit", "init", "tty", "stdin-open", "privileged", "stop-signal", "stop-timeout", "logging", "healthcheck", "image", "restart-policy"} {
		modeCommands["container"] = append(modeCommands["container"], "no "+key)
	}
}

func configureContainerOptions(c *config.Container, words []string) (bool, error) {
	w := words
	remove := w[0] == "no"
	if remove {
		w = w[1:]
	}
	if len(w) == 0 {
		return false, nil
	}
	value := ""
	if len(w) > 1 {
		value = w[1]
	}
	str := func(p *string) {
		if remove {
			*p = ""
		} else {
			*p = value
		}
	}
	integer := func(p *int64) error {
		if remove {
			*p = 0
			return nil
		}
		n, e := strconv.ParseInt(value, 10, 64)
		if e != nil {
			return fmt.Errorf("integer required")
		}
		*p = n
		return nil
	}
	mem := func(p *int) error {
		if remove {
			*p = 0
			return nil
		}
		n, e := memorySize(value, 1)
		if e != nil {
			return e
		}
		*p = n
		return nil
	}
	setmap := func(m *map[string]string, args []string) {
		if remove {
			delete(*m, args[0])
			return
		}
		if *m == nil {
			*m = map[string]string{}
		}
		(*m)[args[0]] = args[1]
	}
	setlist := func(p *[]string, value string) {
		if remove {
			*p = slices.DeleteFunc(*p, func(s string) bool { return s == value })
		} else if !slices.Contains(*p, value) {
			*p = append(*p, value)
		}
	}
	switch w[0] {
	case "user":
		str(&c.User)
	case "guest-hostname":
		str(&c.Hostname)
	case "working-directory":
		str(&c.WorkingDir)
	case "entrypoint":
		if remove {
			c.Entrypoint = nil
		} else {
			c.Entrypoint = append([]string{}, w[1:]...)
		}
	case "cpus":
		if remove {
			c.NanoCPUs = 0
			break
		}
		if !cpuDecimal.MatchString(value) {
			return true, fmt.Errorf("CPU count must be a decimal number with at most nine decimal places")
		}
		n, ok := new(big.Rat).SetString(value)
		if !ok {
			return true, fmt.Errorf("CPU count must be a decimal number")
		}
		n.Mul(n, big.NewRat(1000000000, 1))
		if !n.IsInt() || !n.Num().IsInt64() || n.Sign() <= 0 {
			return true, fmt.Errorf("CPU count must be positive with at most nine decimal places")
		}
		c.NanoCPUs = n.Num().Int64()
	case "cpu-shares":
		return true, integer(&c.CPUShares)
	case "cpu-set":
		str(&c.CPUSet)
	case "memory-reservation":
		return true, mem(&c.MemoryReservationMiB)
	case "memory-swap":
		if value == "unlimited" && !remove {
			c.MemorySwapMiB = -1
		} else {
			return true, mem(&c.MemorySwapMiB)
		}
	case "memory-shared":
		return true, mem(&c.ShmMiB)
	case "init":
		c.Init = !remove
	case "tty":
		c.TTY = !remove
	case "stdin-open":
		c.StdinOpen = !remove
	case "privileged":
		c.Privileged = !remove
	case "stop-signal":
		str(&c.StopSignal)
	case "stop-timeout":
		if remove {
			c.StopTimeout = nil
		} else {
			n, e := strconv.Atoi(value)
			if e != nil {
				return true, fmt.Errorf("stop timeout must be seconds")
			}
			c.StopTimeout = &n
		}
	case "label":
		setmap(&c.Labels, w[1:])
	case "logging":
		if remove && len(w) == 1 {
			c.LogDriver = ""
			c.LogOptions = nil
		} else if w[1] == "driver" {
			c.LogDriver = w[2]
		} else {
			setmap(&c.LogOptions, w[2:])
		}
	case "capability":
		if w[1] == "drop" && strings.EqualFold(w[2], "all") {
			return false, nil
		}
		if w[1] == "add" {
			setlist(&c.CapAdd, strings.ToUpper(w[2]))
		} else {
			setlist(&c.CapDrop, strings.ToUpper(w[2]))
		}
	case "dns":
		switch w[1] {
		case "server":
			setlist(&c.DNS, w[2])
		case "search":
			setlist(&c.DNSSearch, w[2])
		case "option":
			setlist(&c.DNSOptions, w[2])
		}
	case "host":
		setmap(&c.ExtraHosts, w[1:])
	case "sysctl":
		setmap(&c.Sysctls, w[1:])
	case "tmpfs":
		setmap(&c.Tmpfs, w[1:])
	case "device":
		if remove {
			delete(c.Devices, w[1])
		} else {
			if c.Devices == nil {
				c.Devices = map[string]config.Device{}
			}
			c.Devices[w[2]] = config.Device{Source: w[1], Permissions: w[3]}
		}
	case "ulimit":
		if remove {
			delete(c.Ulimits, w[1])
		} else {
			soft, e := strconv.ParseInt(w[2], 10, 64)
			hard, e2 := strconv.ParseInt(w[3], 10, 64)
			if e != nil || e2 != nil {
				return true, fmt.Errorf("ulimit requires integer soft and hard limits")
			}
			if c.Ulimits == nil {
				c.Ulimits = map[string]config.Ulimit{}
			}
			c.Ulimits[w[1]] = config.Ulimit{Soft: soft, Hard: hard}
		}
	case "healthcheck":
		if remove {
			c.Health = config.Healthcheck{}
			break
		}
		switch w[1] {
		case "command":
			c.Health.Command = append([]string{}, w[2:]...)
			c.Health.Disabled = false
		case "disable":
			c.Health = config.Healthcheck{Disabled: true}
		case "interval":
			c.Health.Interval = w[2]
		case "timeout":
			c.Health.Timeout = w[2]
		case "start-period":
			c.Health.StartPeriod = w[2]
		case "start-interval":
			c.Health.StartInterval = w[2]
		case "retries":
			c.Health.Retries, _ = strconv.Atoi(w[2])
		}
	case "memory":
		if !remove {
			return false, nil
		}
		c.MemoryMiB = 0
	case "pids-limit":
		if !remove {
			return false, nil
		}
		c.Pids = 0
	case "image":
		if !remove {
			return false, nil
		}
		c.Image = ""
	case "restart-policy":
		if !remove {
			return false, nil
		}
		c.Restart = ""
	default:
		return false, nil
	}
	return true, nil
}
