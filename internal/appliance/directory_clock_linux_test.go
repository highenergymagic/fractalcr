package appliance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fractalcr/internal/config"
)

func TestDirectoryUsesConfiguredClockAtFileTimestamp(t *testing.T) {
	old := fileSystems
	defer func() { fileSystems = old }()
	root := t.TempDir()
	fileSystems = func() []applianceFS { return []applianceFS{{"data0", root, false}} }
	c := config.Default()
	summer, err := config.ParseSummerTime("PDT", "recurring", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Clock = config.Clock{Zone: "PST", OffsetMinutes: -480, Summer: summer}
	for _, tt := range []struct{ name, stamp, want string }{
		{"winter", "2026-01-01T18:00:00Z", "Jan 1 2026 10:00:00 -08:00"},
		{"summer", "2026-07-01T18:00:00Z", "Jul 1 2026 11:00:00 -07:00"},
	} {
		file := filepath.Join(root, tt.name)
		if err := os.WriteFile(file, []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
		stamp, _ := time.Parse(time.RFC3339, tt.stamp)
		if err := os.Chtimes(file, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		for _, command := range []string{"dir data0:", "dir data0:" + tt.name, "dir all-filesystems"} {
			got, err := Operations(strings.Fields(command), c)
			if err != nil || !strings.Contains(got, tt.want) {
				t.Fatal(command, got, err)
			}
		}
	}
}
