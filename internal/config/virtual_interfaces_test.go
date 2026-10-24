package config

import "testing"

func TestVirtualInterfaceDependencies(t *testing.T) {
	valid := func() Config {
		return Config{Interfaces: map[string]*Network{
			"eth1":          {Interface: "eth1", Role: "container"},
			"Port-channel1": {Interface: "Port-channel1", Role: "container", Members: []string{"eth1"}},
			"Vlan100":       {Interface: "Vlan100", Role: "container", Parent: "Port-channel1"},
		}}
	}
	if err := valid().validateVirtualInterfaces(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(Config){
		"missing parent":      func(c Config) { delete(c.Interfaces, "Port-channel1") },
		"addressed member":    func(c Config) { c.Interfaces["eth1"].Address = "192.0.2.2/24" },
		"member VLAN":         func(c Config) { c.Interfaces["Vlan100"].Parent = "eth1" },
		"oversized VLAN":      func(c Config) { c.Interfaces["Vlan100"].MTU = 9000 },
		"member MTU mismatch": func(c Config) { c.Interfaces["eth1"].MTU = 9000 },
		"duplicate membership": func(c Config) {
			c.Interfaces["Port-channel2"] = &Network{Interface: "Port-channel2", Role: "container", Members: []string{"eth1"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := valid()
			change(c)
			if c.validateVirtualInterfaces() == nil {
				t.Fatal("invalid dependency accepted")
			}
		})
	}
}
