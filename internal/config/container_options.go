package config

import (
	"fmt"
	"net"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ContainerOptions is persisted with the container definition. These values
// become Engine API fields, never command-line fragments.
type ContainerOptions struct {
	Networks             map[string]Endpoint  `json:"networks,omitempty"`
	GPUDevices           []string             `json:"gpu_devices,omitempty"`
	GPUCount             int                  `json:"gpu_count,omitempty"`
	Binds                map[string]BindMount `json:"binds,omitempty"`
	SecurityOptions      []string             `json:"security_options,omitempty"`
	PIDMode              string               `json:"pid_mode,omitempty"`
	IPCMode              string               `json:"ipc_mode,omitempty"`
	UTSMode              string               `json:"uts_mode,omitempty"`
	User                 string               `json:"user,omitempty"`
	Hostname             string               `json:"hostname,omitempty"`
	WorkingDir           string               `json:"working_dir,omitempty"`
	Entrypoint           []string             `json:"entrypoint,omitempty"`
	Labels               map[string]string    `json:"labels,omitempty"`
	NanoCPUs             int64                `json:"nano_cpus,omitempty"`
	CPUShares            int64                `json:"cpu_shares,omitempty"`
	CPUSet               string               `json:"cpu_set,omitempty"`
	MemoryReservationMiB int                  `json:"memory_reservation_mib,omitempty"`
	MemorySwapMiB        int                  `json:"memory_swap_mib,omitempty"`
	ShmMiB               int                  `json:"shm_mib,omitempty"`
	Init                 bool                 `json:"init,omitempty"`
	TTY                  bool                 `json:"tty,omitempty"`
	StdinOpen            bool                 `json:"stdin_open,omitempty"`
	Privileged           bool                 `json:"privileged,omitempty"`
	StopSignal           string               `json:"stop_signal,omitempty"`
	StopTimeout          *int                 `json:"stop_timeout,omitempty"`
	LogDriver            string               `json:"log_driver,omitempty"`
	LogOptions           map[string]string    `json:"log_options,omitempty"`
	CapAdd               []string             `json:"cap_add,omitempty"`
	CapDrop              []string             `json:"cap_drop,omitempty"`
	DNS                  []string             `json:"dns,omitempty"`
	DNSSearch            []string             `json:"dns_search,omitempty"`
	DNSOptions           []string             `json:"dns_options,omitempty"`
	ExtraHosts           map[string]string    `json:"extra_hosts,omitempty"`
	Sysctls              map[string]string    `json:"sysctls,omitempty"`
	Tmpfs                map[string]string    `json:"tmpfs,omitempty"`
	Devices              map[string]Device    `json:"devices,omitempty"`
	Ulimits              map[string]Ulimit    `json:"ulimits,omitempty"`
	Health               Healthcheck          `json:"healthcheck,omitempty"`
}
type Endpoint struct {
	Aliases []string `json:"aliases,omitempty"`
	IPv4    string   `json:"ipv4,omitempty"`
	IPv6    string   `json:"ipv6,omitempty"`
}
type BindMount struct {
	Source      string `json:"source"`
	ReadOnly    bool   `json:"read_only,omitempty"`
	Propagation string `json:"propagation,omitempty"`
}
type Device struct {
	Source      string `json:"source"`
	Permissions string `json:"permissions"`
}
type Ulimit struct {
	Soft int64 `json:"soft"`
	Hard int64 `json:"hard"`
}
type Healthcheck struct {
	Command       []string `json:"command,omitempty"`
	Disabled      bool     `json:"disabled,omitempty"`
	Interval      string   `json:"interval,omitempty"`
	Timeout       string   `json:"timeout,omitempty"`
	StartPeriod   string   `json:"start_period,omitempty"`
	StartInterval string   `json:"start_interval,omitempty"`
	Retries       int      `json:"retries,omitempty"`
}

var cpuSetPattern = regexp.MustCompile(`^[0-9]+(-[0-9]+)?(,[0-9]+(-[0-9]+)?)*$`)
var capabilityPattern = regexp.MustCompile(`^(CAP_)?[A-Z][A-Z0-9_]*$`)

func (v ContainerOptions) Validate() error {
	if v.GPUCount < -1 || v.GPUCount > 1048576 || (v.GPUCount != 0 && len(v.GPUDevices) > 0) {
		return fmt.Errorf("choose a GPU count or explicit GPU devices")
	}
	for _, id := range v.GPUDevices {
		if id == "" || strings.ContainsAny(id, "\x00\r\n,") {
			return fmt.Errorf("invalid GPU device")
		}
	}
	for name, e := range v.Networks {
		if !Name.MatchString(name) || name == "host" || name == "none" {
			return fmt.Errorf("invalid attached network")
		}
		if e.IPv4 != "" && net.ParseIP(e.IPv4).To4() == nil {
			return fmt.Errorf("endpoint requires an IPv4 address")
		}
		if e.IPv6 != "" && (net.ParseIP(e.IPv6) == nil || net.ParseIP(e.IPv6).To4() != nil) {
			return fmt.Errorf("endpoint requires an IPv6 address")
		}
		for _, alias := range e.Aliases {
			if !validServer(alias) {
				return fmt.Errorf("invalid network alias")
			}
		}
	}
	for target, m := range v.Binds {
		if !path.IsAbs(target) || target == "/" || path.Clean(target) != target || !path.IsAbs(m.Source) || path.Clean(m.Source) != m.Source || strings.ContainsAny(target+m.Source, "\x00\r\n") {
			return fmt.Errorf("bind mount requires absolute normalized source and non-root target paths")
		}
		switch m.Propagation {
		case "", "private", "rprivate", "shared", "rshared", "slave", "rslave":
		default:
			return fmt.Errorf("invalid bind propagation")
		}
	}
	containerMode := func(mode string) bool {
		return strings.HasPrefix(mode, "container:") && Name.MatchString(strings.TrimPrefix(mode, "container:"))
	}
	if v.PIDMode != "" && v.PIDMode != "host" && v.PIDMode != "private" && !containerMode(v.PIDMode) {
		return fmt.Errorf("invalid PID namespace mode")
	}
	if v.IPCMode != "" && v.IPCMode != "host" && v.IPCMode != "private" && v.IPCMode != "shareable" && v.IPCMode != "none" && !containerMode(v.IPCMode) {
		return fmt.Errorf("invalid IPC namespace mode")
	}
	if v.UTSMode != "" && v.UTSMode != "host" {
		return fmt.Errorf("UTS namespace mode must be host or default")
	}
	for _, option := range v.SecurityOptions {
		if option == "" || strings.ContainsAny(option, "\x00\r\n") {
			return fmt.Errorf("invalid security option")
		}
	}
	if v.NanoCPUs < 0 || v.NanoCPUs > 1000000*1e9 || v.CPUShares < 0 || (v.CPUShares > 0 && v.CPUShares < 2) || v.CPUShares > 262144 {
		return fmt.Errorf("invalid CPU allocation")
	}
	if v.CPUSet != "" && !cpuSetPattern.MatchString(v.CPUSet) {
		return fmt.Errorf("CPU set requires a comma-separated list of CPU numbers or ranges")
	}
	if v.CPUSet != "" {
		for _, span := range strings.Split(v.CPUSet, ",") {
			low, high, ranged := strings.Cut(span, "-")
			first, err := strconv.ParseUint(low, 10, 31)
			if err != nil {
				return fmt.Errorf("invalid CPU number")
			}
			if ranged {
				last, err := strconv.ParseUint(high, 10, 31)
				if err != nil || last < first {
					return fmt.Errorf("CPU ranges must be ascending")
				}
			}
		}
	}
	if v.MemoryReservationMiB < 0 || v.MemoryReservationMiB > 1048576 || v.MemorySwapMiB < -1 || v.MemorySwapMiB > 2097152 || v.ShmMiB < 0 || v.ShmMiB > 1048576 {
		return fmt.Errorf("invalid memory allocation")
	}
	if v.WorkingDir != "" && !path.IsAbs(v.WorkingDir) {
		return fmt.Errorf("working directory must be absolute")
	}
	if v.Hostname != "" && !validServer(v.Hostname) {
		return fmt.Errorf("invalid container hostname")
	}
	if v.StopTimeout != nil && (*v.StopTimeout < -1 || *v.StopTimeout > 86400) {
		return fmt.Errorf("stop timeout must be -1..86400 seconds")
	}
	for _, value := range append(append([]string{v.User, v.WorkingDir, v.StopSignal}, v.Entrypoint...), v.Health.Command...) {
		if strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("container setting contains control delimiters")
		}
	}
	for _, m := range []map[string]string{v.Labels, v.LogOptions, v.ExtraHosts, v.Sysctls, v.Tmpfs} {
		for k, value := range m {
			if k == "" || strings.ContainsAny(k, "\x00\r\n=") || strings.ContainsAny(value, "\x00\r\n") {
				return fmt.Errorf("invalid container option")
			}
		}
	}
	for target := range v.Tmpfs {
		if !path.IsAbs(target) || path.Clean(target) == "/" {
			return fmt.Errorf("tmpfs target must be an absolute non-root path")
		}
	}
	for target, d := range v.Devices {
		if !strings.HasPrefix(target, "/dev/") || !strings.HasPrefix(d.Source, "/dev/") || path.Clean(target) != target || path.Clean(d.Source) != d.Source || d.Permissions == "" || strings.Trim(d.Permissions, "rwm") != "" {
			return fmt.Errorf("device requires /dev paths and rwm permissions")
		}
	}
	for name, u := range v.Ulimits {
		if !Name.MatchString(name) || u.Soft < -1 || u.Hard < -1 || (u.Hard != -1 && (u.Soft == -1 || u.Soft > u.Hard)) {
			return fmt.Errorf("invalid ulimit")
		}
	}
	for _, cap := range append(append([]string{}, v.CapAdd...), v.CapDrop...) {
		if !capabilityPattern.MatchString(cap) {
			return fmt.Errorf("invalid capability name")
		}
	}
	if v.Health.Retries < 0 || v.Health.Retries > 1000000 {
		return fmt.Errorf("invalid healthcheck retries")
	}
	for _, address := range v.DNS {
		if net.ParseIP(address) == nil {
			return fmt.Errorf("DNS server must be an IP address")
		}
	}
	for _, domain := range v.DNSSearch {
		if domain != "." && !validServer(domain) {
			return fmt.Errorf("invalid DNS search domain")
		}
	}
	for _, option := range v.DNSOptions {
		if option == "" || strings.ContainsAny(option, "\x00\r\n ") {
			return fmt.Errorf("invalid DNS option")
		}
	}
	for name, address := range v.ExtraHosts {
		if !validServer(name) || (net.ParseIP(address) == nil && address != "host-gateway") {
			return fmt.Errorf("host mapping requires a hostname and IP address or host-gateway")
		}
	}
	if v.LogDriver != "" && !DriverName.MatchString(v.LogDriver) {
		return fmt.Errorf("invalid logging driver")
	}
	if v.Health.Disabled && (len(v.Health.Command) > 0 || v.Health.Interval != "" || v.Health.Timeout != "" || v.Health.StartPeriod != "" || v.Health.StartInterval != "" || v.Health.Retries != 0) {
		return fmt.Errorf("enable the healthcheck before configuring its parameters")
	}
	for _, value := range []string{v.Health.Interval, v.Health.Timeout, v.Health.StartPeriod, v.Health.StartInterval} {
		if value != "" {
			d, err := time.ParseDuration(value)
			if err != nil || d < time.Millisecond {
				return fmt.Errorf("healthcheck duration must be at least 1ms")
			}
		}
	}
	return nil
}

