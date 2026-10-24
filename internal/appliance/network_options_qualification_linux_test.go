package appliance

import (
	"fractalcr/internal/config"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// Functional networking qualification uses its own fresh network namespace.
// It never changes the host management link or routing table.
func TestNetworkOptionsQualification(t *testing.T) {
	if os.Getenv("FCR_NETWORK_QUALIFY") != "1" {
		t.Skip("requires root and a disposable network namespace")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	original, err := os.Open("/proc/self/task/" + strconv.Itoa(unix.Gettid()) + "/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	if err = unix.Unshare(unix.CLONE_NEWNET); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := unix.Setns(int(original.Fd()), unix.CLONE_NEWNET); err != nil {
			t.Fatal("restore test network namespace", err)
		}
	}()
	old := resolveLink
	resolveLink = net.InterfaceByName
	defer func() { resolveLink = old }()
	payload := make([]byte, 16)
	payload = append(payload, nlAttribute(syscall.IFLA_IFNAME, []byte("qualification0\x00"))...)
	payload = append(payload, nlAttribute(18|0x8000, nlAttribute(1, []byte("dummy\x00")))...)
	if err = routeRequest(syscall.RTM_NEWLINK, syscall.NLM_F_CREATE|syscall.NLM_F_EXCL, payload); err != nil {
		t.Fatal(err)
	}
	if err = setInterfaceMTU("qualification0", 9000); err != nil {
		t.Fatal(err)
	}
	if err = setInterfaceState("qualification0", true); err != nil {
		t.Fatal(err)
	}
	n := config.Network{Interface: "qualification0", Role: "container", Address: "192.0.2.2/24", SecondaryAddresses: []string{"198.51.100.2/24"}, IPv6Addresses: []string{"2001:db8::2/64"}, Routes: map[string]config.StaticRoute{"203.0.113.0/24": {Gateway: "192.0.2.1", Metric: 100}, "2001:db8:1::/64": {Gateway: "2001:db8::1", Metric: 100}}}
	if err = applyAddress(n); err != nil {
		t.Fatal(err)
	}
	if err = interfaceAddresses(n, false); err != nil {
		t.Fatal(err)
	}
	iface, err := net.InterfaceByName(n.Interface)
	if err != nil || iface.MTU != 9000 {
		t.Fatal(iface, err)
	}
	addresses, err := iface.Addrs()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, address := range addresses {
		found[address.String()] = true
	}
	for _, address := range []string{n.Address, n.SecondaryAddresses[0], n.IPv6Addresses[0]} {
		if !found[address] {
			t.Fatal("missing address", address)
		}
	}
	for _, family := range []int{syscall.AF_INET, syscall.AF_INET6} {
		routes, err := showRoutesFamily(family)
		if err != nil {
			t.Fatal(err)
		}
		expected := "203.0.113.0/24"
		if family == syscall.AF_INET6 {
			expected = "2001:db8:1::/64"
		}
		if !strings.Contains(routes, expected) {
			t.Fatal(routes)
		}
	}
	removeAddress(n)
	if err = interfaceAddresses(n, true); err != nil {
		t.Fatal(err)
	}
	addresses, err = iface.Addrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range addresses {
		if address.String() == n.Address || address.String() == n.IPv6Addresses[0] || address.String() == n.SecondaryAddresses[0] {
			t.Fatal("address removal failed", address)
		}
	}
	t.Log("Native MTU, primary/secondary IPv4, IPv6, static routes and cleanup passed in an isolated network namespace")
	testVirtualLinks(t)
}

func testVirtualLinks(t *testing.T) {
	resolveLink = func(name string) (*net.Interface, error) {
		if _, kind, kernel, _ := config.VirtualInterface(name); kind != "" {
			name = kernel
		}
		return net.InterfaceByName(name)
	}
	c := config.Config{Interfaces: map[string]*config.Network{
		"qualification0": {Interface: "qualification0", Role: "container"},
		"Port-channel1":  {Interface: "Port-channel1", Role: "container", Members: []string{"qualification0"}, BondMode: "active-backup"},
		"Vlan100":        {Interface: "Vlan100", Role: "container", Parent: "Port-channel1"},
	}}
	if err := ensureVirtualInterfaces(c); err != nil {
		t.Fatal(err)
	}
	defer removeVirtualInterfaces(c)
	for name, want := range map[string]string{"Port-channel1": "bond", "Vlan100": "vlan"} {
		iface, err := resolveLink(name)
		if err != nil {
			t.Fatal(err)
		}
		kind, alias, err := linkIdentity(iface.Index)
		if err != nil || kind != want || alias != "fractalcr:"+name {
			t.Fatalf("link %s: %s %s %v", name, kind, alias, err)
		}
	}
	if err := setInterfaceState("Port-channel1", true); err != nil {
		t.Fatal(err)
	}
	if err := setInterfaceState("Vlan100", true); err != nil {
		t.Fatal(err)
	}
	if err := addressRequest("Vlan100", "192.0.2.5/24", false); err != nil {
		t.Fatal(err)
	}
	vlan, _ := resolveLink("Vlan100")
	addresses, err := vlan.Addrs()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, address := range addresses {
		if address.String() == "192.0.2.5/24" {
			found = true
		}
	}
	if !found {
		t.Fatal("VLAN address missing")
	}
	removeVirtualInterfaces(c)
	for _, name := range []string{"fcrbond1", "fcrvl100"} {
		if _, err := net.InterfaceByName(name); err == nil {
			t.Fatal("virtual interface leaked", name)
		}
	}
	if _, err := net.InterfaceByName("qualification0"); err != nil {
		t.Fatal("member interface removed", err)
	}
	t.Log("Native bond, VLAN, addressing and reverse-order cleanup passed")
}
