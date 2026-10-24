package appliance

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"fractalcr/internal/config"
)

type reloadPlan struct {
	At           time.Time `json:"at"`
	Reason       string    `json:"reason"`
	User         string    `json:"user"`
	Scheduled    bool      `json:"scheduled"`
	Requested    time.Time `json:"requested"`
	WarnedMinute bool      `json:"warned_minute,omitempty"`
}

// Pending reloads live outside terminal sessions and survive a management
// service restart, but not a machine reboot (/run is volatile).
type reloadManager struct {
	mu       sync.Mutex
	path     string
	plan     *reloadPlan
	failure  string
	fire     func() error
	notices  []reloadNotice
	sequence uint64
}

type reloadNotice struct {
	sequence uint64
	text     string
}

func (m *reloadManager) notify(text string) {
	m.sequence++
	m.notices = append(m.notices, reloadNotice{m.sequence, text})
	if len(m.notices) > 32 {
		m.notices = m.notices[len(m.notices)-32:]
	}
}
func (m *reloadManager) notifications(after uint64) (uint64, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	for _, n := range m.notices {
		if n.sequence > after {
			b.WriteString(n.text)
		}
	}
	return m.sequence, b.String()
}
func shutdownNotice(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int(d / time.Second)
	return fmt.Sprintf("\n***\n*** --- SHUTDOWN in %d:%02d:%02d ---\n***\n", seconds/3600, seconds/60%60, seconds%60)
}

func newReloadManager(path string, fire func() error) (*reloadManager, error) {
	m := &reloadManager{path: path, fire: fire}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	var p reloadPlan
	if err = json.Unmarshal(data, &p); err != nil || p.At.IsZero() {
		return nil, fmt.Errorf("Invalid pending reload record")
	}
	m.plan = &p
	return m, nil
}

func (m *reloadManager) set(p reloadPlan) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.Marshal(p)
	if err == nil {
		err = config.AtomicWrite(m.path, data, 0600)
	}
	if err != nil {
		return err
	}
	m.plan, m.failure = &p, ""
	if p.Scheduled {
		m.notify(shutdownNotice(p.At.Sub(p.Requested)))
	}
	return nil
}

func (m *reloadManager) cancel() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.plan == nil {
		return "No reload is scheduled.", nil
	}
	if err := os.Remove(m.path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	m.plan, m.failure = nil, ""
	m.notify("\n***\n*** --- SHUTDOWN ABORTED ---\n***\n")
	return "%Reload cancelled.", nil
}

func (m *reloadManager) status(now time.Time, clock ...config.Clock) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure != "" {
		return "% Reload failed: " + m.failure
	}
	if m.plan == nil {
		return "No reload is scheduled."
	}
	d := m.plan.At.Sub(now).Round(time.Second)
	if d < 0 {
		d = 0
	}
	at := m.plan.At
	if len(clock) != 0 {
		at = clock[0].LocalTime(at)
	}
	s := fmt.Sprintf("Reload scheduled for %s (in %s)", at.Format("15:04:05 MST Mon Jan 2 2006"), reloadInterval(d))
	if m.plan.Reason != "" {
		s += "\nReload reason: " + m.plan.Reason
	}
	return s
}

func reloadInterval(d time.Duration) string {
	minutes := int(d / time.Minute)
	hours := minutes / 60
	minutes %= 60
	part := func(n int, unit string) string {
		if n != 1 {
			unit += "s"
		}
		return fmt.Sprintf("%d %s", n, unit)
	}
	if hours == 0 {
		return part(minutes, "minute")
	}
	if minutes == 0 {
		return part(hours, "hour")
	}
	return part(hours, "hour") + " and " + part(minutes, "minute")
}

func (m *reloadManager) due(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.plan == nil {
		return
	}
	if now.Before(m.plan.At) {
		if m.plan.Scheduled && !m.plan.WarnedMinute && m.plan.At.Sub(now) <= time.Minute {
			p := *m.plan
			p.WarnedMinute = true
			data, err := json.Marshal(p)
			if err == nil {
				err = config.AtomicWrite(m.path, data, 0600)
			}
			if err != nil {
				m.failure = err.Error()
				return
			}
			m.plan = &p
			m.notify(shutdownNotice(p.At.Sub(now)))
		}
		return
	}
	// Delete the record before sending the request: a failed supervisor request
	// is reported, never retried behind a newly connected administrator's back.
	if err := os.Remove(m.path); err != nil && !os.IsNotExist(err) {
		m.failure = err.Error()
		return
	}
	m.plan = nil
	if err := m.fire(); err != nil {
		m.failure = err.Error()
	}
}

