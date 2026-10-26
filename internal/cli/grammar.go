package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// This grammar is the authority for parsing, word completion and contextual
// help. Descriptions belong to a token, never to a recursively printed command.
type Command struct {
	Syntax, Description string
	Local               bool
}

var execCommands = []Command{
	{"cd", "Change to the root of the current filesystem", false},
	{"cd <filesystem:>", "Change the default directory", false},
	{"pwd", "Display the current directory", false},
	{"dir", "List files on bootflash", false},
	{"dir <filesystem:>", "Directory or file name", false},
	{"more <filesystem:>", "Display a file", false},
	{"more /ascii <filesystem:>", "Display a file as ASCII text", false},
	{"more /binary <filesystem:>", "Display a file in hex/text format", false},
	{"more /compressed <filesystem:>", "Display a compressed file", false},
	{"more /ebcdic <filesystem:>", "Display an EBCDIC file", false},
	{"show file systems", "Available appliance filesystems", false},
	{"delete <URL>", "Delete a file after confirmation", false},
	{"delete /force <URL>", "Delete a file without confirmation", false},
	{"move <URL> <URL>", "Move a file", false},
	{"rename <URL> <URL>", "Rename a file", false},
	{"mkdir <URL>", "Create a directory", false},
	{"rmdir <URL>", "Remove an empty directory", false},
	{"setup", "Run the physical-console setup dialog", false},
	{"enable", "Enter privileged EXEC", false},
	{"enable <0-15>", "Privilege level", false},
	{"disable", "Return to user EXEC", false},
	{"disable <0-15>", "Privilege level", false},
	{"exit", "Close the EXEC session", false},
	{"logout", "Close the EXEC session", false},
	{"show privilege", "Current privilege level", false},
	{"show users", "Authenticated sessions", false},
	{"show users all", "Display all terminal lines", false},
	{"show users wide", "Display complete user names", false},
	{"show users all wide", "Display all terminal lines in wide format", false},
	{"request container-engine <ARG...>", "Container Engine, Compose, Buildx and Swarm operations", false},
	{"request container-build <ARG...>", "Image builds and build cache operations", false},
	{"terminal length <0-512>", "Set page length; zero disables pagination", true},
	{"terminal width <40-512>", "Set terminal display width", true},
	{"terminal history size <0-256>", "Set command history size", true},
	{"terminal history", "Enable command history", true},
	{"terminal editing", "Enable enhanced command editing", true},
	{"terminal no editing", "Disable enhanced command editing", true},
	{"terminal no history", "Disable command history", true},
	{"terminal no history size", "Restore the default history size", true},
	{"terminal monitor", "Display system messages on this terminal", true},
	{"terminal no monitor", "Disable system messages on this terminal", true},
	{"show version", "Software and platform version", true},
	{"show inventory", "Hardware inventory", true},
	{"show history", "Recent commands in this terminal session", true},
	{"show terminal", "Terminal parameters", false},
	{"show line", "Terminal line summary", false},
	{"show line <0-16>", "Terminal line parameters", false},
	{"show line console 0", "Console line parameters", false},
	{"show line vty <0-15>", "Virtual terminal line parameters", false},
	{"show platform", "Platform status", true},
	{"show platform software service", "Supervised service status", true},
	{"show logging", "System event log", true},
	{"show logging <SERVICE>", "Service or kernel log", true},
	{"request platform software service <SERVICE> start", "Start a service", true},
	{"request platform software service <SERVICE> stop", "Stop a service", true},
	{"request platform software service <SERVICE> restart", "Restart a service", true},
	{"request platform diagnostics containers", "Run container diagnostics", true},
	{"request platform debug service-container enter", "Enter the platform debug shell", true},
	{"request platform debug service-container enable", "Enable the platform debug service", false},
	{"request platform debug service-container disable", "Disable and remove the debug container", false},
	{"reload", "Halt and perform a cold restart", false},
	{"reload cancel", "Cancel a pending reload", false},
	{"reload in <DELAY>", "Delay before reload (minutes or hours:minutes)", false},
	{"reload in <DELAY> <TEXT...>", "Reason for reload", false},
	{"reload at <TIME>", "Time to reload (hh:mm)", false},
	{"reload at <TIME> <TEXT...>", "Date (day month or month day) and reason", false},
	{"reload reason <TEXT...>", "Reason for reload", false},
	{"reload <TEXT...>", "Reason for reload", false},
	{"show reload", "Scheduled reload", false},
	{"configure terminal", "Enter configuration mode", false},
	{"configure candidate", "Stage an explicit configuration transaction", false},
	{"show running-config", "Running configuration", false},
	{"show running-config linenum", "Running configuration with line numbers", false},
	{"show running-config interface <INTERFACE>", "Interface running configuration", false},
	{"show running-config interface <INTERFACE> linenum", "Number the interface configuration lines", false},
	{"show running-config interface <INTERFACE> <INTERFACE-INDEX>", "Interface running configuration", false},
	{"show running-config interface <INTERFACE> <INTERFACE-INDEX> linenum", "Number the interface configuration lines", false},
	{"show running-config view full", "Full running configuration", false},
	{"show running-config json", "Running configuration in interchange format", false},
	{"show candidate-config", "Staged configuration", false},
	{"show startup-config", "Saved startup configuration", false},
	{"show startup-config json", "Saved configuration in interchange format", false},
	{"show configuration diff", "Changes between candidate and running configuration", false},
	{"show interfaces", "Interface addresses and state", false},
	{"show interfaces <INTERFACE>", "Interface addresses, state and counters", false},
	{"show interfaces description", "Interface status and description", false},
	{"show interfaces <INTERFACE> description", "Interface status and description", false},
	{"show ip interface <INTERFACE> brief", "Interface address and protocol status", false},
	{"show ip interface brief", "Interface address summary", false},
	{"show ip route", "IPv4 routing table", false},
	{"show containers", "Container inventory", false},
	{"show container <NAME> detail", "Container configuration and status", false},
	{"show container <NAME> logs", "Recent container logs", false},
	{"show images", "Container images", false},
	{"show clock", "Current system time", false},
	{"show clock detail", "System clock and time source", false},
	{"show calendar", "Hardware calendar", false},
	{"clock set <hh:mm:ss> <1-31> <MONTH> <1993-2035>", "Set the software clock", false},
	{"clock set <hh:mm:ss> <MONTH> <1-31> <1993-2035>", "Set the software clock", false},
	{"clock read-calendar", "Read the hardware calendar into the software clock", false},
	{"clock update-calendar", "Update the hardware calendar from the software clock", false},
	{"calendar set <hh:mm:ss> <1-31> <MONTH> <1993-2035>", "Set the hardware calendar", false},
	{"calendar set <hh:mm:ss> <MONTH> <1-31> <1993-2035>", "Set the hardware calendar", false},
	{"show ntp status", "Time synchronization status", false},
	{"show ntp associations", "Time sources", false},
	{"show networks", "Docker networks", false},
	{"show network <NAME> detail", "Docker network details", false},
	{"request network <NAME> create", "Create a configured Docker network", false},
	{"request network <NAME> remove", "Remove an unused Docker network", false},
	{"request network <NAME> connect <NAME>", "Connect a container", false},
	{"request network <NAME> disconnect <NAME>", "Disconnect a container", false},
	{"show volumes", "Container volumes", false},
	{"show storage", "Datastore status", false},
	{"show storage disks", "Available disks", false},
	{"show storage datastore <NAME>", "Datastore usage", false},
	{"show container-isolation", "Container runtime policy", false},
	{"show container-isolation capabilities", "Runtime and hardware capabilities", false},
	{"show container-isolation detail", "Kata runtime profile", false},
	{"show system resources", "CPU, memory and filesystem resources", false},
	{"copy running-config startup-config", "Save running configuration", false},
	{"copy running-config <URL>", "Copy running configuration", false},
	{"copy startup-config <URL>", "Copy startup configuration", false},
	{"copy <URL> <URL>", "Copy a file between appliance filesystems", false},
	{"copy startup-config candidate-config", "Load startup configuration into the candidate", false},
	{"write memory", "Save running configuration", false},
	{"write terminal", "Display running configuration", false},
	{"request image pull <IMAGE>", "Pull a container image", false},
	{"request container <NAME> create", "Create a configured container", false},
	{"request container <NAME> start", "Start a container", false},
	{"request container <NAME> stop", "Stop a container", false},
	{"request container <NAME> restart", "Restart a container", false},
	{"request container <NAME> remove", "Remove a container", false},
	{"request volume <NAME> create", "Create a named volume", false},
	{"request volume <NAME> remove", "Remove a named volume", false},
	{"request storage datastore <NAME> format confirm <NAME>", "Initialize explicitly selected unused disks", false},
	{"request system container-isolation install runtime kata-containers", "Select the bundled runtime in candidate configuration", false},
	{"request system container-isolation kata-containers vm-factory create", "Prepare the CLH VM factory", false},
	{"ping <IPv4>", "Send IPv4 echo requests", false},
	{"help", "Description of the interactive help system", false},
}

