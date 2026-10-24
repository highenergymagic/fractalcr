package appliance

import (
	"fmt"
	"fractalcr/internal/config"
	"strings"
)

func networkOperation(w []string, c config.Config) (string, error) {
	if !config.Name.MatchString(w[2]) {
		return "", fmt.Errorf("invalid network name")
	}
	name := w[2]
	if len(w) == 4 && w[0] == "show" && w[3] == "detail" {
		return networkDetail(name)
	}
	if w[0] != "request" {
		return "", fmt.Errorf("invalid network operation")
	}
	if len(w) == 5 && (w[3] == "connect" || w[3] == "disconnect") && config.Name.MatchString(w[4]) {
		return engineAction("POST", "/networks/"+name+"/"+w[3], map[string]any{"Container": w[4]}, "Network "+name+": "+w[3]+" completed.")
	}
	if len(w) == 4 && w[3] == "remove" {
		return engineAction("DELETE", "/networks/"+name, nil, "Network "+name+" removed.")
	}
	if len(w) == 4 && w[3] == "create" {
		if err := c.Validate(); err != nil {
			return "", err
		}
		n, ok := c.Networks[name]
		if !ok {
			return "", fmt.Errorf("network is not configured")
		}
		if (n.Driver == "macvlan" || n.Driver == "ipvlan") && n.Parent == "" {
			return "", fmt.Errorf("configure a container-role parent interface before creating this network")
		}
		options := map[string]string{}
		for key, value := range n.Options {
			options[key] = value
		}
		if n.Parent != "" {
			parent, err := kernelInterface(n.Parent)
			if err != nil {
				return "", err
			}
			options["parent"] = parent
		}
		labels := map[string]string{}
		for key, value := range n.Labels {
			labels[key] = value
		}
		labels["com.fractalcr.managed"] = "true"
		body := map[string]any{"Name": name, "Driver": n.Driver, "Internal": n.Internal, "Attachable": n.Attachable, "EnableIPv6": n.IPv6, "Labels": labels, "Options": options, "IPAM": n.IPAMAPI()}
		return engineAction("POST", "/networks/create", body, "Network "+name+" created.")
	}
	return "", fmt.Errorf("invalid network operation")
}

func networkDetail(name string) (string, error) {
	var n struct {
		Name, Driver, Scope  string
		Internal, Attachable bool
		Options              map[string]string
		IPAM                 struct {
			Config []struct{ Subnet, Gateway string }
		}
		Containers map[string]struct{ Name, IPv4Address string }
	}
	if err := engineJSON("GET", "/networks/"+name, nil, &n); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Network %s\n  Driver: %s\n  Scope: %s\n  Internal: %t\n  Attachable: %t\n", displayText(n.Name), displayText(n.Driver), displayText(n.Scope), n.Internal, n.Attachable)
	if parent := n.Options["parent"]; parent != "" {
		fmt.Fprintf(&b, "  Parent interface: %s\n", applianceInterface(parent))
	}
	for _, a := range n.IPAM.Config {
		fmt.Fprintf(&b, "  Subnet: %s, gateway: %s\n", displayText(a.Subnet), displayText(a.Gateway))
	}
	for _, id := range config.Keys(n.Containers) {
		a := n.Containers[id]
		fmt.Fprintf(&b, "  Container %s, address %s\n", displayText(a.Name), displayText(a.IPv4Address))
	}
	return b.String(), nil
}
