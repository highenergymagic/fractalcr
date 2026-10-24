package cli

import (
	"strings"
	"testing"
)

func TestOutputRedirectionGrammar(t *testing.T) {
	s := session(t)
	for _, cmd := range []string{"show version | redirect data0:version", "show clock | append data0:log", "show clock | tee /append data0:log"} {
		w, _, err := s.Resolve(cmd)
		if err != nil {
			t.Fatal(cmd, err)
		}
		_, f, err := SplitFilter(w)
		if err != nil || !f.Redirect() {
			t.Fatal(f, err)
		}
	}
	s.EnforcePrivilege = true
	s.Privilege = 1
	if _, _, err := s.Resolve("show clock | redirect data0:log"); err == nil {
		t.Fatal("user EXEC can redirect output")
	}
	if strings.Contains(s.ContextHelp("show clock | "), "redirect") {
		t.Fatal("privileged redirection shown in user EXEC")
	}
}

func TestOutputFiltersShareGrammar(t *testing.T) {
	s := session(t)
	for _, tt := range []struct{ kind, want string }{{"include", "two"}, {"exclude", "one\nthree"}, {"begin", "two\nthree"}, {"count", "Number of lines which match regexp = 1"}} {
		w, _, e := s.Resolve("sh ver | " + tt.kind + " ^two$")
		if e != nil {
			t.Fatal(e)
		}
		base, f, e := SplitFilter(w)
		if e != nil || strings.Join(base, " ") != "show version" {
			t.Fatal(e, base)
		}
		if got := f.Apply("one\ntwo\nthree\n"); got != tt.want {
			t.Fatal(got, tt.want)
		}
	}
	w, _, _ := s.Resolve("show version | include [")
	if _, _, e := SplitFilter(w); e == nil {
		t.Fatal("invalid expression")
	}
	if got := s.ContextHelp("show version | "); !strings.Contains(got, "include") || strings.Contains(got, "<cr>") {
		t.Fatal(got)
	}
	if got, _ := s.Complete("sh ver | inc"); got != "sh ver | include " {
		t.Fatal(got)
	}
	s.EnforcePrivilege = true
	s.Privilege = 1
	if _, _, e := s.Resolve("show running-config | include secret"); e == nil {
		t.Fatal("filter bypassed authorization")
	}
}

func TestIOSSectionFilters(t *testing.T) {
	s := session(t)
	text := "hostname box\ninterface Ethernet0\n ip address 192.0.2.1 255.255.255.0\n shutdown\n!\nline console 0\n login local\n!\nend\n"
	for _, kind := range []string{"section", "section include", "section exclude"} {
		words, _, err := s.Resolve("show running-config | " + kind + " ^interface")
		if err != nil {
			t.Fatal(err)
		}
		_, filter, err := SplitFilter(words)
		if err != nil {
			t.Fatal(err)
		}
		out := filter.Apply(text)
		if kind == "section exclude" {
			if strings.Contains(out, "192.0.2.1") || !strings.Contains(out, "hostname box") {
				t.Fatal(out)
			}
		} else if !strings.Contains(out, "192.0.2.1") || strings.Contains(out, "console") || strings.Contains(out, "hostname") {
			t.Fatal(out)
		}
	}
}