var configCommands = []Command{
	{"username <NAME> privilege <0-15> secret pbkdf2 <SECRET...>", "Import a PBKDF2 secret verifier", false},
	{"username <NAME> secret pbkdf2 <SECRET...>", "Import a PBKDF2 secret verifier", false},
	{"enable secret pbkdf2 <SECRET...>", "Import a PBKDF2 enable verifier", false},
	{"no enable secret", "Remove the enable secret", false},
	{"no enable secret level <1-15>", "Remove the enable secret for this privilege level", false},
	{"enable secret level <1-15> <SECRET...>", "Set the enable secret for this privilege level", false},
	{"enable secret level <1-15> 0 <SECRET...>", "Cleartext input, stored as a salted hash", false},
	{"enable secret level <1-15> pbkdf2 <SECRET...>", "Import a PBKDF2 enable verifier", false},
	{"aaa new-model", "Enable local authentication, authorization and accounting", false},
	{"no aaa new-model", "Disable local AAA and SSH", false},
	{"aaa authentication login default local", "Authenticate using local users", false},
	{"aaa authorization exec default local", "Authorize local EXEC privilege", false},
	{"aaa accounting exec default start-stop local", "Record session events locally", false},
	{"aaa accounting commands 15 default start-stop local", "Record administrative command events locally", false},
	{"username <NAME> privilege <0-15> secret <SECRET...>", "Set a local user and hashed secret", false},
	{"username <NAME> privilege <0-15> secret 0 <SECRET...>", "Cleartext input, stored as a salted hash", false},
	{"username <NAME> privilege <0-15> password <SECRET...>", "Set a local user and hashed password", false},
	{"username <NAME> privilege <0-15> password 0 <SECRET...>", "Cleartext input, stored as a salted hash", false},
	{"username <NAME> secret <SECRET...>", "Set a local user secret", false},
	{"username <NAME> secret 0 <SECRET...>", "Cleartext input, stored as a salted hash", false},
	{"username <NAME> password <SECRET...>", "Set a local user password", false},
	{"username <NAME> password 0 <SECRET...>", "Cleartext input, stored as a salted hash", false},
	{"username <NAME> privilege <0-15>", "Change an existing local user's privilege", false},
	{"username <NAME> privilege <0-15> ssh-key <KEY...>", "Add an SSH public key", false},
	{"no username <NAME> ssh-key <KEY...>", "Remove an SSH public key", false},
	{"no username <NAME>", "Remove a local user", false},
	{"enable secret <SECRET...>", "Set the privileged EXEC secret", false},
	{"enable secret 0 <SECRET...>", "Cleartext input, stored as a salted hash", false},
	{"ip ssh version 2", "Enable SSH version 2", false},
	{"no ip ssh", "Disable SSH access", false},
	{"line console 0", "Configure console login and timeout", false},
	{"line vty <0-15>", "Configure a virtual terminal line", false},
	{"line vty <0-15> <0-15>", "Configure a range of virtual terminal lines", false},
	{"security passwords min-length <0-256>", "Minimum length for newly configured passwords", false},
	{"no security passwords min-length", "Remove the minimum password length policy", false},
	{"hostname <NAME>", "Set the appliance hostname", false},
	{"clock timezone <ZONE> <HOURS>", "Timezone offset from UTC in hours", false},
	{"clock save interval <8-24>", "Hours between saving network time to NVRAM", false},
	{"clock calendar-valid", "Treat the hardware calendar as an authoritative time source", false},
	{"no clock calendar-valid", "Do not treat the hardware calendar as authoritative", false},
	{"default clock calendar-valid", "Do not treat the hardware calendar as authoritative", false},
	{"no clock save interval <8-24>", "Disable saving network time to NVRAM", false},
	{"default clock save interval <8-24>", "Disable saving network time to NVRAM", false},
	{"clock timezone <ZONE> <HOURS> <0-59>", "Additional timezone offset minutes", false},
	{"no clock timezone", "Use UTC", false},
	{"default clock timezone", "Use UTC", false},
	{"clock summer-time <ZONE> recurring", "Use recurring United States summer-time rules", false},
	{"clock summer-time <ZONE> date <1-31> <MONTH> <1993-2035> <TIME> <1-31> <MONTH> <1993-2035> <TIME>", "Configure dated summer-time transitions", false},
	{"clock summer-time <ZONE> date <1-31> <MONTH> <1993-2035> <TIME> <1-31> <MONTH> <1993-2035> <TIME> <1-1440>", "Summer-time offset in minutes", false},
	{"clock summer-time <ZONE> date <MONTH> <1-31> <1993-2035> <TIME> <MONTH> <1-31> <1993-2035> <TIME>", "Configure dated summer-time transitions", false},
	{"clock summer-time <ZONE> date <MONTH> <1-31> <1993-2035> <TIME> <MONTH> <1-31> <1993-2035> <TIME> <1-1440>", "Summer-time offset in minutes", false},
	{"no clock summer-time", "Disable summer-time adjustment", false},
	{"file prompt alert", "Prompt for destructive file operations", false},
	{"file prompt noisy", "Confirm file operation parameters", false},
	{"file prompt quiet", "Suppress file operation confirmations", false},
	{"no file prompt", "Restore default file prompting", false},
	{"file privilege <0-15>", "Set file access privilege level", false},
	{"no file privilege", "Restore default file access privilege", false},
	{"no file privilege <0-15>", "Restore default file access privilege", false},
	{"default file privilege", "Restore default file access privilege", false},
	{"default file prompt", "Restore default file prompting", false},
	{"interface <INTERFACE>", "Configure a physical interface", false},
	{"network <NAME>", "Configure a Docker network", false},
	{"no network <NAME>", "Remove a network definition", false},
	{"ntp server <SERVER>", "Configure an NTP server", false},
	{"no ntp server <SERVER>", "Remove an NTP server", false},
	{"ntp update-calendar", "Periodically update the calendar from NTP", false},
	{"no ntp update-calendar", "Disable periodic calendar updates", false},
	{"default ntp update-calendar", "Disable periodic calendar updates", false},
	{"no interface <INTERFACE>", "Remove interface configuration", false},
	{"container <NAME>", "Configure a container", false},
	{"no container <NAME>", "Remove a container definition", false},
	{"storage datastore name <NAME>", "Configure a datastore", false},
	{"system container-isolation install runtime kata-containers", "Select the bundled Kata runtime", false},
	{"system container-isolation enable", "Enable container isolation", false},
	{"system container-isolation disable", "Disable container isolation", false},
	{"system container-isolation kata-containers default-runtime clh", "Select Cloud Hypervisor", false},
	{"system container-isolation kata-containers runtime clh default-memory <MEMORY>", "Set guest memory", false},
	{"system container-isolation kata-containers runtime clh vm-factory enable", "Enable the CLH snapshot factory", false},
	{"system container-isolation kata-containers runtime clh vm-factory disable", "Disable VM factory", false},
	{"system container-isolation kata-containers apply-to all-containers", "Use Kata by default", false},
	{"system container-isolation kata-containers apply-to explicit", "Use Kata only when selected", false},
	{"system container-isolation kata-containers base-os <GUEST>", "Select a bundled guest OS", false},
	{"commit", "Apply candidate configuration", false},
	{"abort", "Discard candidate configuration and return to EXEC", false},
	{"end", "Return to privileged EXEC", false},
	{"exit", "Leave the current configuration mode", false},
	{"help", "Description of the interactive help system", false},
}

