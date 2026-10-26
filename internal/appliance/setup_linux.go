package appliance

import (
	"fmt"
	"fractalcr/internal/cli"
	"fractalcr/internal/config"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type setupDialog struct {
	stage              string
	base               string
	candidate          config.Config
	nic, user, profile string
	selected           []string
	ports              func() []platformPort
	disks              func() []string
	diskStatus         func() map[string]string
}

func newSetup(c config.Config) *setupDialog {
	return &setupDialog{stage: "begin", candidate: c.Clone(), base: string(c.JSON()), ports: platformPorts,
		disks:      func() []string { return setupDisks("/sys/class/block") },
		diskStatus: func() map[string]string { return setupDiskStatus("/sys/class/block") }}
}

// Setup excludes USB media and disks in use. Partitioned non-USB disks may be
// selected only for an explicitly confirmed erase; the storage service checks
// bootflash identity and mounts again immediately before destructive work.
func setupDiskStatus(root string) map[string]string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	for _, entry := range entries {
		name := entry.Name()
		if !config.Disk.MatchString(name) {
			continue
		}
		path := filepath.Join(root, name)
		physical, err := filepath.EvalSymlinks(path)
		if err != nil {
			result[name] = "cannot identify device"
			continue
		}
		if strings.Contains(filepath.ToSlash(physical), "/usb") {
			result[name] = "USB media (not available for data0)"
			continue
		}
		ro, err := os.ReadFile(filepath.Join(path, "ro"))
		if err != nil {
			result[name] = "cannot inspect read-only state"
			continue
		}
		if strings.TrimSpace(string(ro)) != "0" {
			result[name] = "read-only"
			continue
		}
		holders, err := os.ReadDir(filepath.Join(path, "holders"))
		if err != nil {
			result[name] = "cannot inspect holders"
			continue
		}
		if len(holders) != 0 {
			result[name] = "in use by another block device"
			continue
		}
		children, err := os.ReadDir(path)
		if err != nil {
			result[name] = "cannot inspect partitions"
			continue
		}
		var partitions []string
		for _, child := range children {
			if _, err := os.Stat(filepath.Join(path, child.Name(), "partition")); err == nil {
				partitions = append(partitions, child.Name())
			}
		}
		if _, err := os.Stat(filepath.Join(path, "partition")); !os.IsNotExist(err) {
			result[name] = "not a whole disk"
			continue
		}
		if len(partitions) != 0 {
			result[name] = "requires erase (partitions: " + strings.Join(partitions, ", ") + ")"
		} else {
			result[name] = "candidate"
		}
	}
	return result
}