// ApplyAPI augments the typed create request. The map keys are Engine schema
// fields; user input supplies values only.
func (v ContainerOptions) ApplyAPI(body, host map[string]any) {
	if len(v.Networks) > 0 {
		endpoints := map[string]any{}
		for _, name := range Keys(v.Networks) {
			e := v.Networks[name]
			endpoints[name] = map[string]any{"Aliases": e.Aliases, "IPAMConfig": map[string]string{"IPv4Address": e.IPv4, "IPv6Address": e.IPv6}}
		}
		primary, _ := host["NetworkMode"].(string)
		if primary == "" {
			host["NetworkMode"] = Keys(v.Networks)[0]
		} else if _, ok := endpoints[primary]; !ok {
			endpoints[primary] = map[string]any{}
		}
		body["NetworkingConfig"] = map[string]any{"EndpointsConfig": endpoints}
	}
	if v.GPUCount != 0 || len(v.GPUDevices) > 0 {
		host["DeviceRequests"] = []map[string]any{{"Driver": "nvidia", "Count": v.GPUCount, "DeviceIDs": v.GPUDevices, "Capabilities": [][]string{{"gpu"}}}}
	}
	mounts, _ := host["Mounts"].([]map[string]any)
	for _, target := range Keys(v.Binds) {
		m := v.Binds[target]
		mounts = append(mounts, map[string]any{"Type": "bind", "Source": m.Source, "Target": target, "ReadOnly": m.ReadOnly, "BindOptions": map[string]string{"Propagation": m.Propagation}})
	}
	host["Mounts"] = mounts
	security, _ := host["SecurityOpt"].([]string)
	host["SecurityOpt"] = append(append([]string{}, security...), v.SecurityOptions...)
	if v.PIDMode != "" {
		host["PidMode"] = v.PIDMode
	}
	if v.IPCMode != "" {
		host["IpcMode"] = v.IPCMode
	}
	if v.UTSMode != "" {
		host["UTSMode"] = v.UTSMode
	}
	body["User"], body["Hostname"], body["WorkingDir"] = v.User, v.Hostname, v.WorkingDir
	if v.Entrypoint != nil {
		body["Entrypoint"] = v.Entrypoint
	}
	body["Tty"], body["OpenStdin"] = v.TTY, v.StdinOpen
	if v.StopSignal != "" {
		body["StopSignal"] = v.StopSignal
	}
	if v.StopTimeout != nil {
		body["StopTimeout"] = *v.StopTimeout
	}
	labels := map[string]string{}
	for k, value := range v.Labels {
		labels[k] = value
	}
	labels["com.fractalcr.managed"] = "true"
	body["Labels"] = labels
	host["NanoCpus"], host["CpuShares"], host["CpusetCpus"] = v.NanoCPUs, v.CPUShares, v.CPUSet
	host["MemoryReservation"] = int64(v.MemoryReservationMiB) * 1024 * 1024
	if v.MemorySwapMiB == -1 {
		host["MemorySwap"] = int64(-1)
	} else {
		host["MemorySwap"] = int64(v.MemorySwapMiB) * 1024 * 1024
	}
	if v.ShmMiB > 0 {
		host["ShmSize"] = int64(v.ShmMiB) * 1024 * 1024
	}
	host["Init"], host["Privileged"] = v.Init, v.Privileged
	host["CapAdd"] = v.CapAdd
	if existing, ok := host["CapDrop"].([]string); ok {
		host["CapDrop"] = append(append([]string{}, existing...), v.CapDrop...)
	} else {
		host["CapDrop"] = v.CapDrop
	}
	host["Dns"], host["DnsSearch"], host["DnsOptions"] = v.DNS, v.DNSSearch, v.DNSOptions
	host["Sysctls"], host["Tmpfs"] = v.Sysctls, v.Tmpfs
	if v.LogDriver != "" {
		host["LogConfig"] = map[string]any{"Type": v.LogDriver, "Config": v.LogOptions}
	}
	extra := []string{}
	for _, k := range Keys(v.ExtraHosts) {
		extra = append(extra, k+":"+v.ExtraHosts[k])
	}
	host["ExtraHosts"] = extra
	devices := []map[string]string{}
	for _, k := range Keys(v.Devices) {
		d := v.Devices[k]
		devices = append(devices, map[string]string{"PathOnHost": d.Source, "PathInContainer": k, "CgroupPermissions": d.Permissions})
	}
	host["Devices"] = devices
	limits := []map[string]any{}
	for _, k := range Keys(v.Ulimits) {
		u := v.Ulimits[k]
		limits = append(limits, map[string]any{"Name": k, "Soft": u.Soft, "Hard": u.Hard})
	}
	host["Ulimits"] = limits
	health := map[string]any{}
	if v.Health.Disabled {
		health["Test"] = []string{"NONE"}
	} else if len(v.Health.Command) > 0 {
		health["Test"] = append([]string{"CMD"}, v.Health.Command...)
	}
	for key, value := range map[string]string{"Interval": v.Health.Interval, "Timeout": v.Health.Timeout, "StartPeriod": v.Health.StartPeriod, "StartInterval": v.Health.StartInterval} {
		if value != "" {
			d, _ := time.ParseDuration(value)
			health[key] = int64(d)
		}
	}
	if v.Health.Retries > 0 {
		health["Retries"] = v.Health.Retries
	}
	if len(health) > 0 {
		body["Healthcheck"] = health
	}
}