var modeCommands = map[string][]string{
	"line-console":      {"login local", "exec-timeout <0-1440>", "exec-timeout <0-1440> <0-59>"},
	"line-vty":          {"login local", "exec-timeout <0-1440>", "exec-timeout <0-1440> <0-59>", "transport input ssh", "transport input none"},
	"interface":         {"role management", "role container", "shutdown", "no shutdown", "ip address dhcp", "no ip address", "ip address <IPv4/PREFIX>", "ip gateway <IPv4>", "ip name-server <IPv4>", "no ip gateway", "no ip name-server"},
	"storage-datastore": {"file-system format btrfs", "preset single", "preset raid1", "disk <DISK>", "disk add <DISK>"},
	"network":           {"driver bridge", "driver overlay", "driver macvlan", "driver ipvlan", "subnet <IPv4/PREFIX>", "gateway <IPv4>", "parent <INTERFACE>", "internal", "attachable", "no internal", "no attachable", "no subnet", "no gateway", "no parent"},
	"container": {"network <NAME>", "no network", "image <IMAGE>", "command <ARG...>", "environment <VARIABLE> <VALUE>", "publish <PORT-MAPPING>",
		"restart-policy no", "restart-policy always", "restart-policy unless-stopped", "restart-policy on-failure",
		"volume <NAME> <PATH>", "volume <NAME> <PATH> read-only", "memory <MEMORY>", "pids-limit <1-1048576>",
		"read-only", "security no-new-privileges", "capability drop all", "container-isolation inherit",
		"container-isolation kata-containers enable", "no container-isolation", "no volume", "no read-only",
		"no security no-new-privileges", "no capability drop all", "no command", "no environment <VARIABLE>", "no publish"},
}

