package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Engine struct {
	StorageBackend     string            `json:"storage_backend,omitempty"`
	RegistryMirrors    []string          `json:"registry_mirrors,omitempty"`
	InsecureRegistries []string          `json:"insecure_registries,omitempty"`
	AddressPools       map[string]int    `json:"address_pools,omitempty"`
	LogDriver          string            `json:"log_driver,omitempty"`
	LogOptions         map[string]string `json:"log_options,omitempty"`
	LiveRestore        bool              `json:"live_restore,omitempty"`
	Debug              bool              `json:"debug,omitempty"`
	IPv6               bool              `json:"ipv6,omitempty"`
	FixedCIDRv6        string            `json:"fixed_cidr_v6,omitempty"`
	BridgeAddress      string            `json:"bridge_address,omitempty"`
	MTU                int               `json:"mtu,omitempty"`
	MaxDownloads       int               `json:"max_downloads,omitempty"`
	MaxUploads         int               `json:"max_uploads,omitempty"`
	ShutdownTimeout    *int              `json:"shutdown_timeout,omitempty"`
	DNS                []string          `json:"dns,omitempty"`
	DNSSearch          []string          `json:"dns_search,omitempty"`
	DNSOptions         []string          `json:"dns_options,omitempty"`
	Labels             map[string]string `json:"labels,omitempty"`
}

func (e Engine) Validate() error {
	if e.StorageBackend != "" && e.StorageBackend != "overlay2" && e.StorageBackend != "blockfile" {
		return fmt.Errorf("storage backend must be overlay2 or blockfile")
	}
	for _, mirror := range e.RegistryMirrors {
		u, err := url.Parse(mirror)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return fmt.Errorf("registry mirror requires an HTTP or HTTPS origin without credentials")
		}
	}
	for _, registry := range e.InsecureRegistries {
		if _, _, err := net.ParseCIDR(registry); err == nil {
			continue
		}
		host := registry
		if h, p, err := net.SplitHostPort(registry); err == nil {
			host = h
			n, err := strconv.Atoi(p)
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("invalid registry port")
			}
		}
		if !validServer(host) {
			return fmt.Errorf("invalid registry host or network")
		}
	}
	for base, size := range e.AddressPools {
		ip, n, err := net.ParseCIDR(base)
		if err != nil || ip.To4() == nil || !ip.Equal(n.IP) {
			return fmt.Errorf("address pool must be an IPv4 network")
		}
		prefix, _ := n.Mask.Size()
		if size < prefix || size > 30 {
			return fmt.Errorf("pool subnet size must be within its network and at most 30")
		}
	}
	if e.LogDriver != "" && !DriverName.MatchString(e.LogDriver) {
		return fmt.Errorf("invalid logging driver")
	}
	for _, m := range []map[string]string{e.LogOptions, e.Labels} {
		for k, v := range m {
			if k == "" || strings.ContainsAny(k, "\x00\r\n=") || strings.ContainsAny(v, "\x00\r\n") {
				return fmt.Errorf("invalid engine option")
			}
		}
	}
	if e.MTU != 0 && (e.MTU < 576 || e.MTU > 65535) {
		return fmt.Errorf("engine MTU must be 576..65535")
	}
	if e.MaxDownloads < 0 || e.MaxDownloads > 1024 || e.MaxUploads < 0 || e.MaxUploads > 1024 {
		return fmt.Errorf("transfer concurrency must be 1..1024")
	}
	if e.ShutdownTimeout != nil && (*e.ShutdownTimeout < 0 || *e.ShutdownTimeout > 86400) {
		return fmt.Errorf("shutdown timeout must be 0..86400")
	}
	if e.BridgeAddress != "" {
		ip, _, err := net.ParseCIDR(e.BridgeAddress)
		if err != nil || ip.To4() == nil {
			return fmt.Errorf("bridge address must be IPv4/prefix")
		}
	}
	if e.FixedCIDRv6 != "" {
		ip, n, err := net.ParseCIDR(e.FixedCIDRv6)
		if err != nil || ip.To4() != nil || !ip.Equal(n.IP) {
			return fmt.Errorf("fixed IPv6 range must be an IPv6 network")
		}
	}
	return (ContainerOptions{DNS: e.DNS, DNSSearch: e.DNSSearch, DNSOptions: e.DNSOptions}).Validate()
}

