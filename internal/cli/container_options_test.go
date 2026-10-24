package cli

import (
	"fractalcr/internal/config"
	"reflect"
	"strings"
	"testing"
)

func TestContainerOptionsPersistenceAndReplay(t *testing.T) {
	s := session(t)
	execute(t, s, "configure terminal", "container app", "image alpine:3.21",
		"user 1000:1000", "guest-hostname guest", "working-directory /srv", "entrypoint /bin/app --foreground",
		`label org.example.note "literal $(no-shell)"`, "cpus 1.25", "cpu-shares 512", "cpu-set 0-2,4",
		"memory 256M", "memory-reservation 128M", "memory-swap 512M", "memory-shared 32M",
		"init", "tty", "stdin-open", "stop-signal SIGTERM", "stop-timeout 20",
		"logging driver local", "logging option max-size 10m", "capability drop all", "capability add NET_BIND_SERVICE",
		"dns server 192.0.2.53", "dns search example.test", "dns option ndots:2", "host host.internal host-gateway",
		"sysctl net.ipv4.ip_unprivileged_port_start 0", "tmpfs /tmp rw,size=64m", "device /dev/null /dev/test rw",
		"ulimit nofile 1024 2048", "healthcheck command /bin/app --check", "healthcheck interval 30s",
		"healthcheck timeout 5s", "healthcheck start-period 10s", "healthcheck start-interval 2s", "healthcheck retries 3", "end", "write memory")
	if s.Running.Hostname != "ctr" || s.Running.Containers["app"].Hostname != "guest" {
		t.Fatal("container hostname changed appliance hostname")
	}
	reloaded, e := New(s.StateDir)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(s.Running, reloaded.Running) {
		t.Fatal("startup configuration lost fields")
	}
	replay := session(t)
	execute(t, replay, "configure terminal")
	for _, line := range strings.Split(s.Running.CLI(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		execute(t, replay, line)
	}
	if !reflect.DeepEqual(s.Running.Containers, replay.Running.Containers) {
		t.Fatalf("CLI replay differs:\n%s", replay.Running.JSON())
	}
	before := s.Running.JSON()
	execute(t, s, "configure terminal", "container app")
	for _, bad := range []string{"cpus -1", "cpu-set garbage", "memory-reservation 512M", "healthcheck timeout 0s", "device /etc/passwd /dev/test rw", "ulimit nofile 2048 1024", "dns server nonsense"} {
		if _, err := s.Execute(bad); err == nil {
			t.Fatalf("accepted %s", bad)
		}
		if string(before) != string(s.Running.JSON()) {
			t.Fatalf("failed command changed config: %s", bad)
		}
	}
	execute(t, s, "no user", "no guest-hostname", "no cpus", "no healthcheck", "no device /dev/test", "no label org.example.note", "no logging", "no capability add NET_BIND_SERVICE")
	v := s.Running.Containers["app"]
	if v.User != "" || v.Hostname != "" || v.NanoCPUs != 0 || len(v.Devices) != 0 || len(v.Labels) != 0 || v.LogDriver != "" || len(v.CapAdd) != 0 || !reflect.DeepEqual(v.Health, config.Healthcheck{}) {
		t.Fatal("no form failed", v)
	}
}
