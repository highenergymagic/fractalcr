package config

import "testing"

func TestNetworkPolicy(t *testing.T) {
	c := Default()
	c.EditInterface("eth0").DHCP = true
	n := c.EditInterface("eth1")
	n.Role = "container"
	c.Networks = map[string]DockerNetwork{"lan": {Driver: "macvlan", Parent: "eth1", Subnet: "192.0.2.0/24", Gateway: "192.0.2.1"}}
	c.NTP.Servers = []string{"time.example.org", "192.0.2.123"}
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	for name, mutate := range map[string]func(*Config){
		"duplicate":               func(c *Config) { c.Interfaces["eth0"] = c.Network },
		"null":                    func(c *Config) { c.Interfaces["eth1"] = nil },
		"two-management":          func(c *Config) { c.Interfaces["eth1"].Role = "management" },
		"dhcp-static":             func(c *Config) { c.Network.Address = "192.0.2.5/24" },
		"container-default-route": func(c *Config) { c.Interfaces["eth1"].Gateway = "192.0.2.1" },
		"ntp-injection":           func(c *Config) { c.NTP.Servers = []string{"a\nallow all"} },
		"management-macvlan":      func(c *Config) { n := c.Networks["lan"]; n.Parent = "eth0"; c.Networks["lan"] = n },
		"gateway-outside-subnet":  func(c *Config) { n := c.Networks["lan"]; n.Gateway = "198.51.100.1"; c.Networks["lan"] = n },
	} {
		t.Run(name, func(t *testing.T) {
			bad := c.Clone()
			mutate(&bad)
			if bad.Validate() == nil {
				t.Fatal("accepted invalid policy")
			}
		})
	}
	if _, e := Decode(c.JSON()); e != nil {
		t.Fatal(e)
	}
}
