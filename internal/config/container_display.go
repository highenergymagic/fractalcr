package config

import (
	"strconv"
	"strings"
)

func (v ContainerOptions) RenderCLI(line func(string, ...any)) {
	for _, name := range Keys(v.Networks) {
		e := v.Networks[name]
		line(" network attach %s", name)
		for _, alias := range e.Aliases {
			line(" network attach %s alias %s", name, alias)
		}
		if e.IPv4 != "" {
			line(" network attach %s ipv4 %s", name, e.IPv4)
		}
		if e.IPv6 != "" {
			line(" network attach %s ipv6 %s", name, e.IPv6)
		}
	}
	if v.GPUCount == -1 {
		line(" gpu all")
	} else if v.GPUCount > 0 {
		line(" gpu count %d", v.GPUCount)
	}
	for _, id := range v.GPUDevices {
		line(" gpu device %s", strconv.Quote(id))
	}
	for _, target := range Keys(v.Binds) {
		m := v.Binds[target]
		suffix := ""
		if m.ReadOnly {
			suffix = " read-only"
		}
		if m.Propagation != "" {
			suffix += " propagation " + m.Propagation
		}
		line(" bind %s %s%s", strconv.Quote(m.Source), strconv.Quote(target), suffix)
	}
	for _, option := range v.SecurityOptions {
		line(" security option %s", strconv.Quote(option))
	}
	for _, item := range []struct{ key, value string }{{"pid", v.PIDMode}, {"ipc", v.IPCMode}, {"uts", v.UTSMode}} {
		if item.value != "" {
			line(" namespace %s %s", item.key, strconv.Quote(item.value))
		}
	}
	for _, item := range []struct{ key, value string }{{"user", v.User}, {"guest-hostname", v.Hostname}, {"working-directory", v.WorkingDir}, {"cpu-set", v.CPUSet}, {"stop-signal", v.StopSignal}, {"logging driver", v.LogDriver}} {
		if item.value != "" {
			line(" %s %s", item.key, strconv.Quote(item.value))
		}
	}
	args := func(key string, values []string) {
		if len(values) == 0 {
			return
		}
		quoted := []string{}
		for _, s := range values {
			quoted = append(quoted, strconv.Quote(s))
		}
		line(" %s %s", key, strings.Join(quoted, " "))
	}
	args("entrypoint", v.Entrypoint)
	if v.NanoCPUs != 0 {
		line(" cpus %d.%09d", v.NanoCPUs/1000000000, v.NanoCPUs%1000000000)
	}
	if v.CPUShares != 0 {
		line(" cpu-shares %d", v.CPUShares)
	}
	for _, item := range []struct {
		key   string
		value int
	}{{"memory-reservation", v.MemoryReservationMiB}, {"memory-swap", v.MemorySwapMiB}, {"memory-shared", v.ShmMiB}} {
		if item.value == -1 {
			line(" %s unlimited", item.key)
		} else if item.value > 0 {
			line(" %s %dM", item.key, item.value)
		}
	}
	for _, item := range []struct {
		key   string
		value bool
	}{{"init", v.Init}, {"tty", v.TTY}, {"stdin-open", v.StdinOpen}, {"privileged", v.Privileged}} {
		if item.value {
			line(" %s", item.key)
		}
	}
	if v.StopTimeout != nil {
		line(" stop-timeout %d", *v.StopTimeout)
	}
	for _, item := range []struct {
		key    string
		values map[string]string
	}{{"label", v.Labels}, {"logging option", v.LogOptions}, {"host", v.ExtraHosts}, {"sysctl", v.Sysctls}, {"tmpfs", v.Tmpfs}} {
		for _, k := range Keys(item.values) {
			line(" %s %s %s", item.key, strconv.Quote(k), strconv.Quote(item.values[k]))
		}
	}
	for _, item := range []struct {
		key    string
		values []string
	}{{"capability add", v.CapAdd}, {"capability drop", v.CapDrop}, {"dns server", v.DNS}, {"dns search", v.DNSSearch}, {"dns option", v.DNSOptions}} {
		for _, s := range item.values {
			line(" %s %s", item.key, strconv.Quote(s))
		}
	}
	for _, target := range Keys(v.Devices) {
		d := v.Devices[target]
		line(" device %s %s %s", strconv.Quote(d.Source), strconv.Quote(target), d.Permissions)
	}
	for _, name := range Keys(v.Ulimits) {
		u := v.Ulimits[name]
		line(" ulimit %s %d %d", name, u.Soft, u.Hard)
	}
	if v.Health.Disabled {
		line(" healthcheck disable")
	} else {
		args("healthcheck command", v.Health.Command)
	}
	for _, item := range []struct{ key, value string }{{"interval", v.Health.Interval}, {"timeout", v.Health.Timeout}, {"start-period", v.Health.StartPeriod}, {"start-interval", v.Health.StartInterval}} {
		if item.value != "" {
			line(" healthcheck %s %s", item.key, item.value)
		}
	}
	if v.Health.Retries > 0 {
		line(" healthcheck retries %d", v.Health.Retries)
	}
}