func setupDisks(root string) []string {
	status := setupDiskStatus(root)
	var result []string
	for name, reason := range status {
		if reason == "candidate" || strings.HasPrefix(reason, "requires erase") {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}
func (s *setupDialog) prompt() reply {
	prompts := map[string]string{
		"begin":    "Enter initial configuration dialog? [yes/no]: ",
		"disk":     "Data disk(s), comma-separated (or 'abort'): ",
		"profile":  "Datastore layout [single/raid1]: ",
		"erase":    "Type ERASE " + strings.Join(s.selected, ",") + " to destroy selected disk contents and initialize data0: ",
		"hostname": "Host name [ctr]: ", "interface": "Management interface: ",
		"address": "IP address/prefix or dhcp [dhcp]: ", "gateway": "Default gateway [none]: ", "dns": "DNS server [none]: ",
		"ntp": "NTP server (address or hostname; blank disables): ", "user": "Administrator username [admin]: ",
		"password": "Administrator password: ", "confirm": "Apply and save this configuration? [yes/no]: ",
	}
	r := reply{Prompt: prompts[s.stage], Challenge: true, Secret: s.stage == "password"}
	if s.stage == "begin" {
		r.Output = "--- System Configuration Dialog ---\nEnter abort at any prompt to leave setup."
	}
	return r
}
func (s *setupDialog) answer(input string, session *cli.Session) reply {
	value := strings.TrimSpace(input)
	if value == "abort" || ((s.stage == "begin" || s.stage == "confirm") && value == "no") {
		return reply{Output: "Setup cancelled. No configuration saved.", Prompt: session.Prompt()}
	}
	fail := func(e error) reply { r := s.prompt(); r.Error = e.Error(); return r }
	switch s.stage {
	case "begin":
		if value != "yes" {
			return fail(fmt.Errorf("enter yes or no"))
		}
		if checkDatastore() != nil {
			s.stage = "disk"
			r := s.prompt()
			available := s.disks()
			r.Output = "Data disks available for initialization (USB media excluded):\n"
			if len(available) == 0 {
				r.Output += "  none\n"
			}
			status := s.diskStatus()
			for _, disk := range available {
				r.Output += "  " + disk + ": " + status[disk] + "\n"
			}
			r.Output += "Existing partitions on selected disks will be destroyed only after explicit confirmation.\n" +
				"single has no redundancy; raid1 mirrors across two or more disks."
			return r
		}
		s.stage = "hostname"
	case "disk":
		choices := make(map[string]bool)
		for _, disk := range s.disks() {
			choices[disk] = true
		}
		selected := strings.Split(value, ",")
		if len(selected) == 0 || len(selected) > 16 {
			return fail(fmt.Errorf("select 1 to 16 listed disks"))
		}
		seen := make(map[string]bool)
		for i, disk := range selected {
			selected[i] = strings.TrimSpace(disk)
			if !choices[selected[i]] || seen[selected[i]] {
				return fail(fmt.Errorf("select distinct data disks from the list"))
			}
			seen[selected[i]] = true
		}
		s.selected = selected
		s.stage = "profile"
		r := s.prompt()
		r.Output = "Selected for data0: " + strings.Join(selected, ", ") + ". All existing data on these disks will be destroyed after confirmation."
		return r
	case "profile":
		if value != "single" && value != "raid1" {
			return fail(fmt.Errorf("enter single or raid1"))
		}
		if value == "raid1" && len(s.selected) < 2 {
			return fail(fmt.Errorf("raid1 requires at least two disks"))
		}
		s.profile = value
		s.stage = "erase"
		r := s.prompt()
		r.Output = "Layout: " + value + ". This operation destroys partitions and filesystems on " + strings.Join(s.selected, ", ") + "."
		return r
	case "erase":
		if value != "ERASE "+strings.Join(s.selected, ",") {
			return fail(fmt.Errorf("enter the exact ERASE confirmation or abort"))
		}
		c := config.Default()
		c.Datastores["data0"] = config.Datastore{Filesystem: "btrfs", Preset: s.profile, Disks: append([]string(nil), s.selected...)}
		if _, e := initializeDatastore("data0", c, true); e != nil {
			return fail(e)
		}
		deadline := time.Now().Add(15 * time.Second)
		for checkDatastore() != nil && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
		}
		if e := checkDatastore(); e != nil {
			return fail(e)
		}
		s.candidate.Datastores = c.Datastores
		s.stage = "hostname"
	case "hostname":
		if value == "" {
			value = "ctr"
		}
		c := s.candidate.Clone()
		c.Hostname = value
		if e := c.Validate(); e != nil {
			return fail(e)
		}
		s.candidate = c
		s.stage = "interface"
		r := s.prompt()
		list := s.ports()
		for _, n := range list {
			if n.Flags&net.FlagLoopback == 0 {
				r.Output += fmt.Sprintf("%-16s %s\n", n.Name, n.HardwareAddr)
			}
		}
		return r
	case "interface":
		selected := ""
		for _, n := range s.ports() {
			if n.Flags&net.FlagLoopback == 0 && (strings.EqualFold(n.Name, value) || n.Interface.Name == value) {
				selected = n.Name
				break
			}
		}
		if selected == "" {
			return fail(fmt.Errorf("select a listed physical interface"))
		}
		for _, n := range s.candidate.AllInterfaces() {
			if n.IsManagement() {
				n.Role = "container"
				n.Gateway = ""
				n.DNS = ""
			}
		}
		value = selected
		target := s.candidate.EditInterface(value)
		*target = config.Network{Interface: value, Role: "management"}
		s.nic = value
		s.stage = "address"
	case "address":
		n := s.candidate.AllInterfaces()[s.nic]
		if value == "" || value == "dhcp" {
			n.DHCP = true
			s.stage = "ntp"
		} else {
			ip, sub, e := net.ParseCIDR(value)
			if e != nil || ip.To4() == nil {
				return fail(fmt.Errorf("use IPv4/prefix or dhcp"))
			}
			ones, _ := sub.Mask.Size()
			if ones < 1 {
				return fail(fmt.Errorf("prefix must be 1..32"))
			}
			n.Address = value
			s.stage = "gateway"
		}
	case "gateway", "dns":
		if value != "" && net.ParseIP(value).To4() == nil {
			return fail(fmt.Errorf("use an IPv4 address or leave blank"))
		}
		n := s.candidate.AllInterfaces()[s.nic]
		if s.stage == "gateway" {
			n.Gateway = value
			s.stage = "dns"
		} else {
			n.DNS = value
			s.stage = "ntp"
		}
	case "ntp":
		c := s.candidate.Clone()
		c.NTP.Servers = nil
		if value != "" {
			c.NTP.Servers = []string{value}
		}
		if e := c.Validate(); e != nil {
			return fail(e)
		}
		s.candidate = c
		s.stage = "user"
	case "user":
		if value == "" {
			value = "admin"
		}
		if !config.Name.MatchString(value) {
			return fail(fmt.Errorf("invalid username"))
		}
		s.user = value
		s.stage = "password"
	case "password":
		hash, e := s.candidate.AAA.HashPassword(input)
		if e != nil {
			return fail(e)
		}
		if s.candidate.AAA.Users == nil {
			s.candidate.AAA.Users = map[string]config.User{}
		}
		s.candidate.AAA.Users[s.user] = config.User{Privilege: 15, Secret: hash}
		s.candidate.AAA.Enabled = true
		s.candidate.AAA.SSH = true
		if e = s.candidate.Validate(); e != nil {
			return fail(e)
		}
		s.stage = "confirm"
		r := s.prompt()
		n := s.candidate.AllInterfaces()[s.nic]
		address := n.Address
		if n.DHCP {
			address = "DHCP"
		}
		r.Output = fmt.Sprintf("Host: %s\nManagement: %s, %s\nGateway: %s\nDNS: %s\nNTP: %s\nAdministrator: %s (privilege 15)\nConsole authentication and SSH: enabled\nDatastore: data0\n", s.candidate.Hostname, s.nic, address, n.Gateway, n.DNS, strings.Join(s.candidate.NTP.Servers, ", "), s.user)
		return r
	case "confirm":
		if value != "yes" {
			return fail(fmt.Errorf("enter yes or no"))
		}
		current, e := loadRunning()
		if e != nil {
			return fail(e)
		}
		if string(current.JSON()) != s.base {
			return fail(fmt.Errorf("running configuration changed during setup; abort and start setup again"))
		}
		if e = Apply(current, s.candidate); e != nil {
			return fail(e)
		}
		s.base = string(s.candidate.JSON())
		if e = Save(s.candidate); e != nil {
			r := fail(e)
			r.Output = "Running configuration applied, but startup configuration could not be saved."
			return r
		}
		session.Running = s.candidate.Clone()
		session.Candidate = s.candidate.Clone()
		s.stage = "complete"
		return reply{Output: "Configuration applied and saved. Authenticate to continue.", Prompt: session.Prompt()}
	}
	return s.prompt()
}
