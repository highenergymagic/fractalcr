package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestRoutesAndVolumesRoundTrip(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "interface eth0", "role management", "ip address 192.0.2.2/24", "exit",
		"ip route 198.51.100.0/24 eth0 192.0.2.1 metric 200", "ipv6 route 2001:db8:1::/64 eth0 direct",
		"volume data", "driver local", "option type tmpfs", "option device tmpfs", "option o size=16m", "label site test", "end", "write memory")
	restored, err := New(s.StateDir)
	if err != nil || !reflect.DeepEqual(s.Running, restored.Running) {
		t.Fatal("persistence", err)
	}
	replay := session(t)
	execute(t, replay, "conf t")
	for _, line := range strings.Split(s.Running.CLI(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "!") {
			execute(t, replay, line)
		}
	}
	if !reflect.DeepEqual(s.Running.Network, replay.Running.Network) || !reflect.DeepEqual(s.Running.Volumes, replay.Running.Volumes) {
		t.Fatal("route/volume replay")
	}
	execute(t, s, "conf t", "no ip route 198.51.100.0/24 eth0", "no ipv6 route 2001:db8:1::/64 eth0", "volume data", "no option o", "no label site", "exit", "no volume data")
	if len(s.Running.Network.Routes) != 0 || len(s.Running.Volumes) != 0 {
		t.Fatal("no forms did not remove settings")
	}
}
