package cli

import (
	"fractalcr/internal/config"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func replayPrivilegePolicy(t *testing.T, original *Session) *Session {
	t.Helper()
	replayed := session(t)
	execute(t, replayed, "configure terminal")
	for _, command := range original.Running.AAA.PrivilegeCLI() {
		execute(t, replayed, command)
	}
	execute(t, replayed, "end")
	if want, got := effectivePrivileges(original.Running.AAA), effectivePrivileges(replayed.Running.AAA); !reflect.DeepEqual(want, got) {
		t.Fatalf("rendered policy changed permissions on replay:\nwant %v\ngot %v", want, got)
	}
	return replayed
}

func samePrivilegeLevels(want, got map[string]int) bool {
	keys := map[string]bool{}
	for key := range want {
		keys[key] = true
	}
	for key := range got {
		keys[key] = true
	}
	for key := range keys {
		mode, command, _ := strings.Cut(key, "\x00")
		a, ok := want[key]
		if !ok {
			a = defaultPrivilegeLevel(mode, command)
		}
		b, ok := got[key]
		if !ok {
			b = defaultPrivilegeLevel(mode, command)
		}
		if a != b {
			return false
		}
	}
	return true
}

func TestPrivilegeAllRetainsDocumentedConfigurationForm(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "privilege configure all level 3 interface", "privilege exec level 3 configure terminal", "end")
	want := []string{"privilege configure all level 3 interface", "privilege exec level 3 configure terminal", "privilege exec level 3 configure"}
	if got := s.Running.AAA.PrivilegeCLI(); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	file := filepath.Join(t.TempDir(), "startup-config.json")
	if err := config.AtomicWrite(file, s.Running.JSON(), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(file)
	if err != nil || !reflect.DeepEqual(loaded.AAA.PrivilegeCLI(), want) {
		t.Fatal("saved group policy did not survive loading", err)
	}
	r := replayPrivilegePolicy(t, s)
	r.EnforcePrivilege, r.Privilege = true, 3
	execute(t, r, "conf t", "interface GigabitEthernet0", "shutdown", "description delegated", "end")
	if !r.Running.AllInterfaces()["GigabitEthernet0"].Shutdown {
		t.Fatal("group lost its submode permissions")
	}
}

func TestPrivilegeGroupsMatchNodeAssignmentsAcrossEdits(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t")
	want := map[string]int{}
	for _, command := range []string{
		"privilege configure all level 5 interface",
		"privilege interface all level 7 ip",
		"privilege interface level 9 description",
		"privilege configure all level 4 line",
		"privilege line all level 6 transport",
		"privilege interface reset shutdown",
		"privilege line reset transport input ssh",
		"privilege configure all level 3 interface",
		"no privilege configure all level 4 line",
		"privilege configure all reset interface",
	} {
		words, err := Words(command)
		if err != nil {
			t.Fatal(err)
		}
		r, ok := parsePrivilege(words)
		if !ok {
			t.Fatal(command)
		}
		path := words[r.start:]
		changed := map[string]config.CommandPrivilege{}
		if r.all {
			changed = privilegeGroup(config.CommandPrivilege{Mode: r.mode, Command: strings.Join(path, " "), Level: r.level, All: true})
		}
		for i := 1; i <= len(path); i++ {
			rule := config.CommandPrivilege{Mode: r.mode, Command: strings.Join(path[:i], " "), Level: r.level}
			changed[privilegeKey(rule)] = rule
		}
		for key, rule := range changed {
			if r.reset {
				delete(want, key)
			} else {
				want[key] = rule.Level
			}
		}
		execute(t, s, command)
		if got := effectivePrivileges(s.Running.AAA); !samePrivilegeLevels(want, got) {
			t.Fatalf("%s changed unrelated nodes:\nwant %v\ngot %v", command, want, got)
		}
		replayPrivilegePolicy(t, s)
	}
}

func TestPrivilegeNestedGroupsAndPartialReset(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "privilege exec all level 5 show", "privilege exec all level 7 show ip", "privilege exec all level 9 show version", "end")
	replayPrivilegePolicy(t, s)
	before := effectivePrivileges(s.Running.AAA)
	execute(t, s, "conf t", "privilege exec reset show ip route", "end")
	for _, key := range []string{"exec\x00show", "exec\x00show ip", "exec\x00show ip route"} {
		delete(before, key)
	}
	if after := effectivePrivileges(s.Running.AAA); !samePrivilegeLevels(before, after) {
		t.Fatalf("partial reset changed unrelated permissions:\nwant %v\ngot %v", before, after)
	}
	replayPrivilegePolicy(t, s)
	execute(t, s, "conf t", "privilege exec all level 4 show", "end")
	if got := s.Running.AAA.PrivilegeCLI(); len(got) != 1 || got[0] != "privilege exec all level 4 show" {
		t.Fatal("covering all-policy retained superseded exceptions", got)
	}
	replayPrivilegePolicy(t, s)
}

func TestPrivilegeGroupExceptionsAndSubmodeReset(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "privilege configure all level 5 interface", "privilege interface level 7 shutdown", "end")
	if rendered := strings.Join(s.Running.AAA.PrivilegeCLI(), "\n"); !strings.Contains(rendered, "privilege configure all level 5 interface") || !strings.Contains(rendered, "privilege interface level 7 shutdown") {
		t.Fatal(rendered)
	}
	replayPrivilegePolicy(t, s)
	s.EnforcePrivilege, s.Privilege, s.Mode = true, 5, "interface"
	if _, _, err := s.Resolve("shutdown"); err == nil {
		t.Fatal("explicit exception was hidden by a group")
	}
	if _, _, err := s.Resolve("description allowed"); err != nil {
		t.Fatal(err)
	}
	s.Privilege, s.Mode = 15, ""
	execute(t, s, "conf t", "privilege interface reset shutdown", "end")
	replayPrivilegePolicy(t, s)
	s.Privilege, s.Mode = 7, "interface"
	if _, _, err := s.Resolve("shutdown"); err == nil {
		t.Fatal("reset node inherited the old group again")
	}
	if _, _, err := s.Resolve("description still-allowed"); err != nil {
		t.Fatal(err)
	}
}

func TestNativePrivilegeGroupFitsConfigurationAndReplays(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "privilege exec all level 5 request container-engine", "end")
	if len(s.Running.AAA.CommandPrivileges) != 2 || len(effectivePrivileges(s.Running.AAA)) < 100 {
		t.Fatal("native catalog was not represented as a group")
	}
	r := replayPrivilegePolicy(t, s)
	r.EnforcePrivilege, r.Privilege = true, 5
	for _, command := range []string{"request container-engine events", "request container-engine debug enable network ls"} {
		if _, _, err := r.Resolve(command); err != nil {
			t.Fatal(command, err)
		}
	}
}