var tokenHelp = map[string]string{
	"username": "Local user accounts", "privilege": "User privilege level", "password": "Set a password (stored as a salted hash)",
	"secret": "Set a secret (stored as a salted hash)", "<SECRET...>": "Password text", "<0-15>": "Privilege level from 0 to 15",
	"|": "Filter command output", "include": "Include matching lines", "exclude": "Exclude matching lines", "begin": "Begin at the first matching line", "count": "Count matching lines", "<REGEX...>": "POSIX extended regular expression",
	"show": "Display system information", "request": "Perform an operational action", "platform": "Platform services and diagnostics",
	"software": "Platform software", "service": "Supervised services", "debug": "Platform debugging", "diagnostics": "Platform diagnostic tests",
	"ip": "IPv4 information or configuration", "storage": "Datastore and disk management", "datastore": "Persistent datastore",
	"container": "Container configuration and operations", "container-isolation": "Container isolation policy", "system": "System configuration",
	"kata-containers": "Kata Containers runtime", "runtime": "Hypervisor settings", "clh": "Cloud Hypervisor", "vm-factory": "VM factory settings",
	"configuration": "Configuration comparison", "copy": "Copy configuration", "write": "Write persistent configuration", "configure": "Enter configuration mode",
	"calendar": "Set the hardware calendar", "clock": "Set the system clock", "delete": "Delete a file",
	"mkdir": "Create a directory", "more": "Display a file", "move": "Move a file", "ping": "Send IPv4 echo requests",
	"rename": "Rename a file", "rmdir": "Remove an empty directory", "terminal": "Set terminal parameters",
	"no": "Remove a setting or restore its default", "do": "Run an EXEC command", "file-system": "Filesystem settings", "disk": "Whole-disk selection",
	"preset": "Storage redundancy profile", "volume": "Persistent named volume", "environment": "Container environment variable",
	"image": "Container image", "command": "Container command and arguments", "security": "Container security settings", "capability": "Linux capabilities",
	"<NAME>": "Object name (case sensitive)", "<SERVICE>": "Service name or kernel", "<IMAGE>": "Image name, tag or digest",
	"<INTERFACE>": "Network interface name", "<INTERFACE-INDEX>": "Interface number", "<DISK>": "Whole-disk device name", "<MEMORY>": "Memory size, for example 256M",
	"<IPv4>": "IPv4 address", "<IPv4/PREFIX>": "IPv4 address and prefix length", "<VARIABLE>": "Environment variable name",
	"<VALUE>": "Value; quote spaces", "<ARG...>": "Arguments; quotes preserve spaces", "<PATH>": "Absolute path",
	"<PORT-MAPPING>": "HOST:CONTAINER[/tcp|udp]", "<1-1048576>": "Integer from 1 to 1048576", "<GUEST>": "Bundled guest OS name",
}

