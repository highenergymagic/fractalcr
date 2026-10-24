// Package filedisplay implements appliance file display without host utilities.
package filedisplay

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"strings"
)

// Limit applies to both the input and decompressed text. The terminal protocol
// transfers bounded responses; oversized files fail rather than appear complete.
const Limit = 256 * 1024

func Format(data []byte, format string) (string, error) {
	if len(data) > Limit {
		return "", fmt.Errorf("File exceeds display limit")
	}
	if format == "/compressed" {
		var r io.ReadCloser
		var err error
		if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
			r, err = gzip.NewReader(bytes.NewReader(data))
		} else {
			r, err = zlib.NewReader(bytes.NewReader(data))
		}
		if err != nil {
			return "", fmt.Errorf("Invalid compressed file")
		}
		defer r.Close()
		data, err = io.ReadAll(io.LimitReader(r, Limit+1))
		if err != nil {
			return "", fmt.Errorf("Invalid compressed file")
		}
		if len(data) > Limit {
			return "", fmt.Errorf("File exceeds display limit")
		}
		format = "/ascii"
	}
	if format == "/ebcdic" {
		translated := make([]byte, len(data))
		for i, b := range data {
			translated[i] = ebcdic037[b]
		}
		data, format = translated, "/ascii"
	}
	if format == "" {
		format = "/ascii"
		for _, b := range data {
			if (b < 32 && b != '\r' && b != '\n' && b != '\t' && b != '\f') || b > 126 {
				format = "/binary"
				break
			}
		}
	}
	switch format {
	case "/ascii":
		// File content cannot inject terminal escape sequences or control a
		// session. Preserve printable ASCII and ordinary line delimiters.
		var out strings.Builder
		for _, b := range data {
			if (b >= 32 && b <= 126) || b == '\n' || b == '\r' || b == '\t' {
				out.WriteByte(b)
			} else {
				out.WriteByte('.')
			}
		}
		return out.String(), nil
	case "/binary":
		var out strings.Builder
		for offset := 0; offset < len(data); offset += 16 {
			row := data[offset:min(offset+16, len(data))]
			fmt.Fprintf(&out, "%08X:  ", offset)
			for i := 0; i < 16; i++ {
				if i > 0 && i%4 == 0 {
					out.WriteByte(' ')
				}
				if i < len(row) {
					fmt.Fprintf(&out, "%02X", row[i])
				} else {
					out.WriteString("  ")
				}
			}
			out.WriteString("    ")
			for i, b := range row {
				if i > 0 && i%4 == 0 {
					out.WriteByte(' ')
				}
				if b >= 32 && b <= 126 {
					out.WriteByte(b)
				} else {
					out.WriteByte('.')
				}
			}
			out.WriteByte('\n')
		}
		return out.String(), nil
	default:
		return "", fmt.Errorf("Invalid display format")
	}
}
