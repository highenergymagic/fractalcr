package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func IPv4CLI(address string) string {
	ip, network, err := net.ParseCIDR(address)
	if err != nil || ip.To4() == nil {
		return address
	}
	return ip.String() + " " + net.IP(network.Mask).String()
}

// CLI renders the configuration model, never a host tool's output. JSON remains
// the durable storage and machine interchange format.
func (c Config) CLI() string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("!\nhostname %s", c.Hostname)
	if clock := c.Clock.CLI(); clock != "" {
		line("%s", clock)
	}
	if c.Clock.Summer != nil {
		line("%s", c.Clock.Summer.CLI())
	}
	if c.Clock.SaveIntervalHours != 0 {
		line("clock save interval %d", c.Clock.SaveIntervalHours)
	}
	if c.FilePrompt != "" && c.FilePrompt != "alert" {
		line("file prompt %s", c.FilePrompt)
	}
	c.Engine.RenderCLI(line)
	if c.AAA.FilePrivilege != nil && *c.AAA.FilePrivilege != 15 {
		line("file privilege %d", *c.AAA.FilePrivilege)
	}
	for _, name := range Keys(c.Volumes) {
		c.Volumes[name].RenderCLI(name, line)
	}
	if c.AAA.PasswordMinLength > 0 {
		line("security passwords min-length %d", c.AAA.PasswordMinLength)
	}
	for _, name := range Keys(c.AAA.Users) {
		u := c.AAA.Users[name]
		if u.Secret != "" {
			line("username %s privilege %d secret %s", name, u.Privilege, SecretCLI(u.Secret))
		}
		for _, key := range u.SSHKeys {
			line("username %s privilege %d ssh-key %s", name, u.Privilege, key)
		}
	}
	if c.AAA.EnableSecret != "" {
		line("enable secret %s", SecretCLI(c.AAA.EnableSecret))
	}
	for level := 1; level < 15; level++ {
		if secret := c.AAA.EnableSecrets[level]; secret != "" {
			line("enable secret level %d %s", level, SecretCLI(secret))
		}
	}
	if c.AAA.Enabled {
		line("aaa new-model\naaa authentication login default local\naaa authorization exec default local")
	}
	for _, privilege := range c.AAA.PrivilegeCLI() {
		line("%s", privilege)
	}
	if c.AAA.SSHAvailable() {
		line("ip ssh version 2")
	}
	for _, name := range c.InterfaceOrder() {
		n := c.AllInterfaces()[name]
		line("!\ninterface %s", name)
		role := n.Role
		if role == "" {
			role = "management"
		}
		line(" role %s", role)
		if _, kind, _, _ := VirtualInterface(name); kind == "bond" {
			n.RenderOptionsCLI(line)
			n.RenderVirtualCLI(line)
		} else {
			n.RenderVirtualCLI(line)
			n.RenderOptionsCLI(line)
		}
		if n.DHCP {
			line(" ip address dhcp")
		} else if n.Address != "" {
			line(" ip address %s", IPv4CLI(n.Address))
		}
		if n.Gateway != "" {
			line(" ip gateway %s", n.Gateway)
		}
		if n.DNS != "" {
			line(" ip name-server %s", n.DNS)
		}
		if n.Shutdown {
			line(" shutdown")
		} else {
			line(" no shutdown")
		}
	}
	for _, server := range c.NTP.Servers {
		line("ntp server %s", server)
	}
	if c.NTP.UpdateCalendar {
		line("ntp update-calendar")
	}
	for _, name := range Keys(c.AllInterfaces()) {
		c.AllInterfaces()[name].RenderRoutesCLI(line)
	}
	for _, name := range Keys(c.Datastores) {
		d := c.Datastores[name]
		line("!\nstorage datastore name %s\n file-system format %s\n preset %s", name, d.Filesystem, d.Preset)
		for i, disk := range d.Disks {
			if i == 0 {
				line(" disk %s", disk)
			} else {
				line(" disk add %s", disk)
			}
		}
		line(" exit")
	}
	if c.Isolation.Installed {
		line("system container-isolation install runtime kata-containers")
	}
	if c.Isolation.Enabled {
		line("system container-isolation enable")
	} else {
		line("system container-isolation disable")
	}
	line("system container-isolation kata-containers default-runtime %s", c.Isolation.Runtime)
	line("system container-isolation kata-containers runtime clh default-memory %dM", c.Isolation.MemoryMiB)
	line("system container-isolation kata-containers apply-to %s", c.Isolation.ApplyTo)
	line("system container-isolation kata-containers base-os %s", c.Isolation.BaseOS)
	for _, name := range Keys(c.Networks) {
		n := c.Networks[name]
		line("!\nnetwork %s\n driver %s", name, n.Driver)
		n.RenderOptionsCLI(line)
		if n.Subnet != "" {
			line(" subnet %s", n.Subnet)
		}
		if n.Gateway != "" {
			line(" gateway %s", n.Gateway)
		}
		if n.Parent != "" {
			line(" parent %s", n.Parent)
		}
		if n.Internal {
			line(" internal")
		}
		if n.Attachable {
			line(" attachable")
		}
		line(" exit")
	}
	for _, name := range Keys(c.Containers) {
		v := c.Containers[name]
		line("!\ncontainer %s", name)
		v.ContainerOptions.RenderCLI(line)
		if v.Image != "" {
			line(" image %s", v.Image)
		}
		if v.Network != "" {
			line(" network %s", v.Network)
		}
		if v.Restart != "" {
			line(" restart-policy %s", v.Restart)
		}
		if len(v.Command) > 0 {
			parts := []string{}
			for _, arg := range v.Command {
				parts = append(parts, strconv.Quote(arg))
			}
			line(" command %s", strings.Join(parts, " "))
		}
		for _, key := range Keys(v.Environment) {
			line(" environment %s %s", key, strconv.Quote(v.Environment[key]))
		}
		for _, p := range v.Publish {
			line(" publish %s", p)
		}
		for _, m := range v.Volumes {
			suffix := ""
			if m.ReadOnly {
				suffix = " read-only"
			}
			line(" volume %s %s%s", m.Name, strconv.Quote(m.Target), suffix)
		}
		if v.MemoryMiB > 0 {
			line(" memory %dM", v.MemoryMiB)
		}
		if v.Pids > 0 {
			line(" pids-limit %d", v.Pids)
		}
		if v.ReadOnly {
			line(" read-only")
		}
		if v.NoNewPrivileges {
			line(" security no-new-privileges")
		}
		if v.DropAllCapabilities {
			line(" capability drop all")
		}
		switch v.Isolation {
		case "none":
			line(" no container-isolation")
		case "kata":
			line(" container-isolation kata-containers enable")
		default:
			line(" container-isolation inherit")
		}
		line(" exit")
	}
	line("!\nline console 0\n exec-timeout %d %d", c.AAA.ConsoleTimeout/60, c.AAA.ConsoleTimeout%60)
	if c.AAA.Enabled {
		line(" login local")
	}
	for first := 0; first < VTYCount; {
		policy := c.AAA.Line(first)
		last := first
		for last+1 < VTYCount && c.AAA.Line(last+1) == policy {
			last++
		}
		line("!\nline vty %d %d\n exec-timeout %d %d", first, last, policy.Timeout/60, policy.Timeout%60)
		if c.AAA.Enabled {
			line(" login local")
		}
		transport := "none"
		if policy.SSH {
			transport = "ssh"
		}
		line(" transport input %s", transport)
		first = last + 1
	}
	line("!\nend")
	return b.String()
}

func ValidateSecret(encoded string) error {
	if strings.HasPrefix(encoded, "$1$") {
		_, err := md5SecretSalt(encoded)
		return err
	}
	if strings.HasPrefix(encoded, "$8$") || strings.HasPrefix(encoded, "$9$") {
		_, _, _, err := iosSecretParts(encoded)
		return err
	}
	_, _, err := secretParts(encoded)
	return err
}