// Numeric literals and shared argument spellings have different meanings in
// different commands. Help belongs to an edge in the command graph, not to the
// spelling of a token in a global dictionary.
func commandTokenHelp(path []string) string {
	word := path[len(path)-1]
	if path[0] == "do" {
		path = path[1:]
	}
	prefix := strings.Join(path, " ")
	privilegePrefix := strings.TrimPrefix(prefix, "no ")
	if privilegePrefix == "privilege" {
		return "Command privilege parameters"
	}
	if strings.HasPrefix(privilegePrefix, "privilege ") {
		switch word {
		case "exec":
			return "Exec mode"
		case "configure":
			return "Global configuration mode"
		case "interface":
			return "Interface configuration mode"
		case "line":
			return "Line configuration mode"
		case "container":
			return "Container configuration mode"
		case "network":
			return "Container network configuration mode"
		case "storage-datastore":
			return "Datastore configuration mode"
		case "engine":
			return "Container engine configuration mode"
		case "volume":
			return "Volume configuration mode"
		case "all":
			return "All suboptions will be set to the same level"
		case "level":
			return "Set privilege level of command"
		case "reset":
			return "Reset privilege level of command"
		case "<0-15>":
			return "Privilege level"
		}
	}
	if strings.HasPrefix(prefix, "clock summer-time ") {
		switch word {
		case "<ZONE>":
			return "Name of time zone in summer"
		case "<1-4>":
			return "Week number"
		case "first":
			return "First week of the month"
		case "last":
			return "Last week of the month"
		case "<DAY>":
			return "Day of the week"
		case "<MONTH>":
			return "Month of the year"
		case "<TIME>":
			return "Transition time (hh:mm)"
		case "<1-31>":
			return "Day of the month"
		case "<1993-2035>":
			return "Year"
		case "<1-1440>":
			return "Offset to add in minutes"
		}
	}
	switch {
	case word == "/ascii":
		return "Display binary files in ASCII format"
	case word == "/binary":
		return "Display binary files in hex/text format"
	case word == "/compressed":
		return "Display compressed files"
	case word == "/ebcdic":
		return "Display binary files in EBCDIC format"
	case word == "/all":
		return "Include all file entries"
	case word == "/recursive":
		return "Include subdirectories"
	case word == "/force":
		return "Delete without prompting"
	case word == "all-filesystems":
		return "List files in every filesystem"
	case word == "<NETMASK>":
		return "IP subnet mask"
	case word == "<DELAY>":
		return "Delay in minutes or hours:minutes"
	case word == "<TIME>":
		return "Time to reload (hh:mm)"
	case prefix == "line console 0":
		return "Console line number"
	case prefix == "line vty <0-15>":
		return "First virtual terminal line number"
	case prefix == "line vty <0-15> <0-15>":
		return "Last virtual terminal line number"
	case word == "0" && len(path) > 1 && (path[len(path)-2] == "password" || path[len(path)-2] == "secret"):
		return "The following password is cleartext"
	case prefix == "exec-timeout <0-1440>":
		return "Timeout in minutes"
	case prefix == "exec-timeout <0-1440> <0-59>":
		return "Additional timeout in seconds"
	case prefix == "security passwords min-length <0-256>":
		return "Minimum password length; zero disables the policy"
	}
	return tokenHelp[word]
}

type node struct {
	word, description     string
	children              []*node
	terminal, local, rest bool
	defaultLevel, level   int
}

