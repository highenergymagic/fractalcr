package filedisplay

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestFormatsAndTerminalSafety(t *testing.T) {
	for _, format := range []string{"", "/ascii"} {
		got, err := Format([]byte("hostname ctr\n"), format)
		if err != nil || got != "hostname ctr\n" {
			t.Fatal(got, err)
		}
	}
	got, err := Format([]byte{0xc8, 0xc5, 0xd3, 0xd3, 0xd6, 0x25}, "/ebcdic")
	if err != nil || got != "HELLO\n" {
		t.Fatal(got, err)
	}
	got, err = Format([]byte("\x1b[2Jhello\x00\x07"), "/ascii")
	if err != nil || got != ".[2Jhello.." {
		t.Fatal(got, err)
	}
	got, err = Format([]byte{0, 1, 2, 3, 'A', 'B', 'C', 'D', 0xff}, "")
	if err != nil || got != "00000000:  00010203 41424344 FF                   .... ABCD .\n" {
		t.Fatal(got, err)
	}
	if _, err := Format(nil, "/invalid"); err == nil {
		t.Fatal("accepted invalid format")
	}
}

func TestCompressedIntegrityAndLimits(t *testing.T) {
	compressed := func(data []byte) []byte {
		var b bytes.Buffer
		w := gzip.NewWriter(&b)
		w.Write(data)
		w.Close()
		return b.Bytes()
	}
	good := compressed([]byte("hostname ctr\n"))
	got, err := Format(good, "/compressed")
	if err != nil || got != "hostname ctr\n" {
		t.Fatal(got, err)
	}
	for _, data := range [][]byte{good[:len(good)-1], []byte("not compressed"), compressed(bytes.Repeat([]byte("a"), Limit+1)), bytes.Repeat([]byte("a"), Limit+1)} {
		if got, err := Format(data, "/compressed"); err == nil || got != "" {
			t.Fatal("accepted invalid/oversized content", len(got), err)
		}
	}
	good[len(good)-8] ^= 1
	if _, err := Format(good, "/compressed"); err == nil {
		t.Fatal("ignored checksum mismatch")
	}
	if _, err := Format([]byte(strings.Repeat("x", Limit+1)), "/ascii"); err == nil {
		t.Fatal("unbounded plain input")
	}
}
