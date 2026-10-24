package appliance

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"fractalcr/internal/config"
	"golang.org/x/sys/unix"
)

var savedClockPath = "/datastore/data0/config/saved-clock.json"
var restoredClockPath = "/run/fractal-clock-restored"
var hardwareCalendarPresent = func() bool {
	_, err := os.Stat("/sys/class/rtc/rtc0")
	// An inaccessible device is not evidence that the calendar is absent.
	return !os.IsNotExist(err)
}

// The tracking reference timestamp is the last NTP update, not the current
// software clock. Saving it cannot promote a later manual clock set to a
// network-authoritative timestamp, even if the clock changes during the query.
var networkClockReference = func() (time.Time, bool) {
	data, err := chronyQuery(33, 5, 0, 104)
	if err != nil {
		return time.Time{}, false
	}
	return trackingReference(data)
}

func trackingReference(data []byte) (time.Time, bool) {
	if len(data) < 40 || !synchronizedTracking(data) {
		return time.Time{}, false
	}
	hi := binary.BigEndian.Uint32(data[28:])
	if hi == 0x7fffffff { // chrony's TV_NOHIGHSEC compatibility sentinel
		hi = 0
	}
	seconds := uint64(hi)<<32 | uint64(binary.BigEndian.Uint32(data[32:]))
	nanos := binary.BigEndian.Uint32(data[36:])
	if seconds > 253402300799 || nanos >= 1e9 {
		return time.Time{}, false
	}
	t := time.Unix(int64(seconds), int64(nanos)).UTC()
	return t, t.Year() >= 1993
}

type savedClock struct {
	Version int       `json:"version"`
	Time    time.Time `json:"time"`
}

func readSavedClock() (time.Time, error) {
	f, err := os.OpenFile(savedClockPath, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > 256 {
		return time.Time{}, fmt.Errorf("invalid saved clock record")
	}
	data, err := io.ReadAll(io.LimitReader(f, 257))
	if err != nil || len(data) > 256 {
		return time.Time{}, fmt.Errorf("invalid saved clock record")
	}
	var record savedClock
	if json.Unmarshal(data, &record) != nil || record.Version != 1 || record.Time.Year() < 1993 || record.Time.Year() > 9999 {
		return time.Time{}, fmt.Errorf("invalid saved clock record")
	}
	return record.Time.UTC(), nil
}

func saveNetworkClock(c config.Clock) error {
	if c.SaveIntervalHours == 0 || hardwareCalendarPresent() {
		return nil
	}
	if err := c.Validate(); err != nil {
		return err
	}
	reference, ok := networkClockReference()
	if !ok {
		return nil
	}
	previous, err := readSavedClock()
	// Correct an older, erroneous future time on the next authoritative update.
	// Otherwise use the last saved NTP time to enforce the write interval even
	// when the time service has restarted.
	if err == nil && !reference.Before(previous) && reference.Sub(previous) < time.Duration(c.SaveIntervalHours)*time.Hour {
		return nil
	}
	if err := checkDatastore(); err != nil {
		return err
	}
	data, err := json.Marshal(savedClock{Version: 1, Time: reference.UTC()})
	if err != nil {
		return err
	}
	return config.AtomicWrite(savedClockPath, append(data, '\n'), 0600)
}

// Called by configuration before networking starts. NVRAM is a lower bound,
// not a running clock; never rewind a newer firmware clock and never restore
// again following a service/configuration restart in the same boot.
func restoreSavedClock(c config.Clock, now time.Time) error {
	if c.SaveIntervalHours == 0 || hardwareCalendarPresent() {
		return nil
	}
	if err := c.Validate(); err != nil {
		return err
	}
	for _, path := range []string{restoredClockPath, runningPath} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	saved, err := readSavedClock()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// A damaged optional time record must not prevent NTP/network recovery.
		log.Print("Saved NVRAM time is invalid; awaiting network time")
	}
	if err == nil && saved.After(now) {
		if err := setSoftwareClock(saved); err != nil {
			return err
		}
		if err := config.AtomicWrite(clockSourcePath, []byte("NVRAM"), 0600); err != nil {
			return err
		}
	}
	return config.AtomicWrite(restoredClockPath, []byte("restored\n"), 0600)
}
