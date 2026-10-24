package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestEngineAndNetworkOptionsRoundTrip(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "system container-engine", "registry mirror https://registry.example.test", "registry insecure 192.0.2.0/24",
		"address-pool 10.64.0.0/16 size 24", "logging driver local", "logging option max-size 10m", "live-restore", "mtu 1400",
		"max-concurrent-downloads 4", "max-concurrent-uploads 2", "shutdown-timeout 30", "dns server 192.0.2.53", "label site lab", "exit",
		"network blue", "ipv6 enable", "ipam pool 10.65.0.0/24", "ipam pool 10.65.0.0/24 gateway 10.65.0.1",
		"ipam pool 10.65.0.0/24 range 10.65.0.128/25", "ipam pool 10.65.0.0/24 reserve router 10.65.0.2",
		"ipam pool fd00:1234::/64", "ipam pool fd00:1234::/64 gateway fd00:1234::1", "label site lab", "option com.docker.network.bridge.enable_icc false", "end", "wr me")
	restored, err := New(s.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Running, restored.Running) {
		t.Fatal("save/reload lost engine or network settings")
	}
	replay := session(t)
	execute(t, replay, "conf t")
	for _, line := range strings.Split(s.Running.CLI(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		execute(t, replay, line)
	}
	if !reflect.DeepEqual(s.Running.Engine, replay.Running.Engine) || !reflect.DeepEqual(s.Running.Networks, replay.Running.Networks) {
		t.Fatal("render/replay lost settings")
	}
	files, err := s.Running.Render()
	if err != nil {
		t.Fatal(err)
	}
	var daemon map[string]any
	if err = json.Unmarshal(files["docker/daemon.json"], &daemon); err != nil {
		t.Fatal(err)
	}
	if daemon["live-restore"] != true || daemon["mtu"] != float64(1400) || daemon["shutdown-timeout"] != float64(30) || daemon["data-root"] != "/datastore/data0/docker" {
		t.Fatal(daemon)
	}
	ipam := s.Running.Networks["blue"].IPAMAPI()
	pools := ipam["Config"].([]map[string]any)
	if len(pools) != 2 || pools[0]["IPRange"] != "10.65.0.128/25" || pools[1]["Subnet"] != "fd00:1234::/64" {
		t.Fatal(ipam)
	}
	execute(t, s, "conf t", "network blue")
	before := string(s.Running.JSON())
	for _, bad := range []string{"ipam pool 10.65.0.0/24 gateway 10.66.0.1", "ipam pool 10.65.0.0/24 range 10.0.0.0/8", "no ipv6 enable", "option parent eth0"} {
		if _, err = s.Execute(bad); err == nil || string(s.Running.JSON()) != before {
			t.Fatal("invalid command accepted or changed configuration", bad, err)
		}
	}
	execute(t, s, "no ipam pool fd00:1234::/64", "no ipv6 enable", "no ipam pool 10.65.0.0/24 reserve router", "exit", "system container-engine", "no registry mirror https://registry.example.test", "no address-pool 10.64.0.0/16", "no shutdown-timeout", "end")
	if len(s.Running.Engine.RegistryMirrors) != 0 || len(s.Running.Engine.AddressPools) != 0 || s.Running.Engine.ShutdownTimeout != nil {
		t.Fatal("no forms failed")
	}
}
