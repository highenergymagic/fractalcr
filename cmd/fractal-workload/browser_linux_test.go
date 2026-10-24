package main

import (
	"bytes"
	"github.com/pkg/browser"
	"testing"
)

func TestBrowserActionsPrintLinksWithoutHostHelpers(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	previous := browser.Stdout
	defer func() { browser.Stdout = previous }()
	var out bytes.Buffer
	browser.Stdout = &out
	for _, link := range []string{"https://example.invalid/login", "docker-desktop://dashboard/build/record", "http://127.0.0.1:8080/trace"} {
		out.Reset()
		if err := browser.OpenURL(link); err != nil {
			t.Fatal(err)
		}
		if out.String() != link+"\n" {
			t.Fatal(out.String())
		}
	}
	if err := browser.OpenURL("https://example.invalid/\x1b[2J"); err == nil {
		t.Fatal("accepted terminal control in link")
	}
}
