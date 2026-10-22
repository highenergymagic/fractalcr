package config

import (
	"bytes"
	"testing"
)

func FuzzDecodeRoundTrip(f *testing.F) {
	f.Add(Default().JSON())
	f.Add([]byte(`{"version":1,"hostname":"ctr","hostname":"duplicate"}`))
	f.Add([]byte(`{"version":1,"hostname":"ctr","aaa":{"users":null}}`))
	f.Add([]byte(`{"version":1,"hostname":"ctr"} {}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1024*1024 {
			t.Skip()
		}
		c, err := Decode(data)
		if err != nil {
			return
		}
		copy, err := Decode(c.JSON())
		if err != nil || !bytes.Equal(c.JSON(), copy.JSON()) {
			t.Fatalf("accepted configuration does not round-trip: %v", err)
		}
	})
}