func (n *node) add(c Command) {
	if (strings.HasPrefix(c.Syntax, "show ") || strings.HasPrefix(c.Syntax, "do show ") || strings.HasPrefix(c.Syntax, "more ") || strings.HasPrefix(c.Syntax, "do more ") || c.Syntax == "dir" || strings.HasPrefix(c.Syntax, "dir ") || c.Syntax == "do dir" || strings.HasPrefix(c.Syntax, "do dir ") || c.Syntax == "write terminal" || c.Syntax == "do write terminal") && !strings.Contains(c.Syntax, " | ") {
		for _, kind := range []string{"include", "exclude", "begin", "count", "section", "section include", "section exclude"} {
			n.add(Command{c.Syntax + " | " + kind + " <REGEX...>", "Filter output using a regular expression", c.Local})
		}
		for _, kind := range []string{"redirect", "append", "tee", "tee /append"} {
			n.add(Command{c.Syntax + " | " + kind + " <filesystem:>", "Write command output to a file", c.Local})
		}
	}
	path := strings.Fields(c.Syntax)
	for i, word := range path {
		var next *node
		for _, child := range n.children {
			if child.word == word {
				next = child
				break
			}
		}
		if next == nil {
			next = &node{word: word, description: commandTokenHelp(path[:i+1]), rest: strings.HasSuffix(word, "...>")}
			n.children = append(n.children, next)
		}
		n = next
	}
	n.terminal, n.local = true, c.Local
	if c.Description != "" && n.description == "" {
		n.description = c.Description
	}
}

func (s *Session) rawGrammar() *node {
	root := &node{}
	if s.Mode == "" {
		for _, c := range execCommands {
			if s.Recovery && !c.Local {
				continue
			}
			root.add(c)
		}
		if s.Platform == nil && !s.Recovery {
			root.add(Command{"request system render <PATH>", "Export configuration for offline use", false})
		}
	} else {
		for _, c := range configCommands {
			if strings.Contains(c.Syntax, "clock calendar-valid") && s.HardwareCalendar != nil && !s.HardwareCalendar() {
				continue
			}
			if strings.Contains(c.Syntax, "clock save interval") && s.HardwareCalendar != nil && s.HardwareCalendar() {
				continue
			}
			if s.Mode != "config" && c.Syntax != "exit" && c.Syntax != "end" && c.Syntax != "help" && c.Syntax != "commit" && c.Syntax != "abort" {
				continue
			}
			root.add(c)
		}
		for _, pattern := range modeCommands[s.Mode] {
			root.add(Command{pattern, "", false})
		}
		for _, c := range execCommands {
			if !strings.HasPrefix(c.Syntax, "configure ") {
				c.Syntax = "do " + c.Syntax
				root.add(c)
			}
		}
	}
	return root
}

func (s *Session) grammar() *node {
	root := s.policyGrammar()
	if s.privilegeTarget {
		var targets func(*node)
		targets = func(n *node) {
			n.terminal = true
			for _, c := range n.children {
				targets(c)
			}
		}
		targets(root)
		root.terminal = false
	}
	if s.EnforcePrivilege && s.Privilege < 15 {
		prunePrivileges(root, s.Privilege)
	}
	return root
}

func init() {
	for _, first := range []string{"<1-4>", "first", "last"} {
		for _, last := range []string{"<1-4>", "first", "last"} {
			base := "clock summer-time <ZONE> recurring " + first + " <DAY> <MONTH> <TIME> " + last + " <DAY> <MONTH> <TIME>"
			configCommands = append(configCommands, Command{base, "Configure recurring summer-time transitions", false}, Command{base + " <1-1440>", "Summer-time offset in minutes", false})
		}
	}
	for _, mode := range []string{"line-console", "line-vty"} {
		modeCommands[mode] = append(modeCommands[mode], "no exec-timeout", "default exec-timeout")
		modeCommands[mode] = append(modeCommands[mode], "length <0-512>", "no length", "default length", "editing", "no editing", "default editing", "history", "no history", "default history", "history size <0-256>", "no history size", "default history size")
	}
	// File options are ordered keywords, not a Unix argument pass-through.
	for _, flags := range []string{"", "/all ", "/recursive ", "/all /recursive ", "/recursive /all "} {
		for _, target := range []string{"", "<filesystem:>", "all-filesystems"} {
			execCommands = append(execCommands, Command{strings.TrimSpace("dir " + flags + target), "List files", false})
		}
	}
	for _, flags := range []string{"/recursive ", "/recursive /force ", "/force /recursive "} {
		execCommands = append(execCommands, Command{"delete " + flags + "<URL>", "Delete files and directories", false})
	}
	for _, flags := range []string{"/force", "/recursive", "/recursive /force", "/force /recursive"} {
		execCommands = append(execCommands, Command{"delete <URL> " + flags, "Delete files", false})
	}
}

type SyntaxError struct {
	Kind   string
	Column int
	Input  string
}

func (e *SyntaxError) Error() string {
	switch e.Kind {
	case "incomplete":
		return "Incomplete command."
	case "ambiguous":
		return fmt.Sprintf("Ambiguous command: %q", e.Input)
	default:
		return "Invalid input detected at '^' marker."
	}
}

func parameter(n *node) bool { return strings.HasPrefix(n.word, "<") }

