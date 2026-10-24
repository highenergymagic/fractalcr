package cli

import (
	"fractalcr/internal/config"
	"strings"
	"testing"
)

func TestConfigurationDisplayRoundTrip(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "hostname firmware", "user admin priv 15 secret 0 Synthetic-display-592!", "security passwords min-length 12", "interface GigabitEthernet0", "ip address dhcp", "exit", "network apps", "subnet 172.29.0.0/24", "exit", "container web", "image sample:1", "network apps", "environment MESSAGE \"hello world\"", "command \"/bin/demo\" \"two words\"", "memory 128M", "read-only", "exit", "line vty 0 4", "exec-timeout 1 20", "end")
	expected := s.Running.CLI()
	if strings.Contains(expected, "Synthetic-display") || !strings.Contains(expected, "secret 9 $9$") {
		t.Fatal("invalid secret display")
	}
	clone := session(t)
	execute(t, clone, "conf t")
	for _, line := range strings.Split(expected, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		if _, err := clone.Execute(line); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
	}
	if clone.Running.CLI() != expected {
		t.Fatalf("display changed after replay\n%s\n%s", expected, clone.Running.CLI())
	}
	if !config.CheckSecret(clone.Running.AAA.Users["admin"].Secret, "Synthetic-display-592!") {
		t.Fatal("imported verifier changed")
	}
}
func TestInterfaceNameCompletionAndExecution(t *testing.T) {
	s := session(t)
	s.InterfaceNames = func() []string { return []string{"GigabitEthernet0", "TenGigabitEthernet0"} }
	s.InterfaceName = func(n string) (string, error) { return "GigabitEthernet0", nil }
	execute(t, s, "conf t")
	if help := s.ContextHelp("interface "); !strings.Contains(help, "GigabitEthernet0") || strings.Contains(help, "<INTERFACE>") {
		t.Fatal(help)
	}
	if completed, _ := s.Complete("interface Gi0"); completed != "interface GigabitEthernet0 " {
		t.Fatal(completed)
	}
	execute(t, s, "interface Gi0")
	if s.Object != "GigabitEthernet0" {
		t.Fatal(s.Object)
	}
}
