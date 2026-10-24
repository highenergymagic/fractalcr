package cli

import (
	"fractalcr/internal/config"
	"strings"
	"testing"
)

func TestIOSFilePromptConfiguration(t *testing.T) {
	s := session(t)
	if _, err := s.Execute("conf t"); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"quiet", "noisy", "alert"} {
		if _, err := s.Execute("file prompt " + mode); err != nil {
			t.Fatal(err)
		}
		c, err := config.Decode(s.Running.JSON())
		if err != nil {
			t.Fatal(err)
		}
		want := mode
		if mode == "alert" {
			want = ""
		}
		if c.FilePrompt != want {
			t.Fatal("policy did not persist", c.FilePrompt)
		}
	}
	for _, line := range []string{"no file prompt", "default file prompt"} {
		if _, err := s.Execute("file prompt quiet"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Execute(line); err != nil {
			t.Fatal(err)
		}
		if s.Running.FilePrompt != "" {
			t.Fatal("default not restored", line)
		}
	}
	if _, err := s.Execute("file prompt invalid"); err == nil {
		t.Fatal("accepted unknown policy")
	}
	c := config.Default()
	c.FilePrompt = "invalid"
	if _, err := config.Decode(c.JSON()); err == nil {
		t.Fatal("accepted invalid saved policy")
	}
}

func TestIOSFilesystemHelpAndReloadGrammar(t *testing.T) {
	s := session(t)
	s.FileSystems = func() []string { return []string{"bootflash:", "data0:", "flash:", "nvram:"} }
	for _, line := range []string{"dir ", "dir /recursive ", "delete ", "copy data0:source "} {
		help, e := s.QueryHelp(line)
		for _, expected := range []string{"<filesystem:>", "data0:", "flash:", "nvram:"} {
			if e != nil || !strings.Contains(help, expected) {
				t.Fatal(line, expected, help, e)
			}
		}
	}
	if help, e := s.QueryHelp("dir fl"); e != nil || help != "flash:" {
		t.Fatal(help, e)
	}
	if line, feedback := s.Complete("dir fl"); line != "dir flash: " || feedback != "" {
		t.Fatal(line, feedback)
	}
	for _, command := range []string{"dir /rec /all flash:", "dir all-filesystems", "dir | include image", "en 15", "dis 1", "reload in 1:30 maintenance", "reload at 12:00 oct 25 maintenance", "reload cancel", "show reload"} {
		if _, local, e := s.Resolve(command); e != nil || local {
			t.Fatal(command, local, e)
		}
	}
	s.EnforcePrivilege = true
	s.Privilege = 1
	for _, command := range []string{"reload", "reload in 10", "reload cancel"} {
		if _, _, e := s.Resolve(command); e == nil {
			t.Fatal("unprivileged reload", command)
		}
	}
	if _, _, e := s.Resolve("enable 15"); e != nil {
		t.Fatal(e)
	}
}

func TestIOSAddressMasksAndSaveOutput(t *testing.T) {
	s := session(t)
	for _, line := range []string{"conf t", "interface GigabitEthernet0", "ip address 192.0.2.10 255.255.255.0", "ip address 198.51.100.10 255.255.255.0 secondary", "end"} {
		if _, e := s.Execute(line); e != nil {
			t.Fatal(line, e)
		}
	}
	out, e := s.Execute("show running-config")
	if e != nil || !strings.Contains(out, "ip address 192.0.2.10 255.255.255.0") || !strings.Contains(out, "ip address 198.51.100.10 255.255.255.0 secondary") {
		t.Fatal(out, e)
	}
	if out, e = s.Execute("wr me"); e != nil || out != "Building configuration...\n[OK]" {
		t.Fatal(out, e)
	}
	for _, line := range []string{"conf t", "interface GigabitEthernet0", "no ip address 198.51.100.10 255.255.255.0 secondary", "no ip address 192.0.2.10 255.255.255.0"} {
		if _, e := s.Execute(line); e != nil {
			t.Fatal(line, e)
		}
	}
	if _, e = s.Execute("ip address 192.0.2.10 255.0.255.0"); e == nil {
		t.Fatal("accepted noncontiguous mask")
	}
}

func TestIOSLineTimeoutReset(t *testing.T) {
	s := session(t)
	for _, line := range []string{"conf t", "line console 0", "exec-timeout 1 30", "no exec-timeout"} {
		if _, err := s.Execute(line); err != nil {
			t.Fatal(line, err)
		}
	}
	if s.Running.AAA.ConsoleTimeout != 0 {
		t.Fatal("no exec-timeout did not disable timeout")
	}
	if _, err := s.Execute("default exec-timeout"); err != nil || s.Running.AAA.ConsoleTimeout != 600 {
		t.Fatal(err, s.Running.AAA.ConsoleTimeout)
	}
}
