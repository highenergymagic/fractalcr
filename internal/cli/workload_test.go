package cli

import (
	"fractalcr/internal/workloadcatalog"
	"reflect"
	"strings"
	"testing"
)

func TestApplianceWorkloadKeywords(t *testing.T) {
	s := session(t)
	cases := []struct {
		line, service string
		args          []string
	}{
		{"request service create name web replicas 3 nginx", "engine", []string{"service", "create", "--name=web", "--replicas=3", "--", "nginx"}},
		{"request compose file data0:compose.yaml up web detach enable", "engine", []string{"compose", "--file=data0:compose.yaml", "up", "--detach=true", "--", "web"}},
		{"request container-engine run memory 256M interactive enable tty disable argument alpine arguments echo --help", "engine", []string{"run", "--memory=256M", "--interactive=true", "--tty=false", "--", "alpine", "echo", "--help"}},
		{"request container-engine client-context lab container ls all enable", "engine", []string{"--context=lab", "container", "ls", "--all=true"}},
		{"request buildkit debug workers addr unix:///run/buildkit/buildkitd.sock timeout 10", "build", []string{"debug", "workers", "--addr=unix:///run/buildkit/buildkitd.sock", "--timeout=10"}},
		{"request buildx build platform linux/amd64 load enable data0:", "engine", []string{"buildx", "build", "--platform=linux/amd64", "--load=true", "--", "data0:"}},
	}
	for _, tc := range cases {
		words, _, err := s.Resolve(tc.line)
		if err != nil {
			t.Fatal(tc.line, err)
		}
		if strings.Contains(Join(words), "--") && !strings.Contains(tc.line, "arguments") {
			t.Fatal("native flags leaked into resolved appliance grammar", words)
		}
		service, args, ok := WorkloadArguments(words)
		if !ok || service != tc.service || !reflect.DeepEqual(args, tc.args) {
			t.Fatalf("%s: %s %#v", tc.line, service, args)
		}
	}
}

func TestWorkloadPrimitiveValidation(t *testing.T) {
	for _, tc := range []struct {
		kind, value string
		valid       bool
	}{
		{"int", "-1", true}, {"int", "one", false},
		{"uint64", "-1", false}, {"uint16", "65536", false},
		{"uint16", "65535", true}, {"float", "NaN", false},
		{"float", "+Inf", false}, {"float", "0.5", true},
		{"duration", "5s", true}, {"duration", "five", false},
		{"ip", "::1", true}, {"ip", "999.1.1.1", false},
	} {
		if got := validWorkloadValue(workloadcatalog.Flag{Type: tc.kind}, tc.value); got != tc.valid {
			t.Errorf("%s %q: %v", tc.kind, tc.value, got)
		}
	}
	s := session(t)
	for _, line := range []string{"request service create replicas -1 nginx", "request service create replicas many nginx"} {
		if _, _, err := s.Resolve(line); err == nil {
			t.Errorf("accepted %s", line)
		}
	}
}

func TestCatalogDescriptionsUseNativeKeywords(t *testing.T) {
	var visit func(*workloadcatalog.Command)
	visit = func(c *workloadcatalog.Command) {
		for _, f := range c.Flags {
			if text := nativeDescription(c, f.Description); strings.Contains(text, "--") {
				t.Errorf("%s %s: %s", c.Name, f.Name, text)
			}
		}
		for i := range c.Children {
			visit(&c.Children[i])
		}
	}
	visit(&workloadRoot)
	visit(&buildkitRoot)
}
func TestWorkloadKeywordHelpAndCompletion(t *testing.T) {
	s := session(t)
	for _, tc := range []struct{ line, want string }{
		{"request service create ", "replicas"}, {"request compose up web ", "detach"},
		{"request container-engine run memory ", "<bytes>"}, {"request container-engine run tty ", "enable"},
		{"request container-engine run tty d", "disable"}, {"request buildkit build tlsca", "tlscacert"},
		{"request buildx history inspect ", "attachment"},
	} {
		h := s.ContextHelp(tc.line)
		if !strings.Contains(h, tc.want) || strings.Contains(h, "--") {
			t.Fatalf("%s: %s", tc.line, h)
		}
	}
	if h := s.ContextHelp("request container-engine run "); strings.Contains(h, "<cr>") {
		t.Fatal(h)
	}
	if h := s.ContextHelp("request container-engine cp argument a argument b "); !strings.Contains(h, "<cr>") || strings.Contains(h, "<ARG>") {
		t.Fatal(h)
	}
	for _, line := range []string{"request compose up help", "request buildx build help", "request container-engine run help"} {
		w, _, err := s.Resolve(line)
		if err != nil {
			t.Fatal(err)
		}
		service, _, ok := WorkloadArguments(w)
		if !ok || service != "help" {
			t.Fatal("help would invoke upstream client", w)
		}
	}
}
func TestWorkloadRejectsUnixOptionsAndIncompleteGrammar(t *testing.T) {
	s := session(t)
	for _, line := range []string{
		"request container-engine run --memory 256M alpine", "request container-engine run -it alpine",
		"request compose up web --detach", "request buildkit --help", "request container-engine run -- alpine",
		"request container-engine run memory", "request container-engine run tty true alpine", "request container-engine run tty enable",
		"request container-engine exec app", "request container-engine cp app:/file", "request compose version extra",
		"request container-engine run argument", "request container-engine run arguments", "request buildx history inspect a b",
	} {
		if _, _, err := s.Resolve(line); err == nil {
			t.Fatal("accepted", line)
		}
	}
}
func TestLiteralOperandsCannotInjectClientOptions(t *testing.T) {
	s := session(t)
	w, _, err := s.Resolve("request container-engine inspect argument --help")
	if err != nil {
		t.Fatal(err)
	}
	_, args, _ := WorkloadArguments(w)
	if !reflect.DeepEqual(args, []string{"inspect", "--", "--help"}) {
		t.Fatal(args)
	}
	w, _, err = s.Resolve("request container-engine run name --help alpine")
	if err != nil {
		t.Fatal(err)
	}
	_, args, _ = WorkloadArguments(w)
	if !reflect.DeepEqual(args, []string{"run", "--name=--help", "--", "alpine"}) {
		t.Fatal(args)
	}
}
func TestWorkloadAbbreviationsAndMode(t *testing.T) {
	s := session(t)
	w, _, err := s.Resolve("req ser cre replicas 3 nginx")
	if err != nil {
		t.Fatal(err)
	}
	_, args, _ := WorkloadArguments(w)
	if !reflect.DeepEqual(args, []string{"service", "create", "--replicas=3", "--", "nginx"}) {
		t.Fatal(args)
	}
	s.Mode = "config"
	if _, _, err = s.Resolve("do request compose version"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Resolve("request compose version"); err == nil {
		t.Fatal("EXEC accepted in config mode")
	}
}
