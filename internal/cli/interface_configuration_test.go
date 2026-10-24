package cli

import (
	"strings"
	"testing"
)

func TestInterfaceRunningConfiguration(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "interface GigabitEthernet0")
	if s.Prompt() != s.Running.Hostname+"(config-if)# " {
		t.Fatal(s.Prompt())
	}
	execute(t, s, "end")
	execute(t, s, "conf t", "interface GigabitEthernet0", "description selected-port", "ip address 192.0.2.1 255.255.255.0", "exit", "interface GigabitEthernet1", "description other-port", "exit", "hostname secret-hostname", "end")
	var original string
	for _, command := range []string{"show running-config interface GigabitEthernet0", "sh run int gi0", "sh run int gi 0"} {
		out, err := s.Execute(command)
		if err != nil || !strings.Contains(out, "interface GigabitEthernet0\n") || !strings.Contains(out, " description selected-port\n") || !strings.Contains(out, " ip address 192.0.2.1 255.255.255.0\n") || strings.Contains(out, "other-port") || strings.Contains(out, "secret-hostname") || strings.Contains(out, " exit") {
			t.Fatal(command, out, err)
		}
		if original != "" && out != original {
			t.Fatal("different display for interface alias", out, original)
		}
		original = out
	}
	for _, command := range []string{"sh run int gi0 linenum", "sh run int gi 0 linenum"} {
		out, err := s.Execute(command + " | include interface")
		if err != nil || out != " 2 : interface GigabitEthernet0" {
			t.Fatal(command, out, err)
		}
	}
	for _, command := range []string{"sh run int gi999", "sh run int gi junk", "sh run int gi 0 stray"} {
		if _, err := s.Execute(command); err == nil {
			t.Fatal("accepted invalid interface", command)
		}
	}
	literal := `uplink  "quoted" to C:\rack\new-port | literal`
	execute(t, s, "conf t", "interface GigabitEthernet0", "desc "+literal)
	words, _, err := s.Resolve("desc " + literal)
	if err != nil {
		t.Fatal(err)
	}
	execute(t, s, Join(words), "end")
	if got := s.Running.AllInterfaces()["GigabitEthernet0"].Description; got != literal {
		t.Fatal("description was interpreted", got)
	}
	out, err := s.Execute("show running-config interface gi0")
	if err != nil || !strings.Contains(out, " description "+literal+"\n") {
		t.Fatal(out, err)
	}
	clone := session(t)
	execute(t, clone, "conf t")
	for _, line := range strings.Split(s.Running.CLI(), "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "!") {
			continue
		}
		if _, err := clone.Execute(line); err != nil {
			t.Fatal(line, err)
		}
	}
	if got := clone.Running.AllInterfaces()["GigabitEthernet0"].Description; got != literal {
		t.Fatal("description replay changed", got)
	}
	execute(t, s, "conf t", "file privilege 5", "privilege exec level 5 show running-config interface", "end")
	s.EnforcePrivilege, s.Privilege = true, 5
	if out, err := s.Execute("sh run int gi0"); err != nil || strings.Contains(out, "selected-port") || strings.Contains(out, "interface GigabitEthernet0") {
		t.Fatal("restricted interface leaked", out, err)
	}
	s.Running.AAA.FilePrivilege = nil
	if _, err := s.Execute("sh run int gi0"); err == nil {
		t.Fatal("bypassed file access level")
	}
}