func (e Engine) ApplyDaemon(d map[string]any) {
	if e.StorageBackend == "blockfile" {
		d["storage-driver"] = "blockfile"
		d["features"] = map[string]bool{"containerd-snapshotter": true}
		d["containerd"] = "/run/fractal-containerd/containerd.sock"
	}
	if len(e.RegistryMirrors) > 0 {
		d["registry-mirrors"] = e.RegistryMirrors
	}
	if len(e.InsecureRegistries) > 0 {
		d["insecure-registries"] = e.InsecureRegistries
	}
	if len(e.AddressPools) > 0 {
		pools := []map[string]any{}
		for _, base := range Keys(e.AddressPools) {
			pools = append(pools, map[string]any{"base": base, "size": e.AddressPools[base]})
		}
		d["default-address-pools"] = pools
	}
	if e.LogDriver != "" {
		d["log-driver"] = e.LogDriver
	}
	if len(e.LogOptions) > 0 {
		d["log-opts"] = e.LogOptions
	}
	if e.LiveRestore {
		d["live-restore"] = true
	}
	if e.Debug {
		d["debug"] = true
	}
	if e.IPv6 {
		d["ipv6"] = true
	}
	if e.FixedCIDRv6 != "" {
		d["fixed-cidr-v6"] = e.FixedCIDRv6
	}
	if e.BridgeAddress != "" {
		d["bip"] = e.BridgeAddress
	}
	if e.MTU != 0 {
		d["mtu"] = e.MTU
	}
	if e.MaxDownloads != 0 {
		d["max-concurrent-downloads"] = e.MaxDownloads
	}
	if e.MaxUploads != 0 {
		d["max-concurrent-uploads"] = e.MaxUploads
	}
	if e.ShutdownTimeout != nil {
		d["shutdown-timeout"] = *e.ShutdownTimeout
	}
	if len(e.DNS) > 0 {
		d["dns"] = e.DNS
	}
	if len(e.DNSSearch) > 0 {
		d["dns-search"] = e.DNSSearch
	}
	if len(e.DNSOptions) > 0 {
		d["dns-opts"] = e.DNSOptions
	}
	if len(e.Labels) > 0 {
		labels := []string{}
		for _, k := range Keys(e.Labels) {
			labels = append(labels, k+"="+e.Labels[k])
		}
		d["labels"] = labels
	}
}

func (e Engine) RenderCLI(line func(string, ...any)) {
	// Always render the mode so a saved config is replayable even with defaults.
	line("!\nsystem container-engine")
	if e.StorageBackend != "" {
		line(" storage backend %s", e.StorageBackend)
	}
	for _, item := range []struct {
		key    string
		values []string
	}{{"registry mirror", e.RegistryMirrors}, {"registry insecure", e.InsecureRegistries}, {"dns server", e.DNS}, {"dns search", e.DNSSearch}, {"dns option", e.DNSOptions}} {
		for _, v := range item.values {
			line(" %s %s", item.key, strconv.Quote(v))
		}
	}
	for _, base := range Keys(e.AddressPools) {
		line(" address-pool %s size %d", base, e.AddressPools[base])
	}
	for _, item := range []struct{ key, value string }{{"logging driver", e.LogDriver}, {"bridge address", e.BridgeAddress}, {"ipv6 fixed-cidr", e.FixedCIDRv6}} {
		if item.value != "" {
			line(" %s %s", item.key, strconv.Quote(item.value))
		}
	}
	for _, item := range []struct {
		key   string
		value bool
	}{{"live-restore", e.LiveRestore}, {"debug", e.Debug}, {"ipv6 enable", e.IPv6}} {
		if item.value {
			line(" %s", item.key)
		}
	}
	for _, item := range []struct {
		key   string
		value int
	}{{"mtu", e.MTU}, {"max-concurrent-downloads", e.MaxDownloads}, {"max-concurrent-uploads", e.MaxUploads}} {
		if item.value != 0 {
			line(" %s %d", item.key, item.value)
		}
	}
	if e.ShutdownTimeout != nil {
		line(" shutdown-timeout %d", *e.ShutdownTimeout)
	}
	for _, item := range []struct {
		key    string
		values map[string]string
	}{{"logging option", e.LogOptions}, {"label", e.Labels}} {
		for _, k := range Keys(item.values) {
			line(" %s %s %s", item.key, strconv.Quote(k), strconv.Quote(item.values[k]))
		}
	}
	line(" exit")
}
