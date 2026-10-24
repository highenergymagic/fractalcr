package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestIOSAbbreviationsAndErrors(t *testing.T) {
	s := session(t)
	for _, tc := range []struct {
		input, canonical string
		local            bool
	}{
		{"sh ver", "show version", true}, {"SH VER", "show version", true},
		{"show platform", "show platform", true}, {"sh plat soft serv", "show platform software service", true},
		{"conf t", "configure terminal", false}, {"wr mem", "write memory", false},
		{"req plat soft serv container-engine rest", "request platform software service container-engine restart", true},
	} {
		words, local, err := s.Resolve(tc.input)
		if err != nil || Join(words) != tc.canonical || local != tc.local {
			t.Fatalf("%q: %v %v %v", tc.input, words, local, err)
		}
	}
	for _, tc := range []struct {
		input, kind string
		column      int
	}{
		{"sh", "incomplete", 2}, {"sh c", "ambiguous", 3}, {"show nonsense", "invalid", 5},
		{"show version extra", "invalid", 13}, {"show container", "incomplete", 14},
	} {
		_, _, err := s.Resolve(tc.input)
		var syntax *SyntaxError
		if !errors.As(err, &syntax) || syntax.Kind != tc.kind || syntax.Column != tc.column {
			t.Fatalf("%q: %#v", tc.input, err)
		}
	}
}

func TestNativeArgumentsAreNotKeywordsOrShellText(t *testing.T) {
	s := session(t)
	input := `req container-engine run env 'Mixed=Keep Case' label 'literal=$(id);*' arguments Image:TAG /bin/echo ''`
	got, local, err := s.Resolve(input)
	want := []string{"request", "container-engine", "run", "env", "Mixed=Keep Case", "label", "literal=$(id);*", "arguments", "Image:TAG", "/bin/echo", ""}
	if err != nil || local || !reflect.DeepEqual(got, want) {
		t.Fatalf("%q %v %v", got, local, err)
	}
}

func TestIOSHelpIsOneLevelAndWhitespaceSensitive(t *testing.T) {
	s := session(t)
	if got := s.ContextHelp("sh"); got != "show" {
		t.Fatal(got)
	}
	root := s.ContextHelp("")
	if strings.Contains(root, "version") || strings.Contains(root, "hostname") || strings.Contains(root, "Use ?") {
		t.Fatal(root)
	}
	show := s.ContextHelp("show ")
	if !strings.Contains(show, "version") || !strings.Contains(show, "platform") || strings.Contains(show, "software") || strings.Contains(show, "<cr>") {
		t.Fatal(show)
	}
	platform := s.ContextHelp("sh plat ")
	if !strings.Contains(platform, "software") || !strings.Contains(platform, "<cr>") || strings.Contains(platform, "service ") {
		t.Fatal(platform)
	}
	if got := s.ContextHelp("show ver"); got != "version" {
		t.Fatal(got)
	}
	if got := s.ContextHelp("show version "); !strings.Contains(got, "<cr>") || !strings.Contains(got, "|") || strings.Contains(got, "include") {
		t.Fatal(got)
	}
	if got := s.ContextHelp("show gibberish "); got != "" {
		t.Fatal(got)
	}
	if got := s.ContextHelp("show container "); !strings.Contains(got, "<NAME>") {
		t.Fatal(got)
	}
}

func TestIOSCompletionOnlyCompletesCurrentUniqueWord(t *testing.T) {
	s := session(t)
	for _, tc := range []struct{ in, out string }{
		{"sh", "show "}, {"sh ver", "sh version "}, {"show plat", "show platform "},
		{"show c", "show c"}, {"show ", "show "}, {"bogus", "bogus"},
	} {
		got, help := s.Complete(tc.in)
		if got != tc.out || (help != "" && help != "\a") {
			t.Fatalf("%q: %q %q", tc.in, got, help)
		}
	}
}

func TestIOSModeAndValueSemantics(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "cont WebApp", "env MixedCase \"literal ? and $(command)\"")
	c := s.Candidate.Containers["WebApp"]
	if c.Environment["MixedCase"] != "literal ? and $(command)" {
		t.Fatal(c)
	}
	if _, _, err := s.Resolve("sh ver"); err == nil {
		t.Fatal("EXEC command accepted without do")
	}
	words, local, err := s.Resolve("do sh ver")
	if err != nil || !local || Join(words) != "do show version" {
		t.Fatal(words, local, err)
	}
	if help := s.ContextHelp("do sh "); !strings.Contains(help, "version") || strings.Contains(help, "hostname") {
		t.Fatal(help)
	}
	if help := s.ContextHelp(""); !strings.Contains(help, "environment") || strings.Contains(help, "reload") {
		t.Fatal(help)
	}
	for _, word := range []string{"", "a b", `a\b`, `a"b`, "'", "a?b"} {
		want := []string{"command", word}
		got, err := Words(Join(want))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal(want, got, err)
		}
	}
}

func FuzzResolveAndHelp(f *testing.F) {
	for _, s := range []string{"sh ver", "show ?", "do sh", "command \"\"", "request compose --file x up", "request container-engine service create --replicas 3 image", "request buildkit debug workers", "\x1b", strings.Repeat("a", 2048)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, line string) {
		s := &Session{}
		s.Resolve(line)
		s.ContextHelp(line)
		s.Complete(line)
	})
}
