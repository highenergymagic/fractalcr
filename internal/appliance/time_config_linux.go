package appliance

import (
	"fmt"
	"os"
	"time"

	"fractalcr/internal/config"
)

var restartTimeService = func() error {
	before, err := control(0, "")
	if err != nil {
		return err
	}
	state, present := before["time-sync"]
	if !present {
		return fmt.Errorf("time synchronization service unavailable")
	}
	if state.Enabled == 0 {
		return nil
	}
	if _, err := control(3, "time-sync"); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		after, err := control(0, "")
		if err != nil {
			return err
		}
		s := after["time-sync"]
		if s.Starts > state.Starts {
			if s.Ready() {
				return nil
			}
			if s.State == 5 {
				return fmt.Errorf("time synchronization service failed")
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("time synchronization service did not become ready")
}

func applyTimeConfiguration(old, next config.Config) error {
	if err := config.AtomicWrite(journalPath, old.JSON(), 0600); err != nil {
		return err
	}
	apply := func(c config.Config) error {
		if err := config.AtomicWrite(runningPath, c.JSON(), 0600); err != nil {
			return err
		}
		return restartTimeService()
	}
	if err := apply(next); err != nil {
		if rollback := apply(old); rollback != nil {
			return fmt.Errorf("time configuration failed: %v; rollback failed: %v", err, rollback)
		}
		if remove := os.Remove(journalPath); remove != nil {
			return remove
		}
		return fmt.Errorf("time configuration failed; previous configuration restored: %w", err)
	}
	return os.Remove(journalPath)
}

func timeServiceConfiguration(c config.NTP) string {
	text := "driftfile /run/fractal-time-state/drift\npidfile /run/chrony/chronyd.pid\nbindcmdaddress /run/chrony/chronyd.sock\ncmdport 0\nport 0\nmakestep 1 3\n"
	// rtcsync enables Linux's synchronized-clock RTC updates. Chrony only
	// enables those updates after acquiring a valid external time source.
	if c.UpdateCalendar {
		text += "rtcsync\n"
	}
	for _, server := range c.Servers {
		text += "server " + server + " iburst\n"
	}
	return text
}
