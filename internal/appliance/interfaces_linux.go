package appliance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"fractalcr/internal/config"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type portIdentity struct {
	Name     string `json:"name"`
	Identity string `json:"identity"`
}
type platformPort struct {
	net.Interface
	Name, Identity string
	Speed          int64
}

var portMutex sync.Mutex

func speedClass(speed int64) string {
	switch {
	case speed <= 0:
		return "Ethernet"
	case speed <= 100:
		return "FastEthernet"
	case speed <= 1000:
		return "GigabitEthernet"
	case speed <= 2500:
		return "TwoGigabitEthernet"
	case speed <= 5000:
		return "FiveGigabitEthernet"
	case speed <= 10000:
		return "TenGigabitEthernet"
	case speed <= 25000:
		return "TwentyFiveGigabitEthernet"
	case speed <= 40000:
		return "FortyGigabitEthernet"
	case speed <= 50000:
		return "FiftyGigabitEthernet"
	case speed <= 100000:
		return "HundredGigabitEthernet"
	case speed <= 200000:
		return "TwoHundredGigabitEthernet"
	default:
		return "FourHundredGigabitEthernet"
	}
}
func platformPorts() []platformPort {
	portMutex.Lock()
	defer portMutex.Unlock()
	if lock, e := os.OpenFile("/run/interface-names.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600); e == nil {
		defer lock.Close()
		if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX) == nil {
			defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		}
	}
	list, _ := net.Interfaces()
	ports := []platformPort{}
	for _, n := range list {
		device, err := filepath.EvalSymlinks("/sys/class/net/" + n.Name + "/device")
		if err != nil || n.Flags&net.FlagLoopback != 0 {
			continue
		}
		speedData, _ := os.ReadFile("/sys/class/net/" + n.Name + "/speed")
		speed, _ := strconv.ParseInt(strings.TrimSpace(string(speedData)), 10, 64)
		// Physical bus location survives link negotiation, interface renaming and
		// configured MAC changes. The saved name never changes when speed changes.
		ports = append(ports, platformPort{Interface: n, Identity: device, Speed: speed})
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i].Identity < ports[j].Identity })
	var saved []portIdentity
	persistent := "/datastore/data0/config/interface-names.json"
	data, err := os.ReadFile(persistent)
	if err != nil {
		data, _ = os.ReadFile("/run/interface-names.json")
	}
	_ = json.Unmarshal(data, &saved)
	used := map[string]bool{}
	names := map[string]string{}
	for _, p := range saved {
		if p.Name != "" && p.Identity != "" && !used[p.Name] {
			used[p.Name] = true
			names[p.Identity] = p.Name
		}
	}
	for i := range ports {
		p := &ports[i]
		p.Name = names[p.Identity]
		if p.Name != "" {
			continue
		}
		prefix := speedClass(p.Speed)
		for index := 0; ; index++ {
			name := fmt.Sprintf("%s%d", prefix, index)
			if !used[name] {
				p.Name = name
				used[name] = true
				break
			}
		}
		saved = append(saved, portIdentity{p.Name, p.Identity})
	}
	data, _ = json.Marshal(saved)
	writePortIdentities("/run/interface-names.json", data)
	if dataOnline() == nil {
		writePortIdentities(persistent, data)
	}
	return ports
}

