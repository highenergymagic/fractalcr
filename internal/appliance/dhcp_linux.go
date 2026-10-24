package appliance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"fractalcr/internal/config"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
)

func leaseNetwork(base config.Network, ack *dhcpv4.DHCPv4) (config.Network, error) {
	if ack == nil {
		return base, fmt.Errorf("DHCP acknowledgement is missing")
	}
	ip := ack.YourIPAddr.To4()
	ones, bits := ack.SubnetMask().Size()
	if ip == nil || !ip.IsGlobalUnicast() || bits != 32 || ones < 1 {
		return base, fmt.Errorf("invalid DHCP address or netmask")
	}
	base.DHCP = false
	base.Address = fmt.Sprintf("%s/%d", ip, ones)
	base.Gateway, base.DNS = "", ""
	if base.IsManagement() {
		for _, option := range []struct {
			values []net.IP
			target *string
		}{{ack.Router(), &base.Gateway}, {ack.DNS(), &base.DNS}} {
			if len(option.values) == 0 {
				continue
			}
			address := option.values[0].To4()
			if address == nil || !address.IsGlobalUnicast() {
				return base, fmt.Errorf("invalid DHCP gateway or name server")
			}
			*option.target = address.String()
		}
	}
	return base, nil
}
func leaseTimers(ack *dhcpv4.DHCPv4) (time.Duration, time.Duration, time.Duration, error) {
	lifetime := ack.IPAddressLeaseTime(0)
	if lifetime < time.Second {
		return 0, 0, 0, fmt.Errorf("invalid DHCP lease lifetime")
	}
	t1 := ack.IPAddressRenewalTime(lifetime / 2)
	t2 := ack.IPAddressRebindingTime(lifetime - lifetime/8)
	if t1 <= 0 || t1 >= t2 || t2 >= lifetime {
		t1, t2 = lifetime/2, lifetime-lifetime/8
	}
	return t1, t2, lifetime, nil
}
func waitDHCP(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func installLease(base config.Network, ack *dhcpv4.DHCPv4) error {
	lease, err := leaseNetwork(base, ack)
	if err != nil {
		return err
	}
	clearLease(base.Interface)
	data, _ := json.Marshal(lease)
	if err = config.AtomicWrite(leasePath(base.Interface), data, 0600); err != nil {
		return err
	}
	return applyAddress(lease)
}

// DHCP runs in the network service. Packets become lease objects directly;
// no child client, environment-variable hook, shell, or utility is involved.
func runDHCP(ctx context.Context, base config.Network, kernel string) error {
	raw, err := nclient4.New(kernel, nclient4.WithTimeout(2*time.Second), nclient4.WithRetry(2))
	if err != nil {
		return fmt.Errorf("DHCP socket unavailable on %s", base.Interface)
	}
	defer raw.Close()
	defer clearLease(base.Interface)
	var lease *nclient4.Lease
	var renewAt, rebindAt, expires time.Time
	for ctx.Err() == nil {
		if lease == nil {
			lease, err = raw.Request(ctx)
		} else {
			if !waitDHCP(ctx, time.Until(renewAt)) {
				return nil
			}
			if !time.Now().Before(expires) {
				clearLease(base.Interface)
				lease = nil
				continue
			}
			rebinding := !time.Now().Before(rebindAt)
			boundary := rebindAt
			if rebinding {
				boundary = expires
			}
			requestCtx, stop := context.WithDeadline(ctx, boundary)
			var next *nclient4.Lease
			if rebinding {
				request, e := dhcpv4.NewRenewFromAck(lease.ACK, dhcpv4.WithBroadcast(true))
				err = e
				if e == nil {
					reply, e := raw.SendAndRead(requestCtx, nclient4.DefaultServers, request, nclient4.IsMessageType(dhcpv4.MessageTypeAck, dhcpv4.MessageTypeNak))
					err = e
					if e == nil {
						if reply.MessageType() == dhcpv4.MessageTypeNak {
							err = &nclient4.ErrNak{Offer: lease.Offer, Nak: reply}
						} else {
							next = &nclient4.Lease{Offer: reply, ACK: reply, CreationTime: time.Now()}
						}
					}
				}
			} else {
				server := lease.ACK.ServerIdentifier().To4()
				if server == nil {
					err = fmt.Errorf("DHCP server identifier missing")
				} else {
					unicast, e := nclient4.New(kernel, nclient4.WithUnicast(&net.UDPAddr{IP: lease.ACK.YourIPAddr, Port: 68}), nclient4.WithServerAddr(&net.UDPAddr{IP: server, Port: 67}), nclient4.WithTimeout(time.Second), nclient4.WithRetry(1))
					err = e
					if e == nil {
						next, err = unicast.Renew(requestCtx, lease)
						unicast.Close()
					}
				}
			}
			stop()
			if ctx.Err() != nil {
				return nil
			}
			if err != nil {
				var nak *nclient4.ErrNak
				if errors.As(err, &nak) || !time.Now().Before(expires) {
					clearLease(base.Interface)
					lease = nil
					continue
				}
				delay := time.Until(boundary) / 2
				if delay > 30*time.Second {
					delay = 30 * time.Second
				}
				if delay < 100*time.Millisecond {
					delay = 100 * time.Millisecond
				}
				renewAt = time.Now().Add(delay)
				continue
			}
			// Some servers omit unchanged network options on renewal.
			for _, option := range []dhcpv4.OptionCode{dhcpv4.OptionSubnetMask, dhcpv4.OptionRouter, dhcpv4.OptionDomainNameServer} {
				if next.ACK.Options.Get(option) == nil {
					next.ACK.Options.Update(dhcpv4.OptGeneric(option, lease.ACK.Options.Get(option)))
				}
			}
			lease = next
		}
		if err != nil {
			lease = nil
			if !waitDHCP(ctx, 5*time.Second) {
				return nil
			}
			continue
		}
		t1, t2, lifetime, e := leaseTimers(lease.ACK)
		if e != nil {
			clearLease(base.Interface)
			lease = nil
			if !waitDHCP(ctx, time.Second) {
				return nil
			}
			continue
		}
		if _, e = leaseNetwork(base, lease.ACK); e != nil {
			lease = nil
			if !waitDHCP(ctx, time.Second) {
				return nil
			}
			continue
		}
		if e = installLease(base, lease.ACK); e != nil {
			return e
		}
		renewAt = lease.CreationTime.Add(t1)
		rebindAt = lease.CreationTime.Add(t2)
		expires = lease.CreationTime.Add(lifetime)
	}
	return nil
}
