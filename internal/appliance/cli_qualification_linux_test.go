package appliance

import (
	"fmt"
	"fractalcr/internal/cli"
	"os"
	"testing"
	"time"
)

// Opt-in qualification against a real Engine. Creates only uniquely named
// test resources and removes them even if an assertion fails.
func TestCLIEngineQualification(t *testing.T) {
	if os.Getenv("FCR_CLI_ENGINE_QUALIFY") != "1" {
		t.Skip("requires disposable Engine resources")
	}
	name := fmt.Sprintf("fcr-cli-qual-%d", time.Now().UnixNano())
	front, back := name+"-front", name+"-back"
	s, err := cli.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	execute := func(commands ...string) {
		t.Helper()
		for _, command := range commands {
			if _, err := s.Execute(command); err != nil {
				t.Fatalf("%s: %v", command, err)
			}
		}
	}
	execute("conf t", "network "+front, "label com.fractalcr.qualification cli", "exit", "network "+back, "label com.fractalcr.qualification cli", "exit",
		"container "+name, "image debian:trixie-slim", "command sleep 60", "user 1000:1000", "guest-hostname workload-test", "cpus 0.25", "memory 64M", "pids-limit 64", "memory-shared 16M",
		"read-only", "capability drop all", "security no-new-privileges", "logging driver local", "logging option max-size 1m", "label com.fractalcr.qualification cli",
		"network attach "+front, "network attach "+front+" alias application", "network attach "+back, "tmpfs /tmp rw,size=8m", "ulimit nofile 1024 2048",
		"healthcheck command true", "healthcheck interval 1s", "healthcheck timeout 1s", "healthcheck retries 2", "end", "write memory")
	for _, network := range []string{front, back} {
		if _, err := networkOperation([]string{"request", "network", network, "create"}, s.Running); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := engineRequest("DELETE", "/networks/"+network, nil); err != nil {
				t.Error("cleanup network", err)
			}
		})
	}
	if _, err := createContainer(name, s.Running); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := engineRequest("DELETE", "/containers/"+name+"?force=true", nil); err != nil {
			t.Error("cleanup container", err)
		}
	})
	if _, err := engineRequest("POST", "/containers/"+name+"/start", nil); err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Config struct {
			User, Hostname string
			Labels         map[string]string
		}
		HostConfig struct {
			NanoCpus, Memory, ShmSize int64
			ReadonlyRootfs            bool
			LogConfig                 struct{ Type string }
			Ulimits                   []struct {
				Name       string
				Soft, Hard int64
			}
		}
		NetworkSettings struct {
			Networks map[string]struct{ Aliases []string }
		}
		State struct {
			Running bool
			Health  struct{ Status string }
		}
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if err := engineJSON("GET", "/containers/"+name+"/json", nil, &actual); err != nil {
			t.Fatal(err)
		}
		if actual.State.Health.Status == "healthy" || time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !actual.State.Running || actual.State.Health.Status != "healthy" {
		t.Fatal("configured healthcheck did not become healthy", actual.State)
	}
	if actual.Config.User != "1000:1000" || actual.Config.Hostname != "workload-test" || actual.Config.Labels["com.fractalcr.qualification"] != "cli" || actual.HostConfig.NanoCpus != 250000000 || actual.HostConfig.Memory != 64*1024*1024 || actual.HostConfig.ShmSize != 16*1024*1024 || !actual.HostConfig.ReadonlyRootfs || actual.HostConfig.LogConfig.Type != "local" || len(actual.NetworkSettings.Networks) != 2 {
		t.Fatalf("Engine did not receive CLI configuration: %+v", actual)
	}
	restored, err := cli.New(s.StateDir)
	if err != nil || string(restored.Running.JSON()) != string(s.Running.JSON()) {
		t.Fatal("configuration did not survive reload", err)
	}
	t.Log("CLI -> running/startup config -> real Engine: resources, identity, labels, logging, two networks, healthcheck, and lifecycle passed")
}
