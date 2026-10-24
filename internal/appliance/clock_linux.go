package appliance

import (
	"fmt"
	"fractalcr/internal/config"
	"golang.org/x/sys/unix"
	"os"
	"strings"
	"time"
)

var setSoftwareClock = func(t time.Time) error {
	ts := unix.NsecToTimespec(t.UnixNano())
	return unix.ClockSettime(unix.CLOCK_REALTIME, &ts)
}
var clockSourcePath = "/run/fractal-clock-source"
var readHardwareClock = func() (time.Time, error) {
	f, err := os.OpenFile("/dev/rtc0", os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()
	r, err := unix.IoctlGetRTCTime(int(f.Fd()))
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(int(r.Year)+1900, time.Month(r.Mon)+1, int(r.Mday), int(r.Hour), int(r.Min), int(r.Sec), 0, time.UTC), nil
}
var writeHardwareClock = func(t time.Time) error {
	f, err := os.OpenFile("/dev/rtc0", os.O_WRONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	t = t.UTC()
	r := unix.RTCTime{Sec: int32(t.Second()), Min: int32(t.Minute()), Hour: int32(t.Hour()), Mday: int32(t.Day()), Mon: int32(t.Month()) - 1, Year: int32(t.Year()) - 1900, Wday: int32(t.Weekday()), Yday: int32(t.YearDay()) - 1}
	return unix.IoctlSetRTCTime(int(f.Fd()), &r)
}

func clockOperation(w []string, c config.Config) (string, error) {
	var err error
	switch strings.Join(w, " ") {
	case "clock update-calendar":
		err = writeHardwareClock(time.Now())
	case "clock read-calendar":
		var now time.Time
		now, err = readHardwareClock()
		if err == nil {
			err = setSoftwareClock(now)
		}
	default:
		if len(w) != 6 || w[1] != "set" {
			return "", fmt.Errorf("Invalid clock command")
		}
		var now time.Time
		now, err = config.ParseClockTime(w[2:], c.Clock.Location())
		if err == nil {
			now, err = config.ParseClockTime(w[2:], c.Clock.LocalTime(now).Location())
		}
		if err == nil {
			if w[0] == "calendar" {
				err = writeHardwareClock(now)
			} else {
				err = setSoftwareClock(now)
			}
		}
	}
	if err != nil {
		return "", fmt.Errorf("Clock update failed: %w", err)
	}
	if w[0] == "clock" && w[1] != "update-calendar" {
		source := "user configuration"
		if w[1] == "read-calendar" {
			source = "hardware calendar"
		}
		if err = config.AtomicWrite(clockSourcePath, []byte(source), 0600); err != nil {
			return "", err
		}
	}
	return "", nil
}
