package main

import (
	"bytes"
	appliancecli "fractalcr/internal/cli"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/cli/cli/command"
	clientconfig "github.com/docker/cli/cli/config"
)

func TestEngineConnectionFlagsAndSavedContext(t *testing.T) {
	for _, key := range []string{"DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_TLS", "DOCKER_TLS_VERIFY"} {
		t.Setenv(key, "")
	}
	oldDir := clientconfig.Dir()
	defer clientconfig.SetDir(oldDir)
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("API-Version", "1.47")
		if strings.HasSuffix(r.URL.Path, "/_ping") {
			w.Write([]byte("OK"))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/info") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"Name":"cli-qualification","OSType":"linux"}`))
			return
		}
		t.Errorf("unexpected API request: %s", r.URL.Path)
		w.WriteHeader(404)
	}))
	defer server.Close()
	host := "tcp://" + strings.TrimPrefix(server.URL, "http://")
	run := func(args ...string) string {
		t.Helper()
		var output bytes.Buffer
		client, err := command.NewDockerCli(command.WithInputStream(io.NopCloser(strings.NewReader(""))), command.WithOutputStream(&output), command.WithErrorStream(&output))
		if err != nil {
			t.Fatal(err)
		}
		root := engineRoot(client)
		root.SetOut(&output)
		root.SetErr(&output)
		root.SetArgs(append([]string{"--config", dir}, args...))
		if err = root.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output.String())
		}
		return output.String()
	}
	if output := run("--host", host, "info", "--format", "{{.Name}}"); !strings.Contains(output, "cli-qualification") {
		t.Fatal(output)
	}
	run("context", "create", "qualification", "--docker", "host="+host)
	run("context", "use", "qualification")
	if output := run("info", "--format", "{{.Name}}"); !strings.Contains(output, "cli-qualification") {
		t.Fatal("saved context ignored", output)
	}
	if output := run("--context", "qualification", "info", "--format", "{{.Name}}"); !strings.Contains(output, "cli-qualification") {
		t.Fatal("explicit context ignored", output)
	}
}

func TestNativeKeywordsReachEngineAPI(t *testing.T) {
	for _, key := range []string{"DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_TLS", "DOCKER_TLS_VERIFY"} {
		t.Setenv(key, "")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("API-Version", "1.47")
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Write([]byte("OK"))
		case strings.HasSuffix(r.URL.Path, "/info"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"Name":"native-keywords","OSType":"linux"}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	s, err := appliancecli.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	line := "request container-engine host tcp://" + strings.TrimPrefix(server.URL, "http://") + " info format '{{.Name}}'"
	words, _, err := s.Resolve(line)
	if err != nil {
		t.Fatal(err)
	}
	service, argv, ok := appliancecli.WorkloadArguments(words)
	if !ok || service != "engine" {
		t.Fatalf("dispatch: %s %v", service, argv)
	}
	var output bytes.Buffer
	client, err := command.NewDockerCli(command.WithInputStream(io.NopCloser(strings.NewReader(""))), command.WithOutputStream(&output), command.WithErrorStream(&output))
	if err != nil {
		t.Fatal(err)
	}
	root := engineRoot(client)
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(argv)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != "native-keywords" {
		t.Fatal(output.String())
	}
}

func TestWorkloadCatalogUsesLinkedCommands(t *testing.T) {
	client, err := command.NewDockerCli()
	if err != nil {
		t.Fatal(err)
	}
	root := engineRoot(client)
	model := catalog(root)
	names := map[string]bool{}
	for _, child := range model.Children {
		names[child.Name] = true
	}
	for _, name := range []string{"compose", "buildx", "build", "bake", "service", "swarm", "context"} {
		if !names[name] {
			t.Fatal("missing workload family", name)
		}
	}
	if names["completion"] {
		t.Fatal("shell completion affordance exposed")
	}
	for _, name := range []string{"build", "bake"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd.Flags().Lookup("builder") == nil {
			t.Fatal("linked build alias lost builder selection", name, err)
		}
	}
	for _, path := range [][]string{{"image", "build"}, {"builder", "build"}} {
		cmd, _, err := root.Find(path)
		if err != nil || cmd.Flags().Lookup("builder") == nil {
			t.Fatal("legacy build alias still bypasses linked builder", path, err)
		}
	}
}
