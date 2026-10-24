package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestVirtualInterfaceReplay(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "interface eth1", "role container", "exit", "interface eth2", "role container", "exit",
		"interface po1", "role container", "mtu 9000", "member eth1", "member eth2", "channel-mode 802.3ad", "exit",
		"interface vl100", "role container", "parent po1", "mtu 9000", "ip address 192.0.2.2/24", "exit", "end", "write memory")
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
	if !reflect.DeepEqual(s.Running.AllInterfaces(), replay.Running.AllInterfaces()) {
		t.Fatal("virtual interface replay")
	}
	execute(t, s, "conf t", "no interface vl100", "no interface po1")
	if s.Running.AllInterfaces()["Vlan100"] != nil || s.Running.AllInterfaces()["Port-channel1"] != nil {
		t.Fatal("aliases did not remove virtual interfaces")
	}
}
