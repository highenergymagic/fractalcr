package appliance

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"os"
	"testing"
)

func TestSparseBlockSeed(t *testing.T) {
	pack := func(records ...[]byte) []byte {
		var raw, packed bytes.Buffer
		raw.WriteString("FCRBLK01")
		_ = binary.Write(&raw, binary.LittleEndian, uint64(blockfileSize))
		for _, r := range records {
			raw.Write(r)
		}
		z := gzip.NewWriter(&packed)
		_, _ = z.Write(raw.Bytes())
		_ = z.Close()
		return packed.Bytes()
	}
	extent := func(offset, length uint64, data string) []byte {
		var b bytes.Buffer
		_ = binary.Write(&b, binary.LittleEndian, offset)
		_ = binary.Write(&b, binary.LittleEndian, length)
		b.WriteString(data)
		return b.Bytes()
	}
	for _, tc := range []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"valid", pack(extent(4096, 4, "test"), make([]byte, 16)), true},
		{"overflow", pack(extent(uint64(blockfileSize)-1, 2, "ab")), false},
		{"overlap", pack(extent(0, 4, "test"), extent(2, 2, "no")), false},
		{"truncated", pack(extent(0, 5, "test")), false},
		{"trailer", pack(make([]byte, 16), []byte("extra")), false},
		{"oversized", pack(extent(0, 1<<30+1, "")), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "seed")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			err = restoreBlockSeed(f, bytes.NewReader(tc.data))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if tc.valid {
				st, _ := f.Stat()
				if st.Size() != blockfileSize {
					t.Fatal(st.Size())
				}
				var data [4]byte
				if _, err := f.ReadAt(data[:], 4096); err != nil || string(data[:]) != "test" {
					t.Fatal(string(data[:]), err)
				}
			}
		})
	}
}