// Interface discovery runs on help, status and configuration paths. Do not
// turn every read into a durable datastore transaction.
func writePortIdentities(path string, data []byte) {
	if current, err := os.ReadFile(path); err == nil && bytes.Equal(current, data) {
		return
	}
	_ = config.AtomicWrite(path, data, 0600)
}
func kernelInterface(name string) (string, error) {
	if _, _, kernel, _ := config.VirtualInterface(name); kernel != "" {
		return kernel, nil
	}
	for _, p := range platformPorts() {
		if strings.EqualFold(p.Name, name) || p.Interface.Name == name {
			return p.Interface.Name, nil
		}
	}
	return "", fmt.Errorf("Interface %s is unavailable", name)
}
func canonicalInterface(name string) (string, error) {
	if canonical, _, _, _ := config.VirtualInterface(name); canonical != "" {
		return canonical, nil
	}
	ports := platformPorts()
	for _, p := range ports {
		if strings.EqualFold(p.Name, name) || p.Interface.Name == name {
			return p.Name, nil
		}
	}
	matches := []string{}
	for _, p := range ports { // IOS accepts an unambiguous type abbreviation followed by the full index.
		cut := strings.IndexAny(name, "0123456789")
		full := strings.IndexAny(p.Name, "0123456789")
		if cut > 0 && full > 0 && strings.HasPrefix(strings.ToLower(p.Name[:full]), strings.ToLower(name[:cut])) && p.Name[full:] == name[cut:] {
			matches = append(matches, p.Name)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("Ambiguous interface name")
	}
	return "", fmt.Errorf("Interface %s is unavailable", name)
}

func interfaceNames() []string {
	names := []string{}
	for _, p := range append(platformPorts(), virtualDisplayPorts()...) {
		names = append(names, p.Name)
	}
	return names
}
func applianceInterface(name string) string {
	for _, p := range append(platformPorts(), virtualDisplayPorts()...) {
		if p.Interface.Name == name {
			return p.Name
		}
	}
	if name == "lo" {
		return "Loopback0"
	}
	if name == "docker0" {
		return "ContainerBridge0"
	}
	return "ContainerNetwork"
}
func interfaceByName(name string) (*net.Interface, error) {
	kernel, err := kernelInterface(name)
	if err != nil {
		return nil, err
	}
	return net.InterfaceByName(kernel)
}
func migrateInterfaces(c *config.Config) {
	if c.Network != nil {
		if name, err := canonicalInterface(c.Network.Interface); err == nil {
			c.Network.Interface = name
		}
	}
	revised := map[string]*config.Network{}
	for key, n := range c.Interfaces {
		if n == nil {
			revised[key] = n
			continue
		}
		if name, err := canonicalInterface(key); err == nil {
			key = name
			n.Interface = name
		}
		revised[key] = n
	}
	c.Interfaces = revised
	for _, n := range c.AllInterfaces() {
		if n == nil {
			continue
		}
		if n.Parent != "" {
			if name, err := canonicalInterface(n.Parent); err == nil {
				n.Parent = name
			}
		}
		for i, member := range n.Members {
			if name, err := canonicalInterface(member); err == nil {
				n.Members[i] = name
			}
		}
	}
	for key, n := range c.Networks {
		if n.Parent != "" {
			if name, err := canonicalInterface(n.Parent); err == nil {
				n.Parent = name
				c.Networks[key] = n
			}
		}
	}
}
func showInterfaces(brief bool, c config.Config) (string, error) {
	var b strings.Builder
	if brief {
		b.WriteString("Interface                       IP-Address      OK? Method Status                Protocol\n")
	}
	for _, p := range append(platformPorts(), virtualDisplayPorts()...) {
		address := "unassigned"
		addresses, _ := p.Addrs()
		for _, a := range addresses {
			ip, _, err := net.ParseCIDR(a.String())
			if err == nil && ip.To4() != nil {
				address = ip.String()
				break
			}
		}
		state, protocol := "down", "down"
		if p.Flags&net.FlagUp != 0 {
			state = "up"
		}
		if p.Flags&net.FlagRunning != 0 {
			protocol = "up"
		}
		method := "unset"
		n := c.AllInterfaces()[p.Name]
		if n == nil {
			n = c.AllInterfaces()[p.Interface.Name]
		}
		if n != nil {
			if n.Shutdown {
				state = "administratively down"
			}
			if n.DHCP {
				method = "DHCP"
			} else if n.Address != "" {
				method = "manual"
			}
		}
		if brief {
			fmt.Fprintf(&b, "%-31s %-15s YES %-6s %-21s %s\n", p.Name, address, method, state, protocol)
			continue
		}
		mac := strings.ReplaceAll(p.HardwareAddr.String(), ":", "")
		if len(mac) == 12 {
			mac = mac[:4] + "." + mac[4:8] + "." + mac[8:]
		}
		fmt.Fprintf(&b, "%s is %s, line protocol is %s\n  Hardware is Ethernet, address is %s\n  Internet address is %s\n  MTU %d bytes", p.Name, state, protocol, mac, address, p.MTU)
		if p.Speed > 0 {
			fmt.Fprintf(&b, ", BW %d Kbit/sec", p.Speed*1000)
		}
		b.WriteString("\n")
		if n != nil && n.Description != "" {
			fmt.Fprintf(&b, "  Description: %s\n", displayText(n.Description))
		}
		for _, a := range addresses {
			if strings.Contains(a.String(), ":") {
				fmt.Fprintf(&b, "  IPv6 address is %s\n", a.String())
			}
		}
		counters := map[string]uint64{}
		for _, key := range []string{"rx_packets", "tx_packets", "rx_bytes", "tx_bytes", "rx_errors", "tx_errors", "rx_dropped", "tx_dropped"} {
			data, _ := os.ReadFile("/sys/class/net/" + p.Interface.Name + "/statistics/" + key)
			counters[key], _ = strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
		}
		fmt.Fprintf(&b, "  %d packets input, %d bytes, %d input errors, %d drops\n  %d packets output, %d bytes, %d output errors, %d drops\n", counters["rx_packets"], counters["rx_bytes"], counters["rx_errors"], counters["rx_dropped"], counters["tx_packets"], counters["tx_bytes"], counters["tx_errors"], counters["tx_dropped"])
	}
	return b.String(), nil
}