func acceptsArgument(n *node, word string) bool {
	if n.word == "<INTERFACE-INDEX>" {
		if word == "" || word[0] < '0' || word[0] > '9' {
			return false
		}
		for _, c := range word {
			if (c < '0' || c > '9') && c != '/' && c != '.' {
				return false
			}
		}
		return true
	}
	if n.word == "<MONTH>" {
		for _, month := range []string{"january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december"} {
			if len(word) >= 3 && strings.HasPrefix(month, strings.ToLower(word)) {
				return true
			}
		}
		return false
	}
	// Ranges are grammar constraints, so parsing, help traversal and completion
	// all reject an out-of-range value at the same token.
	limits := strings.Split(strings.Trim(n.word, "<>"), "-")
	if len(limits) == 2 {
		lo, a := strconv.Atoi(limits[0])
		hi, b := strconv.Atoi(limits[1])
		if a == nil && b == nil {
			if word == "" {
				return false
			}
			for _, c := range word {
				if c < '0' || c > '9' {
					return false
				}
			}
			value, err := strconv.Atoi(word)
			return err == nil && value >= lo && value <= hi
		}
	}
	return true
}
func (n *node) match(word string) (*node, string) {
	var matches []*node
	var argument *node
	for _, child := range n.children {
		if parameter(child) {
			if acceptsArgument(child, word) {
				argument = child
			}
			continue
		}
		if strings.EqualFold(word, child.word) {
			return child, ""
		}
		if _, err := strconv.Atoi(child.word); err == nil {
			continue
		}
		if strings.HasPrefix(child.word, strings.ToLower(word)) {
			matches = append(matches, child)
		}
	}
	if len(matches) == 1 {
		return matches[0], ""
	}
	if len(matches) > 1 {
		return nil, "ambiguous"
	}
	if argument != nil {
		return argument, ""
	}
	if n.rest {
		return n, ""
	}
	return nil, "invalid"
}

// Resolve canonicalizes only grammar keywords. User values retain their case,
// punctuation and argument boundaries; no command ever passes through a shell.
func (s *Session) Resolve(line string) ([]string, bool, error) {
	words, local, _, err := s.resolveMode(line)
	return words, local, err
}

func (s *Session) resolveMode(line string) ([]string, bool, string, error) {
	words, local, err := s.resolveHere(line)
	if syntax, ok := err.(*SyntaxError); ok && syntax.Kind == "invalid" && s.Mode != "" && s.Mode != "config" {
		parent := *s
		parent.Mode = "config"
		if w, l, e := parent.resolveHere(line); e == nil {
			return w, l, "config", nil
		} else if failure, ok := e.(*SyntaxError); ok && failure.Column > syntax.Column {
			return nil, false, s.Mode, e
		}
	}
	return words, local, s.Mode, err
}

func (s *Session) resolveHere(line string) ([]string, bool, error) {
	words, spans, err := s.lexLine(line)
	if err != nil {
		return nil, false, err
	}
	if len(words) == 0 {
		return nil, false, nil
	}
	// Retain the original appliance's exact shortcut without making "conf t"
	// ambiguous with a second command keyword.
	if len(words) == 1 && strings.EqualFold(words[0], "config") && s.Mode == "" {
		if !s.Allowed([]string{"configure", "terminal"}) {
			return nil, false, &SyntaxError{Kind: "invalid", Input: line, Column: 0}
		}
		return []string{"configure", "terminal"}, false, nil
	}
	n := s.grammar()
	for i, word := range words {
		next, kind := n.match(word)
		if kind != "" {
			return nil, false, &SyntaxError{kind, spans[i], line}
		}
		if !parameter(next) {
			words[i] = next.word
		}
		n = next
	}
	if !n.terminal {
		return nil, false, &SyntaxError{"incomplete", len(line), line}
	}
	words, err = canonicalWorkload(words, line, spans)
	if err == nil {
		if _, _, native := workloadInput(words); native && !s.workloadPolicyAllows(words) {
			return nil, false, &SyntaxError{Kind: "invalid", Input: line, Column: spans[len(spans)-1]}
		}
	}
	if err == nil {
		words, err = s.resolvePrivilege(words, line, spans)
	}
	return words, n.local, err
}

func (s *Session) helpNode(line string) (*node, string, int, bool) {
	words, spans, err := s.lexLine(line)
	if err != nil {
		return nil, "", 0, false
	}
	partial := ""
	start := len(line)
	full := len(line) == 0 || strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t")
	if !full && len(words) > 0 {
		partial = words[len(words)-1]
		start = spans[len(spans)-1]
		words = words[:len(words)-1]
	}
	n := s.grammar()
	for _, word := range words {
		next, kind := n.match(word)
		if kind != "" {
			return nil, "", 0, full
		}
		n = next
	}
	return n, partial, start, full
}