func (m *reloadManager) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for now := range ticker.C {
		m.due(now)
	}
}

func decimal(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("number required")
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid number")
		}
	}
	return strconv.Atoi(s)
}

func reloadTime(s string, maxHour int) (int, int, error) {
	h, min, ok := strings.Cut(s, ":")
	a, ea := decimal(h)
	b, eb := decimal(min)
	if !ok || ea != nil || eb != nil || a > maxHour || b > 59 {
		return 0, 0, fmt.Errorf("Invalid time")
	}
	return a, b, nil
}

func reloadMonth(s string) time.Month {
	if len(s) < 3 {
		return 0
	}
	for m := time.January; m <= time.December; m++ {
		if strings.HasPrefix(strings.ToLower(m.String()), strings.ToLower(s)) {
			return m
		}
	}
	return 0
}

func parseReload(w []string, now time.Time, clocks ...config.Clock) (reloadPlan, error) {
	zone, offset := now.Zone()
	clock := config.Clock{Zone: zone, OffsetMinutes: offset / 60}
	if len(clocks) != 0 {
		clock = clocks[0]
	}
	now = clock.LocalTime(now)
	p := reloadPlan{At: now, Requested: now}
	args := w[1:]
	if len(args) > 0 && strings.HasPrefix(args[0], "/") {
		return p, fmt.Errorf("Image authentication is enforced by ROMMON; reload verification overrides are not supported")
	}
	if len(args) > 0 && args[0] == "warm" {
		return p, fmt.Errorf("Warm reload is not supported by this platform")
	}
	if len(args) > 0 {
		switch args[0] {
		case "in":
			if len(args) < 2 {
				return p, fmt.Errorf("Incomplete command.")
			}
			minutes, err := decimal(args[1])
			if strings.Contains(args[1], ":") {
				var h, m int
				h, m, err = reloadTime(args[1], 576)
				minutes = h*60 + m
			}
			if err != nil || minutes > 24*24*60 {
				return p, fmt.Errorf("Reload must occur within 24 days")
			}
			p.At, p.Scheduled = now.Add(time.Duration(minutes)*time.Minute), true
			if minutes == 0 {
				p.Scheduled = false
			}
			args = args[2:]
		case "at":
			if len(args) < 2 {
				return p, fmt.Errorf("Incomplete command.")
			}
			h, m, err := reloadTime(args[1], 23)
			if err != nil {
				return p, err
			}
			wall := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, time.UTC)
			p.Scheduled = true
			args = args[2:]
			explicitDate := false
			if len(args) >= 2 {
				month := reloadMonth(args[0])
				day, e := decimal(args[1])
				if month == 0 {
					month = reloadMonth(args[1])
					day, e = decimal(args[0])
				}
				if month != 0 && e == nil {
					wall = time.Date(now.Year(), month, day, h, m, 0, 0, time.UTC)
					if day < 1 || wall.Month() != month {
						return p, fmt.Errorf("Invalid date")
					}
					explicitDate = true
					args = args[2:]
				}
			}
			// Select the first future occurrence of this local reading. During
			// the autumn fold the second occurrence may still be ahead of us.
			found := false
			for attempt := 0; attempt < 2; attempt++ {
				instants := clock.LocalInstants(wall)
				if len(instants) == 0 {
					return p, fmt.Errorf("Specified time does not exist due to summer-time transition")
				}
				for _, instant := range instants {
					if instant.After(now) {
						p.At, found = instant, true
						break
					}
				}
				if found {
					break
				}
				if explicitDate {
					wall = wall.AddDate(1, 0, 0)
				} else {
					wall = wall.AddDate(0, 0, 1)
				}
			}
			if !found {
				return p, fmt.Errorf("Invalid reload time")
			}
			if p.At.Sub(now) > 24*24*time.Hour {
				return p, fmt.Errorf("Reload must occur within 24 days")
			}
		case "reason":
			args = args[1:]
		}
	}
	p.Reason = strings.Join(args, " ")
	if len(p.Reason) > 255 || strings.ContainsAny(p.Reason, "\r\n\x00") {
		return p, fmt.Errorf("Reload reason must be 1 to 255 characters")
	}
	p.At = clock.LocalTime(p.At)
	return p, nil
}

