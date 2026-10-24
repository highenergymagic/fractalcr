package appliance

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

func sessionUsers(sessions map[int32]*loginSession, current *loginSession, now time.Time) string {
	var users []*loginSession
	for _, s := range sessions {
		if s.authenticated {
			users = append(users, s)
		}
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].transport != users[j].transport {
			return users[i].transport == "console"
		}
		return users[i].vty < users[j].vty
	})
	var b strings.Builder
	b.WriteString("    Line       User       Host(s)              Idle       Location\n")
	for _, s := range users {
		line := "0 con 0"
		if s.transport == "ssh" {
			line = fmt.Sprintf("%d vty %d", s.vty+1, s.vty)
		}
		marker := " "
		if s == current {
			marker = "*"
		}
		seconds := int(now.Sub(s.last) / time.Second)
		if seconds < 0 || s == current {
			seconds = 0
		}
		idle := fmt.Sprintf("%02d:%02d:%02d", seconds/3600, (seconds/60)%60, seconds%60)
		peer := s.peer
		if host, _, err := net.SplitHostPort(peer); err == nil {
			peer = host
		}
		fmt.Fprintf(&b, "%s %10s %-10s %-20s %-10s %s\n", marker, line, displayText(s.user), "idle", idle, displayText(peer))
	}
	return b.String()
}
