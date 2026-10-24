package appliance

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"syscall"
)

var resolveLink = interfaceByName

func nlAttribute(kind uint16, data []byte) []byte {
	size := len(data) + 4
	b := make([]byte, (size+3)&^3)
	binary.NativeEndian.PutUint16(b, uint16(size))
	binary.NativeEndian.PutUint16(b[2:], kind)
	copy(b[4:], data)
	return b
}
func nlUint(value uint32) []byte {
	b := make([]byte, 4)
	binary.NativeEndian.PutUint32(b, value)
	return b
}
func routeRequest(kind, flags uint16, payload []byte) error {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC, syscall.NETLINK_ROUTE)
	if err != nil {
		return fmt.Errorf("Network service is unavailable")
	}
	defer syscall.Close(fd)
	_ = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &syscall.Timeval{Sec: 3})
	packet := make([]byte, 16+len(payload))
	binary.NativeEndian.PutUint32(packet, uint32(len(packet)))
	binary.NativeEndian.PutUint16(packet[4:], kind)
	binary.NativeEndian.PutUint16(packet[6:], flags|syscall.NLM_F_REQUEST|syscall.NLM_F_ACK)
	binary.NativeEndian.PutUint32(packet[8:], 1)
	copy(packet[16:], payload)
	if err = syscall.Sendto(fd, packet, 0, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		return fmt.Errorf("Network request failed")
	}
	for {
		reply := make([]byte, 8192)
		n, _, e := syscall.Recvfrom(fd, reply, 0)
		if e != nil {
			return fmt.Errorf("Network request timed out")
		}
		messages, e := syscall.ParseNetlinkMessage(reply[:n])
		if e != nil {
			return fmt.Errorf("Invalid network response")
		}
		for _, m := range messages {
			if m.Header.Seq != 1 || m.Header.Type != syscall.NLMSG_ERROR || len(m.Data) < 4 {
				continue
			}
			code := int32(binary.NativeEndian.Uint32(m.Data))
			if code == 0 {
				return nil
			}
			return syscall.Errno(-code)
		}
	}
}
func setInterfaceState(name string, up bool) error {
	iface, err := resolveLink(name)
	if err != nil {
		return err
	}
	payload := make([]byte, 16)
	payload[0] = syscall.AF_UNSPEC
	binary.NativeEndian.PutUint32(payload[4:], uint32(iface.Index))
	if up {
		binary.NativeEndian.PutUint32(payload[8:], syscall.IFF_UP)
	}
	binary.NativeEndian.PutUint32(payload[12:], syscall.IFF_UP)
	if err = routeRequest(syscall.RTM_NEWLINK, 0, payload); err != nil {
		return fmt.Errorf("Interface %s: cannot change administrative state", name)
	}
	return nil
}
func addressRequest(name, address string, remove bool) error {
	iface, err := resolveLink(name)
	if err != nil {
		return err
	}
	ip, network, err := net.ParseCIDR(address)
	if err != nil {
		return fmt.Errorf("Invalid interface address")
	}
	prefix, _ := network.Mask.Size()
	payload := make([]byte, 8)
	payload[0] = syscall.AF_INET
	addressBytes := ip.To4()
	if addressBytes == nil {
		payload[0] = syscall.AF_INET6
		addressBytes = ip.To16()
	}
	payload[1] = byte(prefix)
	binary.NativeEndian.PutUint32(payload[4:], uint32(iface.Index))
	if ip.To4() != nil {
		payload = append(payload, nlAttribute(syscall.IFA_LOCAL, addressBytes)...)
	}
	payload = append(payload, nlAttribute(syscall.IFA_ADDRESS, addressBytes)...)
	kind, flags := uint16(syscall.RTM_NEWADDR), uint16(syscall.NLM_F_CREATE|syscall.NLM_F_REPLACE)
	if remove {
		kind = syscall.RTM_DELADDR
		flags = 0
	}
	err = routeRequest(kind, flags, payload)
	if remove && (err == syscall.EADDRNOTAVAIL || err == syscall.ESRCH) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("Interface %s: address update failed", name)
	}
	return nil
}
func gatewayRequest(name, gateway string, remove bool) error {
	iface, err := resolveLink(name)
	if err != nil {
		return err
	}
	ip := net.ParseIP(gateway)
	if ip == nil {
		return fmt.Errorf("Invalid gateway address")
	}
	payload := make([]byte, 12)
	payload[0] = syscall.AF_INET
	addressBytes := ip.To4()
	if addressBytes == nil {
		payload[0] = syscall.AF_INET6
		addressBytes = ip.To16()
	}
	payload[4] = syscall.RT_TABLE_MAIN
	payload[5] = syscall.RTPROT_BOOT
	payload[7] = syscall.RTN_UNICAST
	payload = append(payload, nlAttribute(syscall.RTA_GATEWAY, addressBytes)...)
	payload = append(payload, nlAttribute(syscall.RTA_OIF, nlUint(uint32(iface.Index)))...)
	kind, flags := uint16(syscall.RTM_NEWROUTE), uint16(syscall.NLM_F_CREATE|syscall.NLM_F_REPLACE)
	if remove {
		kind = syscall.RTM_DELROUTE
		flags = 0
	}
	err = routeRequest(kind, flags, payload)
	if remove && err == syscall.ESRCH {
		return nil
	}
	if err != nil {
		return fmt.Errorf("Interface %s: gateway update failed", name)
	}
	return nil
}
func showRoutes() (string, error) {
	return showRoutesFamily(syscall.AF_INET)
}
func showRoutesFamily(family int) (string, error) {
	data, err := syscall.NetlinkRIB(syscall.RTM_GETROUTE, family)
	if err != nil {
		return "", fmt.Errorf("Routing table is unavailable")
	}
	messages, err := syscall.ParseNetlinkMessage(data)
	if err != nil {
		return "", fmt.Errorf("Invalid routing table response")
	}
	var b strings.Builder
	b.WriteString("Codes: C - connected, S - static, * - candidate default\n\n")
	for _, m := range messages {
		if m.Header.Type != syscall.RTM_NEWROUTE || len(m.Data) < 12 || int(m.Data[0]) != family || m.Data[4] != syscall.RT_TABLE_MAIN {
			continue
		}
		attrs, e := syscall.ParseNetlinkRouteAttr(&m)
		if e != nil {
			continue
		}
		destination, gateway, device := "0.0.0.0", "", ""
		metric := uint32(0)
		addressSize := 4
		if family == syscall.AF_INET6 {
			destination = "::"
			addressSize = 16
		}
		for _, a := range attrs {
			switch a.Attr.Type {
			case syscall.RTA_DST:
				if len(a.Value) == addressSize {
					destination = net.IP(a.Value).String()
				}
			case syscall.RTA_GATEWAY:
				if len(a.Value) == addressSize {
					gateway = net.IP(a.Value).String()
				}
			case syscall.RTA_OIF:
				if len(a.Value) >= 4 {
					if iface, e := net.InterfaceByIndex(int(binary.NativeEndian.Uint32(a.Value))); e == nil {
						device = applianceInterface(iface.Name)
					}
				}
			case syscall.RTA_PRIORITY:
				if len(a.Value) >= 4 {
					metric = binary.NativeEndian.Uint32(a.Value)
				}
			}
		}
		if gateway != "" {
			code := "S "
			if m.Data[1] == 0 {
				code = "S*"
			}
			fmt.Fprintf(&b, "%s   %s/%d [1/%d] via %s, %s\n", code, destination, m.Data[1], metric, gateway, device)
		} else {
			code := "C"
			if m.Data[5] == syscall.RTPROT_STATIC {
				code = "S"
			}
			fmt.Fprintf(&b, "%s    %s/%d is directly connected, %s\n", code, destination, m.Data[1], device)
		}
	}
	return b.String(), nil
}
