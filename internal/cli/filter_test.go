package cli

import (
	"strings"
	"testing"
)

func TestOutputFiltersShareGrammar(t *testing.T) {
	s := session(t)
	for _, tt := range []struct{ kind, want string }{{"include", "two"}, {"exclude", "one\nthree"}, {"begin", "two\nthree"}, {"count", "1"}} {
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
