package appliance

import (
	"encoding/binary"
	"fmt"
	"fractalcr/internal/config"
	"net"
	"strings"
	"syscall"
)

func nestedAttributes(data []byte) map[uint16][]byte {
	result := map[uint16][]byte{}
	for len(data) >= 4 {
		length := int(binary.NativeEndian.Uint16(data))
		if length < 4 || length > len(data) {
			break
		}
		kind := binary.NativeEndian.Uint16(data[2:]) & 0x3fff
		result[kind] = data[4:length]
		aligned := (length + 3) &^ 3
		if aligned > len(data) {
			break
		}
		data = data[aligned:]
	}
	return result
}

func linkIdentity(index int) (kind, alias string, err error) {
	data, err := syscall.NetlinkRIB(syscall.RTM_GETLINK, syscall.AF_UNSPEC)
	if err != nil {
		return "", "", err
	}
	messages, err := syscall.ParseNetlinkMessage(data)
	if err != nil {
		return "", "", err
	}
	for _, message := range messages {
		if message.Header.Type != syscall.RTM_NEWLINK || len(message.Data) < 16 || int(binary.NativeEndian.Uint32(message.Data[4:])) != index {
			continue
		}
		attrs := nestedAttributes(message.Data[16:])
		info := nestedAttributes(attrs[18])
		return strings.TrimRight(string(info[1]), "\x00"), strings.TrimRight(string(attrs[20]), "\x00"), nil
	}
	return "", "", fmt.Errorf("interface disappeared")
}

func removeVirtualInterface(name string) error {
	canonical, kind, kernel, _ := config.VirtualInterface(name)
	if kind == "" {
		return fmt.Errorf("not a virtual interface")
	}
	iface, err := net.InterfaceByName(kernel)
	if err != nil {
		return nil
	}
	actual, alias, err := linkIdentity(iface.Index)
	if err != nil {
		return err
	}
	if actual != kind || alias != "fractalcr:"+canonical {
		return fmt.Errorf("interface name conflicts with an unmanaged link")
	}
	payload := make([]byte, 16)
	binary.NativeEndian.PutUint32(payload[4:], uint32(iface.Index))
	return routeRequest(syscall.RTM_DELLINK, 0, payload)
}

func createVirtualInterface(name string, n *config.Network) error {
	canonical, kind, kernel, id := config.VirtualInterface(name)
	if kind == "" {
		return fmt.Errorf("invalid virtual interface")
	}
	if err := removeVirtualInterface(name); err != nil {
		return err
	}
	payload := make([]byte, 16)
	payload = append(payload, nlAttribute(3, append([]byte(kernel), 0))...)
	payload = append(payload, nlAttribute(20, append([]byte("fractalcr:"+canonical), 0))...)
	data := []byte{}
	if kind == "vlan" {
		parent, err := resolveLink(n.Parent)
		if err != nil {
			return err
		}
		payload = append(payload, nlAttribute(5, nlUint(uint32(parent.Index)))...)
		value := make([]byte, 2)
		binary.NativeEndian.PutUint16(value, uint16(id))
		data = nlAttribute(1, value)
	} else {
		mode := map[string]byte{"balance-rr": 0, "active-backup": 1, "": 1, "balance-xor": 2, "broadcast": 3, "802.3ad": 4, "balance-tlb": 5, "balance-alb": 6}[n.BondMode]
		data = append(nlAttribute(1, []byte{mode}), nlAttribute(3, nlUint(100))...)
	}
	info := append(nlAttribute(1, append([]byte(kind), 0)), nlAttribute(2|0x8000, data)...)
	payload = append(payload, nlAttribute(18|0x8000, info)...)
	if err := routeRequest(syscall.RTM_NEWLINK, syscall.NLM_F_CREATE|syscall.NLM_F_EXCL, payload); err != nil {
		return fmt.Errorf("create %s: %w", canonical, err)
	}
	created, err := net.InterfaceByName(kernel)
	if err != nil {
		return err
	}
	// Some link drivers do not apply the common alias attribute in newlink.
	// Set ownership explicitly on the newly created interface before using it.
	ownership := make([]byte, 16)
	binary.NativeEndian.PutUint32(ownership[4:], uint32(created.Index))
	ownership = append(ownership, nlAttribute(20, append([]byte("fractalcr:"+canonical), 0))...)
	if err = routeRequest(syscall.RTM_NEWLINK, 0, ownership); err != nil {
		rollback := make([]byte, 16)
		binary.NativeEndian.PutUint32(rollback[4:], uint32(created.Index))
		_ = routeRequest(syscall.RTM_DELLINK, 0, rollback)
		return err
	}
	if kind == "bond" {
		bond, err := net.InterfaceByName(kernel)
		if err != nil {
			return err
		}
		for _, member := range n.Members {
			iface, err := resolveLink(member)
			if err != nil {
				return err
			}
			wasUp := iface.Flags&net.FlagUp != 0
			if err = setInterfaceState(member, false); err != nil {
				return err
			}
			request := make([]byte, 16)
			binary.NativeEndian.PutUint32(request[4:], uint32(iface.Index))
			request = append(request, nlAttribute(10, nlUint(uint32(bond.Index)))...)
			if err = routeRequest(syscall.RTM_NEWLINK, 0, request); err != nil {
				_ = setInterfaceState(member, wasUp)
				return fmt.Errorf("attach %s to %s: %w", member, name, err)
			}
			if err = setInterfaceState(member, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureVirtualInterfaces(c config.Config) error {
	created := []string{}
	for _, name := range c.InterfaceOrder() {
		n := c.AllInterfaces()[name]
		_, kind, _, _ := config.VirtualInterface(name)
		if kind == "" || kind == "vlan" && n.Parent == "" {
			continue
		}
		if err := createVirtualInterface(name, n); err != nil {
			for i := len(created) - 1; i >= 0; i-- {
				_ = removeVirtualInterface(created[i])
			}
			_ = removeVirtualInterface(name)
			return err
		}
		created = append(created, name)
	}
	return nil
}

func removeVirtualInterfaces(c config.Config) {
	names := c.InterfaceOrder()
	for i := len(names) - 1; i >= 0; i-- {
		if _, kind, _, _ := config.VirtualInterface(names[i]); kind != "" {
			_ = removeVirtualInterface(names[i])
		}
	}
}

func virtualDisplayPorts() []platformPort {
	ports := []platformPort{}
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		name := ""
		if strings.HasPrefix(iface.Name, "fcrvl") {
			name = "Vlan" + strings.TrimPrefix(iface.Name, "fcrvl")
		} else if strings.HasPrefix(iface.Name, "fcrbond") {
			name = "Port-channel" + strings.TrimPrefix(iface.Name, "fcrbond")
		}
		canonical, kind, _, _ := config.VirtualInterface(name)
		if canonical == "" {
			continue
		}
		actual, alias, err := linkIdentity(iface.Index)
		if err == nil && actual == kind && alias == "fractalcr:"+canonical {
			ports = append(ports, platformPort{Interface: iface, Name: canonical, Identity: "virtual:" + canonical})
		}
	}
	return ports
}
