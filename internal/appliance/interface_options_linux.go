package appliance

import (
	"encoding/binary"
	"fmt"
	"fractalcr/internal/config"
	"syscall"
)

func setInterfaceMTU(name string, mtu int) error {
	iface, err := resolveLink(name)
	if err != nil {
		return err
	}
	if mtu == 0 {
		mtu = 1500
	}
	if iface.MTU == mtu {
		return nil
	}
	payload := make([]byte, 16)
	binary.NativeEndian.PutUint32(payload[4:], uint32(iface.Index))
	payload = append(payload, nlAttribute(syscall.IFLA_MTU, nlUint(uint32(mtu)))...)
	if err = routeRequest(syscall.RTM_NEWLINK, 0, payload); err != nil {
		return fmt.Errorf("interface %s: MTU update failed: %w", name, err)
	}
	return nil
}

func interfaceAddresses(n config.Network, remove bool) error {
	if remove {
		if err := staticRoutes(n, true, true); err != nil {
			return err
		}
	}
	for _, list := range [][]string{n.SecondaryAddresses, n.IPv6Addresses} {
		for _, address := range list {
			if err := addressRequest(n.Interface, address, remove); err != nil {
				return err
			}
		}
	}
	if n.IPv6Gateway != "" {
		if err := gatewayRequest(n.Interface, n.IPv6Gateway, remove); err != nil {
			return err
		}
	}
	if !remove {
		return staticRoutes(n, true, false)
	}
	return nil
}