type reloadDialog struct {
	plan reloadPlan
	save bool
}

func (d *reloadDialog) prompt() reply {
	if d.save {
		return reply{Prompt: "System configuration has been modified. Save? [yes/no]: ", Challenge: true}
	}
	return reply{Prompt: "Proceed with reload? [confirm]", Challenge: true}
}

func (s *loginSession) beginReload(w []string, m *reloadManager, now time.Time) reply {
	now = s.cli.Running.Clock.LocalTime(now)
	r := reply{Prompt: s.cli.Prompt()}
	if len(w) == 2 && w[1] == "cancel" {
		var err error
		r.Output, err = m.cancel()
		if err != nil {
			r.Error = err.Error()
		}
		return r
	}
	if len(w) > 1 && w[1] == "at" {
		_, source := softwareClockState(now)
		if source == "none" {
			r.Error = "The clock has not been set"
			return r
		}
	}
	p, err := parseReload(w, now, s.cli.Running.Clock)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	p.User = s.user
	startup, err := config.Load(startupPath)
	if err != nil && !os.IsNotExist(err) {
		r.Error = "Cannot read startup configuration"
		return r
	}
	s.pendingReload = &reloadDialog{plan: p, save: err != nil || string(startup.JSON()) != string(s.cli.Running.JSON())}
	r = s.pendingReload.prompt()
	if p.Scheduled {
		r.Output = fmt.Sprintf("Reload scheduled for %s (in %s)", p.At.Format("15:04:05 MST Mon Jan 2 2006"), reloadInterval(p.At.Sub(now)))
	}
	return r
}

func (s *loginSession) answerReload(answer string, m *reloadManager, now time.Time) reply {
	d := s.pendingReload
	r := reply{Prompt: s.cli.Prompt()}
	if s.cli.Privilege != 15 {
		s.pendingReload = nil
		r.Error = "Authorization failed."
		return r
	}
	if d.save {
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			if s.cli.Save == nil {
				r.Error = "Configuration storage is unavailable"
				s.pendingReload = nil
				return r
			}
			if err := s.cli.Save(s.cli.Running); err != nil {
				r.Error = err.Error()
				s.pendingReload = nil
				return r
			}
			d.save = false
			r = d.prompt()
			r.Output = "Building configuration...\n[OK]"
			return r
		case "n", "no":
			d.save = false
			return d.prompt()
		default:
			r = d.prompt()
			r.Output = "% Please answer 'yes' or 'no'."
			return r
		}
	}
	s.pendingReload = nil
	if answer != "" && !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
		return r
	}
	if !d.plan.Scheduled {
		d.plan.At = now.Add(time.Second)
	}
	if d.plan.Scheduled && d.plan.At.Before(now) {
		r.Error = "Scheduled reload time has passed"
		return r
	}
	d.plan.Requested = now
	if err := m.set(d.plan); err != nil {
		r.Error = err.Error()
		return r
	}
	if d.plan.Scheduled {
		r.Output = m.status(now, s.cli.Running.Clock)
	} else {
		r.Output = "\nReload requested.\n"
	}
	return r
}

// Configuration commands share the "enable" keyword. Only the resolved EXEC
// forms enter authentication; enable secret must reach the config transaction.
func execEnableLevel(w []string) (int, bool) {
	if len(w) > 0 && w[0] == "do" {
		w = w[1:]
	}
	if len(w) == 0 || w[0] != "enable" {
		return 0, false
	}
	if len(w) == 1 {
		return 15, true
	}
	if len(w) != 2 {
		return 0, false
	}
	level, err := strconv.Atoi(w[1])
	return level, err == nil && level >= 0 && level <= 15
}

func (s *loginSession) enable(level int, c config.Config) reply {
	r := reply{Prompt: s.cli.Prompt()}
	if level <= s.cli.Privilege {
		s.cli.Privilege = level
		r.Prompt = s.cli.Prompt()
		return r
	}
	if c.AAA.EnableVerifier(level) == "" {
		if s.transport == "console" {
			s.cli.Privilege = level
			r.Prompt = s.cli.Prompt()
		} else {
			r.Error = "No password set"
		}
		return r
	}
	s.stage, s.enableLevel = "enable", level
	return s.challenge()
}
