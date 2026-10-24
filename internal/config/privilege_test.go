package config

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompactPrivilegeValidationAndRoundTrip(t *testing.T) {
	rules := []CommandPrivilege{{Mode: "exec", Command: "show", Level: 5, All: true},
		{Mode: "exec", Command: "show ip route", Level: 7}, {Mode: "exec", Command: "show ip", Level: 7}}
	packed := CompactPrivileges(rules)
	if len(packed) != 2 || len(packed[1].Commands) != 2 {
		t.Fatal(packed)
	}
	if err := validateCommandPrivileges(packed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual((AAA{CommandPrivileges: rules}).OrderedPrivileges(), (AAA{CommandPrivileges: packed}).OrderedPrivileges()) {
		t.Fatal("compaction changed policy")
	}
	for _, bad := range [][]CommandPrivilege{
		{{Mode: "exec", Commands: []string{"show", "show"}, Level: 5}},
		{{Mode: "exec", Command: "show", Commands: []string{"show ip"}, Level: 5}},
		{{Mode: "exec", Commands: []string{"show"}, Level: 5, All: true}},
		{{Mode: "exec", Commands: []string{"show\nreload"}, Level: 5}},
		{{Mode: "exec", Commands: []string{"show"}, Level: 16}},
		{{Mode: "exec", Command: "show", Level: 5}, {Mode: "exec", Commands: []string{"show"}, Level: 5}},
	} {
		if validateCommandPrivileges(bad) == nil {
			t.Fatal("accepted malformed or duplicate policy", bad)
		}
	}
}

func TestConfigurationCapacitySupportsPoliciesAndRemainsBounded(t *testing.T) {
	c := Default()
	for i := 0; i < 1200; i++ {
		c.AAA.CommandPrivileges = append(c.AAA.CommandPrivileges, CommandPrivilege{Mode: "exec", Command: fmt.Sprintf("show platform diagnostic-%04d", i), Level: 5})
	}
	data := c.JSON()
	if len(data) <= 64*1024 || len(data) > MaxConfigBytes {
		t.Fatal("test must exercise the expanded but bounded capacity", len(data))
	}
	p := filepath.Join(t.TempDir(), "startup-config.json")
	if err := AtomicWrite(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(p)
	if err != nil || !reflect.DeepEqual(c.AAA.CommandPrivileges, loaded.AAA.CommandPrivileges) {
		t.Fatal("large policy did not survive reboot loading", err)
	}
	for i := 1200; i < 3000; i++ {
		c.AAA.CommandPrivileges = append(c.AAA.CommandPrivileges, CommandPrivilege{Mode: "exec", Command: fmt.Sprintf("show platform diagnostic-%04d", i), Level: 5})
	}
	if len(c.JSON()) <= MaxConfigBytes {
		t.Fatal("oversize fixture is too small")
	}
	if c.Validate() == nil {
		t.Fatal("accepted an oversized configuration")
	}
}
