package appliance

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"fractalcr/internal/config"
	"golang.org/x/crypto/ssh"
	"net"
	"strings"
	"testing"
)

func cloudTestKey(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}
func TestCloudConfig(t *testing.T) {
	key := cloudTestKey(t)
	user := []byte(fmt.Sprintf("#cloud-config\nssh_pwauth: false\nusers:\n  - name: freyja\n    lock_passwd: true\n    ssh_authorized_keys: ['%s']\n", key))
	meta := []byte("instance-id: test-one\nlocal-hostname: cloud-router\n")
	mac, _ := net.ParseMAC("52:54:00:12:34:56")
	ports := []platformPort{{Name: "Ethernet0", Interface: net.Interface{HardwareAddr: mac}}}
	c, err := cloudConfig(meta, user, nil, ports)
	if err != nil {
		t.Fatal(err)
	}
	if c.Hostname != "cloud-router" || !c.AAA.Enabled || !c.AAA.SSH || c.AAA.Users["freyja"].Secret != "" || c.AAA.Users["freyja"].Privilege != 15 || !c.Network.DHCP {
		t.Fatal(c)
	}
	if config.CheckSecret(c.AAA.Users["freyja"].Secret, "") {
		t.Fatal("key-only account accepted empty password")
	}
	before := c.AAA.Users["freyja"].AuthVerifier()
	u := c.AAA.Users["freyja"]
	u.SSHKeys = []string{cloudTestKey(t)}
	if u.AuthVerifier() == before {
		t.Fatal("key rotation did not revoke sessions")
	}
	for _, bad := range []string{string(user) + "runcmd: [id]\n", strings.ReplaceAll(string(user), "ssh_pwauth: false", "ssh_pwauth: true"), strings.ReplaceAll(string(user), "lock_passwd: true", "lock_passwd: false"), string(user) + "---\n{}", strings.ReplaceAll(string(user), key, "not-a-key"), strings.ReplaceAll(string(user), key, "command=\"id\" "+key)} {
		if _, err = cloudConfig(meta, []byte(bad), nil, ports); err == nil {
			t.Fatal("unsafe provisioning accepted")
		}
	}
	ports = append(ports, platformPort{Name: "Ethernet1"})
	if _, err = cloudConfig(meta, user, nil, ports); err == nil {
		t.Fatal("ambiguous interface accepted")
	}
	network := []byte("version: 2\nethernets:\n  management:\n    match: {macaddress: '52:54:00:12:34:56'}\n    dhcp4: true\n")
	if _, err = cloudConfig(meta, user, network, ports); err != nil {
		t.Fatal(err)
	}
}
