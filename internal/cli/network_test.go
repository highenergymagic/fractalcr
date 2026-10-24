package cli

import "testing"

func TestMultipleInterfacesAndDockerNetworks(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "interface eth0", "role management", "ip address dhcp", "exit", "interface eth1", "role container", "no ip address", "exit", "ntp server time.example.org", "network workload", "driver macvlan", "parent eth1", "subnet 192.0.2.0/24", "gateway 192.0.2.1", "exit", "container app", "network workload", "commit")
	if len(s.Running.AllInterfaces()) != 2 || !s.Running.Network.DHCP || s.Running.Containers["app"].Network != "workload" {
		t.Fatal(s.Running)
	}
	execute(t, s, "end")
	if _, e := s.Execute("show capabilities"); e == nil {
		t.Fatal("removed command still exists")
	}
	execute(t, s, "conf t", "interface eth0", "ip address 198.51.100.2/24", "commit")
	if s.Running.Network.DHCP {
		t.Fatal("static conversion retained DHCP")
	}
}
