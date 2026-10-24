package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestAttachmentsAndInterfaceOptionsReplay(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "interface eth0", "role management", "description uplink to management", "mtu 9000", "ip address 192.0.2.2/24", "ip address 198.51.100.2/24 secondary", "ipv6 address 2001:db8::2/64", "ipv6 gateway 2001:db8::1", "exit",
		"network front", "exit", "network back", "exit", "container app", "image example/app:1", "network attach front", "network attach front alias web", "network attach front ipv4 10.1.0.2", "network attach back",
		"gpu device GPU-example", "bind /datastore/data0/files /srv read-only propagation rprivate", "security option apparmor=unconfined", "namespace pid private",
		"publish 127.0.0.1:8080-8082:80-82/tcp", "end", "write memory")
	replay := session(t)
	execute(t, replay, "conf t")
	for _, line := range strings.Split(s.Running.CLI(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "!") {
			execute(t, replay, line)
		}
	}
	if !reflect.DeepEqual(s.Running.Containers, replay.Running.Containers) || !reflect.DeepEqual(s.Running.Network, replay.Running.Network) {
		t.Fatalf("replay mismatch\n%s", replay.Running.JSON())
	}
	execute(t, s, "conf t", "container app", "no network attach front alias web", "no gpu device GPU-example", "no bind /srv", "no security option apparmor=unconfined", "no namespace pid", "no publish 127.0.0.1:8080-8082:80-82/tcp")
	v := s.Running.Containers["app"]
	if len(v.Networks["front"].Aliases)+len(v.GPUDevices)+len(v.Binds)+len(v.SecurityOptions)+len(v.Publish) != 0 || v.PIDMode != "" {
		t.Fatal("no forms failed", v)
	}
}
