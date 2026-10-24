package appliance

import (
	"net"
	"strings"
	"testing"

	"fractalcr/internal/config"
)

func TestInterfaceDescriptionUsesConfiguredAndObservedState(t *testing.T) {
	c := config.Default()
	c.Interfaces = map[string]*config.Network{
		"GigabitEthernet0": {Interface: "GigabitEthernet0", Description: "Management uplink"},
		"GigabitEthernet1": {Interface: "GigabitEthernet1", Shutdown: true, Description: "Spare\x1b"},
	}
	ports := []platformPort{
		{Name: "GigabitEthernet0", Interface: net.Interface{Flags: net.FlagUp | net.FlagRunning}},
		{Name: "GigabitEthernet1", Interface: net.Interface{Flags: net.FlagUp | net.FlagRunning}},
		{Name: "GigabitEthernet2", Interface: net.Interface{Flags: net.FlagUp}},
	}
	out, err := renderInterfaces(ports, "description", c)
	if err != nil {
		t.Fatal(err)
	}
	want := "Interface                      Status         Protocol Description\n" +
		"GigabitEthernet0               up             up       Management uplink\n" +
		"GigabitEthernet1               admin down     down     Spare \n" +
		"GigabitEthernet2               up             down     \n"
	if out != want {
		t.Fatalf("got %q; want %q", out, want)
	}
	selected, err := renderInterfaces(ports[:1], "description", c)
	if err != nil || strings.Contains(selected, "GigabitEthernet1") {
		t.Fatal(selected, err)
	}
}
