package appliance

import (
	"context"
	"net"
	"testing"
	"time"

	"fractalcr/internal/config"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
)

func TestDHCPObjectsOverSocket(t *testing.T) {
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	api, err := nclient4.NewWithConn(conn, net.HardwareAddr{2, 0, 0, 0, 0, 1}, nclient4.WithServerAddr(server.LocalAddr().(*net.UDPAddr)), nclient4.WithTimeout(time.Second), nclient4.WithRetry(1))
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	result := make(chan error, 1)
	go func() {
		data := make([]byte, 1500)
		for _, kind := range []dhcpv4.MessageType{dhcpv4.MessageTypeOffer, dhcpv4.MessageTypeAck, dhcpv4.MessageTypeAck} {
			server.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, peer, e := server.ReadFromUDP(data)
			if e != nil {
				result <- e
				return
			}
			request, e := dhcpv4.FromBytes(data[:n])
			if e != nil {
				result <- e
				return
			}
			reply, e := dhcpv4.NewReplyFromRequest(request, dhcpv4.WithMessageType(kind), dhcpv4.WithYourIP(net.IPv4(192, 0, 2, 10)),
				dhcpv4.WithOption(dhcpv4.OptServerIdentifier(net.IPv4(127, 0, 0, 1))),
				dhcpv4.WithOption(dhcpv4.OptSubnetMask(net.CIDRMask(24, 32))),
				dhcpv4.WithOption(dhcpv4.OptIPAddressLeaseTime(time.Hour)),
				dhcpv4.WithOption(dhcpv4.OptRouter(net.IPv4(192, 0, 2, 1))),
				dhcpv4.WithOption(dhcpv4.OptDNS(net.IPv4(192, 0, 2, 53))))
			if e != nil {
				result <- e
				return
			}
			if _, e = server.WriteToUDP(reply.ToBytes(), peer); e != nil {
				result <- e
				return
			}
		}
		result <- nil
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	lease, err := api.Request(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err = api.Renew(ctx, lease)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-result; err != nil {
		t.Fatal(err)
	}
	base := config.Network{Interface: "GigabitEthernet0"}
	network, err := leaseNetwork(base, lease.ACK)
	if err != nil || network.Address != "192.0.2.10/24" {
		t.Fatal(network, err)
	}
	t1, t2, lifetime, err := leaseTimers(lease.ACK)
	if err != nil || t1 != 30*time.Minute || t2 != time.Hour*7/8 || lifetime != time.Hour {
		t.Fatal(t1, t2, lifetime, err)
	}
	lease.ACK.UpdateOption(dhcpv4.OptSubnetMask(net.IPMask{255, 0, 255, 0}))
	if _, err = leaseNetwork(base, lease.ACK); err == nil {
		t.Fatal("noncontiguous mask accepted")
	}
	lease.ACK.UpdateOption(dhcpv4.OptIPAddressLeaseTime(time.Duration(0xffffffff) * time.Second))
	t1, t2, lifetime, err = leaseTimers(lease.ACK)
	if err != nil || t1 <= 0 || t2 <= t1 || lifetime <= t2 {
		t.Fatal("lease timer overflow", t1, t2, lifetime, err)
	}
	lease.ACK.UpdateOption(dhcpv4.OptIPAddressLeaseTime(0))
	if _, _, _, err = leaseTimers(lease.ACK); err == nil {
		t.Fatal("expired lease accepted")
	}
}
