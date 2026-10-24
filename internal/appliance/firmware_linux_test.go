package appliance

import (
	"encoding/binary"
	"encoding/json"
	"fractalcr/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplianceFilesConfinePathsAndPreserveDestinations(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	old := fileSystems
	fileSystems = func() []applianceFS { return []applianceFS{{"data0", root, false}} }
	defer func() { fileSystems = old }()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"data0:../secret", "data0:/../secret", "data0:escape/secret"} {
		if _, err := fileOperation([]string{"delete", p}); err == nil {
			t.Fatal("escaped root", p)
		}
	}
	if _, err := copyFile([]string{"copy", "running-config", "data0:config.bak"}, config.Default()); err != nil {
		t.Fatal(err)
	}
	if _, err := copyFile([]string{"copy", "running-config", "data0:config.bak"}, config.Default()); err == nil {
		t.Fatal("overwrote existing destination")
	}
	if _, err := fileOperation([]string{"move", "data0:config.bak", "data0:saved.bak"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fileOperation([]string{"dir", "data0:"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fileOperation([]string{"delete", "data0:saved.bak"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "secret")); err != nil {
		t.Fatal("outside file affected", err)
	}
	if _, err := fileOperation([]string{"delete", "data0:"}); err == nil {
		t.Fatal("deleted filesystem root")
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".transfer-") {
			t.Fatal("leaked transfer")
		}
	}
}
func TestEngineUsesTypedRequestsAndFormatsOwnOutput(t *testing.T) {
	old := engineClient
	defer func() { engineClient = old }()
	var created map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.47/containers/json":
			w.Write([]byte(`[{"Names":["/web"],"Image":"test:1","State":"running","Ports":[]}]`))
		case "/v1.47/containers/create":
			if r.Method != "POST" || r.URL.Query().Get("name") != "web" {
				t.Error("bad create request")
			}
			_ = json.NewDecoder(r.Body).Decode(&created)
			w.WriteHeader(201)
			w.Write([]byte(`{"Id":"abc"}`))
		case "/v1.47/containers/web/start":
			w.WriteHeader(204)
		default:
			w.WriteHeader(500)
			w.Write([]byte(`{"message":"failure at /var/lib/docker/private"}`))
		}
	}))
	defer server.Close()
	engineClient = &http.Client{Transport: rewriteEngineURL{server.URL, http.DefaultTransport}}
	out, err := engineInventory("containers")
	if err != nil || !strings.Contains(out, "web") || strings.Contains(out, "CONTAINER ID") {
		t.Fatal(out, err)
	}
	c := config.Default()
	c.Containers["web"] = config.Container{Isolation: "inherit", Image: "test:1", MemoryMiB: 128, ReadOnly: true, NoNewPrivileges: true, DropAllCapabilities: true, Publish: []string{"127.0.0.1:8080:80"}, Volumes: []config.Volume{{Name: "data", Target: "/data", ReadOnly: true}}}
	if _, err = createContainer("web", c); err != nil {
		t.Fatal(err)
	}
	host := created["HostConfig"].(map[string]any)
	if host["Memory"] != float64(128*1024*1024) || host["ReadonlyRootfs"] != true || host["Runtime"] != "runc" {
		t.Fatal(host)
	}
	if _, err = engineAction("POST", "/containers/web/start", nil, "Started"); err != nil {
		t.Fatal(err)
	}
	_, err = engineRequest("GET", "/bad", nil)
	if err == nil || strings.Contains(err.Error(), "/var/") {
		t.Fatal("backend error exposed", err)
	}
}

type rewriteEngineURL struct {
	base      string
	transport http.RoundTripper
}

func (r rewriteEngineURL) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.URL.Host = strings.TrimPrefix(r.base, "http://")
	return r.transport.RoundTrip(copy)
}
func TestProtocolEncodingAndDisplay(t *testing.T) {
	a := nlAttribute(4, []byte{1, 2, 3})
	if len(a) != 8 || binary.NativeEndian.Uint16(a) != 7 || binary.NativeEndian.Uint16(a[2:]) != 4 {
		t.Fatal(a)
	}
	if speedClass(1000) != "GigabitEthernet" || speedClass(10000) != "TenGigabitEthernet" || speedClass(-1) != "Ethernet" {
		t.Fatal("speed class")
	}
	if displayText("test\x1b[2J\r") != "test [2J " {
		t.Fatal("terminal control not sanitized")
	}
	data := []byte{8, 0, 0, 0, 1, 2, 3, 4}
	binary.BigEndian.PutUint16(data[2:], icmpChecksum(data))
	if icmpChecksum(data) != 0 {
		t.Fatal("ICMP checksum")
	}
}

func TestLiveTimeProtocol(t *testing.T) {
	if os.Getenv("FCR_TEST_CHRONY") != "1" {
		t.Skip("requires isolated chronyd fixture")
	}
	status, err := ntpStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "Clock is ") {
		t.Fatal(status)
	}
	associations, err := ntpAssociations()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(associations, "sys.peer") {
		t.Fatal(associations)
	}
}
