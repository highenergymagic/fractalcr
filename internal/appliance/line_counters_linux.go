package appliance

import (
	"encoding/json"
	"fmt"
	"os"

	"fractalcr/internal/config"
)

// Counters belong to the running platform, not a management process or a saved
// configuration. /run survives service restarts and resets on a machine boot.
type lineCounters struct {
	Uses [config.VTYCount + 1]uint64 `json:"uses"`
	path string
}

func loadLineCounters(path string) (*lineCounters, error) {
	c := &lineCounters{path: path}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	var state struct {
		Uses []uint64 `json:"uses"`
	}
	if err := json.Unmarshal(data, &state); err != nil || len(state.Uses) != len(c.Uses) {
		return c, fmt.Errorf("invalid line counter state")
	}
	copy(c.Uses[:], state.Uses)
	return c, nil
}

func (c *lineCounters) record(number int) error {
	if number < 0 || number >= len(c.Uses) {
		return fmt.Errorf("invalid terminal line")
	}
	if c.Uses[number] != ^uint64(0) {
		c.Uses[number]++
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return config.AtomicWrite(c.path, data, 0600)
}