func (s *Session) ContextHelp(line string) string {
	if target, prefix, ok := s.privilegeHelpTarget(line); ok {
		return target.ContextHelp(prefix)
	}
	if help, ok := s.workloadHelp(line); ok && !s.privilegeTarget {
		return help
	}
	n, partial, _, full := s.helpNode(line)
	if n == nil {
		return ""
	}
	items := append([]*node{}, n.children...)
	if n.rest {
		items = append(items, n)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].word < items[j].word })
	var out []string
	for _, child := range items {
		if (child.word == "<filesystem:>" || child.word == "<URL>") && s.FileSystems != nil {
			if full {
				out = append(out, fmt.Sprintf("  %-22s %s", "<filesystem:>", "File system or file name"))
			}
			for _, name := range s.displayFileSystems(line) {
				if full {
					out = append(out, fmt.Sprintf("  %-22s Directory or file name", name))
				} else if strings.HasPrefix(name, partial) {
					out = append(out, name)
				}
			}
			continue
		}
		if child.word == "<INTERFACE>" && s.InterfaceNames != nil {
			for _, name := range s.InterfaceNames() {
				if full {
					out = append(out, fmt.Sprintf("  %-28s Physical interface", name))
				} else if strings.HasPrefix(strings.ToLower(name), strings.ToLower(partial)) {
					out = append(out, name)
				}
			}
			continue
		}
		if !full {
			if !parameter(child) && strings.HasPrefix(child.word, strings.ToLower(partial)) {
				out = append(out, child.word)
			}
		} else {
			description := child.description
			if description == "" {
				description = "Configure " + strings.ReplaceAll(child.word, "-", " ")
			}
			out = append(out, fmt.Sprintf("  %-22s %s", child.word, description))
		}
	}
	if full && n.terminal {
		out = append(out, "  <cr>")
	}
	separator := "\n"
	if !full {
		separator = "  "
	}
	return strings.Join(out, separator)
}

// QueryHelp reports malformed prefixes instead of silently printing an empty
// help response. The partial last word is not an executable command.
func (s *Session) QueryHelp(line string) (string, error) {
	if help := s.ContextHelp(line); help != "" {
		return help, nil
	}
	if s.Mode != "" && s.Mode != "config" && strings.TrimSpace(line) != "" {
		parent := *s
		parent.Mode = "config"
		if help := parent.ContextHelp(line); help != "" {
			return help, nil
		}
	}
	_, _, err := s.Resolve(line)
	if syntax, ok := err.(*SyntaxError); ok && syntax.Kind == "incomplete" {
		return "", nil
	}
	return "", err
}

func (s *Session) Complete(line string) (string, string) {
	completed, feedback := s.completeHere(line)
	if feedback != "" && s.Mode != "" && s.Mode != "config" {
		// Use the whole prefix, not only its first token: an interface's
		// "ip address" and the parent's "ip ssh" share their first keyword.
		// A local ambiguity must still win over a parent completion.
		if _, _, err := s.resolveHere(line); err != nil {
			if syntax, ok := err.(*SyntaxError); ok && syntax.Kind == "invalid" {
				parent := *s
				parent.Mode = "config"
				return parent.completeHere(line)
			}
		}
	}
	return completed, feedback
}

func (s *Session) completeHere(line string) (string, string) {
	if target, prefix, ok := s.privilegeHelpTarget(line); ok {
		completed, feedback := target.completeHere(prefix)
		return line[:len(line)-len(prefix)] + completed, feedback
	}
	if help, ok := s.workloadHelp(line); ok && !s.privilegeTarget {
		if strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t") {
			return line, "\a"
		}
		matches := strings.Fields(help)
		if len(matches) == 1 && !strings.HasPrefix(matches[0], "<") {
			_, spans, err := lex(line)
			if err == nil && len(spans) > 0 {
				return line[:spans[len(spans)-1]] + matches[0] + " ", ""
			}
		}
		return line, "\a"
	}
	n, partial, start, full := s.helpNode(line)
	if n == nil || full {
		return line, "\a"
	}
	var found string
	for _, child := range n.children {
		if (child.word == "<filesystem:>" || child.word == "<URL>") && s.FileSystems != nil {
			for _, name := range s.displayFileSystems(line) {
				if strings.HasPrefix(name, partial) {
					if found != "" {
						return line, "\a"
					}
					found = name
				}
			}
		}
		if child.word == "<INTERFACE>" && s.InterfaceNames != nil {
			if s.InterfaceName != nil {
				if name, err := s.InterfaceName(partial); err == nil {
					return line[:start] + name + " ", ""
				}
			}
			for _, name := range s.InterfaceNames() {
				if strings.HasPrefix(strings.ToLower(name), strings.ToLower(partial)) {
					if found != "" {
						return line, "\a"
					}
					found = name
				}
			}
			if found != "" {
				return line[:start] + found + " ", ""
			}
		}
	}
	for _, child := range n.children {
		if !parameter(child) && strings.EqualFold(child.word, partial) {
			return line[:start] + child.word + " ", ""
		}
	}
	for _, child := range n.children {
		if !parameter(child) && strings.HasPrefix(child.word, strings.ToLower(partial)) {
			if found != "" {
				return line, "\a"
			}
			found = child.word
		}
	}
	if found == "" {
		return line, "\a"
	}
	return line[:start] + found + " ", ""
}

const Help = "Use ? at a word boundary for the next valid keywords or arguments.\nAppend ? to a partial word for matching keywords. The input line is preserved.\nTab completes a unique keyword. Keywords accept unambiguous abbreviations.\nUse Ctrl-V before a literal question mark."
