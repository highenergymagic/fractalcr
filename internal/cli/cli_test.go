package cli

import (
	"errors"
	"fractalcr/internal/config"
	"reflect"
	"strings"
	"testing"
)

func session(t *testing.T) *Session {
	t.Helper()
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func execute(t *testing.T, s *Session, commands ...string) {
	t.Helper()
	for _, c := range commands {
		if _, e := s.Execute(c); e != nil {
			t.Fatalf("%s: %v", c, e)
		}
	}
}
func TestStagingAndPersistence(t *testing.T) {
	s := session(t)
	execute(t, s, "configure candidate", "hostname edge", "interface eth0", "ip address 192.0.2.2/24", "ip gateway 192.0.2.1", "exit")
	if s.Running.Hostname != "ctr" {
		t.Fatal("candidate changed running state")
	}
	s.Apply = func(a, b config.Config) error {
		if b.Network.Address != "192.0.2.2/24" {
			t.Fatal(b)
		}
		return errors.New("platform failure")
	}
	if _, e := s.Execute("commit"); e == nil || s.Running.Hostname != "ctr" {
		t.Fatal("failed apply committed")
	}
	s.Apply = nil
	execute(t, s, "commit", "end", "write memory")
	reloaded, e := New(s.StateDir)
	if e != nil || reloaded.Running.Hostname != "edge" {
		t.Fatal(e)
	}
	execute(t, s, "configure candidate", "hostname discarded", "abort")
	if s.Running.Hostname != "edge" || s.Candidate.Hostname != "edge" {
		t.Fatal("abort changed running")
	}
}
func TestRuntimeInheritance(t *testing.T) {
	s := session(t)
	execute(t, s, "config", "system container-isolation install runtime kata-containers", "system container-isolation enable",
		"system container-isolation kata-containers apply-to all-containers", "container native", "no container-isolation", "exit",
		"container inherited", "exit", "container explicit", "container-isolation kata-containers enable", "commit")
	if s.Running.EffectiveRuntime("native") != "runc" || s.Running.EffectiveRuntime("inherited") != "io.containerd.kata.v2" || s.Running.EffectiveRuntime("explicit") != "io.containerd.kata.v2" {
		t.Fatal("runtime precedence")
	}
	execute(t, s, "exit", "system container-isolation kata-containers apply-to explicit", "commit")
	if s.Running.EffectiveRuntime("inherited") != "runc" || s.Running.EffectiveRuntime("explicit") != "io.containerd.kata.v2" {
		t.Fatal("explicit precedence")
	}
}
func TestContainerArguments(t *testing.T) {
	s := session(t)
	execute(t, s, "config", "container app", "image example/app:1", `command "/path with space" "literal $(touch /x)"`, `environment MESSAGE "hello world"`, "publish 8080:80/tcp", "restart-policy unless-stopped", "commit")
	c := s.Running.Containers["app"]
	if c.Command[1] != "literal $(touch /x)" || c.Environment["MESSAGE"] != "hello world" {
		t.Fatal(c)
	}
	if _, e := s.Execute("command 'unclosed"); e == nil {
		t.Fatal("unclosed quote accepted")
	}
	execute(t, s, "no environment MESSAGE", "no publish", "no command", "commit")
	c = s.Running.Containers["app"]
	if len(c.Command)+len(c.Publish)+len(c.Environment) != 0 {
		t.Fatal(c)
	}
}
func TestStorageNoImplicitAction(t *testing.T) {
	s := session(t)
	execute(t, s, "config", "storage datastore name data0", "preset raid1", "disk vdb")
	if len(s.Running.Datastores["data0"].Disks) != 1 {
		t.Fatal("incomplete definition was not retained")
	}
	execute(t, s, "disk add vdc", "commit")
	if !reflect.DeepEqual(s.Running.Datastores["data0"].Disks, []string{"vdb", "vdc"}) {
		t.Fatal("disk list")
	}
}
func TestWords(t *testing.T) {
	w, e := Words(`environment X "a b"`)
	if e != nil || strings.Join(w, "|") != "environment|X|a b" {
		t.Fatal(w, e)
	}
	w, e = Words(`command "" 'literal *'`)
	if e != nil || !reflect.DeepEqual(w, []string{"command", "", "literal *"}) {
		t.Fatal(w, e)
	}
}

func TestDeletingCurrentObject(t *testing.T) {
	s := session(t)
	execute(t, s, "config", "interface eth0", "ip address 192.0.2.1/24", "no interface eth0")
	if s.Mode != "config" || s.Candidate.Network != nil {
		t.Fatal("stale interface context")
	}
	if _, err := s.Execute("ip address 192.0.2.2/24"); err == nil {
		t.Fatal("accepted interface command without interface")
	}
	execute(t, s, "container app", "no container app")
	if s.Mode != "config" {
		t.Fatal("stale container context")
	}
}
