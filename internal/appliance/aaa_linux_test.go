package appliance

import (
	"errors"
	"fractalcr/internal/cli"
	"fractalcr/internal/config"
	"path/filepath"
	"testing"
	"time"
)

func TestAAAAuthenticationAuthorizationTimeoutAndAccounting(t *testing.T) {
	oldCheck, oldAudit := checkDatastore, auditPath
	checkDatastore = func() error { return nil }
	auditPath = filepath.Join(t.TempDir(), "audit.jsonl")
	defer func() { checkDatastore = oldCheck; auditPath = oldAudit }()
	loginFailures = map[string]loginFailure{}
	hash, _ := config.HashSecret("operator test password")
	c := config.Default()
	c.AAA.Enabled = true
	c.AAA.Users = map[string]config.User{"operator": {Secret: hash, Privilege: 1}, "admin": {Secret: hash, Privilege: 15}}
	c.AAA.EnableSecret = hash
	s := &loginSession{cli: &cli.Session{Running: c, Candidate: c.Clone(), EnforcePrivilege: true}}
	now := time.Now()
	r, _ := s.authenticate(managementRequest{Action: "connect"}, c, now)
	if !r.Challenge || r.Secret {
		t.Fatal(r)
	}
	r, _ = s.authenticate(managementRequest{Command: "operator"}, c, now)
	if !r.Secret {
		t.Fatal(r)
	}
	r, _ = s.authenticate(managementRequest{Command: "operator test password"}, c, now)
	if r.Challenge || !s.authenticated || s.cli.Privilege != 1 {
		t.Fatal(r)
	}
	for _, w := range [][]string{{"request", "container-engine", "run"}, {"configure", "terminal"}, {"show", "running-config"}, {"do", "reload"}, {"request", "platform", "debug", "service-container", "enter"}} {
		if s.cli.Allowed(w) {
			t.Fatalf("authorized %v", w)
		}
	}
	if !s.cli.Allowed([]string{"show", "version"}) {
		t.Fatal("public show denied")
	}
	s.stage = "enable"
	r, _ = s.authenticate(managementRequest{Command: "operator test password"}, c, now)
	if s.cli.Privilege != 15 || r.Challenge {
		t.Fatal(r)
	}
	r, _ = s.authenticate(managementRequest{Action: "tick"}, c, now.Add(601*time.Second))
	if s.authenticated || !r.Challenge {
		t.Fatal("idle privilege retained")
	}
	for i := 0; i < 3; i++ {
		s.authenticate(managementRequest{Command: "operator"}, c, now)
		s.authenticate(managementRequest{Command: "incorrect"}, c, now)
	}
	s.authenticate(managementRequest{Command: "operator"}, c, now)
	r, _ = s.authenticate(managementRequest{Command: "operator test password"}, c, now)
	if s.authenticated || r.Error == "" {
		t.Fatal("lockout bypass")
	}
	checkDatastore = func() error { return errors.New("offline") }
	s.stage = "password"
	s.user = "admin"
	r, _ = s.authenticate(managementRequest{Command: "operator test password"}, c, now.Add(time.Minute))
	if s.authenticated || !r.Close {
		t.Fatal("failed accounting allowed login")
	}
}

func TestCommandExecutionDoesNotConsumeIdleTimeout(t *testing.T) {
	c := config.Default()
	c.AAA.Enabled = true
	c.AAA.ConsoleTimeout = 5
	start := time.Now()
	s := &loginSession{cli: &cli.Session{Running: c, Candidate: c.Clone()}, authenticated: true, bootstrap: true, transport: "console", last: start}
	// A slow durable configuration write takes longer than the idle limit.
	finished := start.Add(20 * time.Second)
	s.commandCompleted(managementRequest{Command: "exec-timeout 0 5"}, finished)
	r, _ := s.authenticate(managementRequest{Action: "tick"}, c, finished.Add(time.Second))
	if r.Challenge || !s.authenticated {
		t.Fatal("command execution consumed the prompt's idle timeout", r)
	}
	s.commandCompleted(managementRequest{Action: "tick"}, finished.Add(4*time.Second))
	if !s.last.Equal(finished) {
		t.Fatal("heartbeat renewed the idle deadline")
	}
	s.authenticated = false
	s.commandCompleted(managementRequest{Command: "logout"}, finished.Add(30*time.Second))
	if !s.last.Equal(finished) {
		t.Fatal("closed session renewed the idle deadline")
	}
}

func TestVTYPrincipalRequiresAAAAndRevocation(t *testing.T) {
	oldCheck, oldAudit := checkDatastore, auditPath
	checkDatastore = func() error { return nil }
	auditPath = filepath.Join(t.TempDir(), "audit.jsonl")
	defer func() { checkDatastore = oldCheck; auditPath = oldAudit }()
	hash, _ := config.HashSecret("administrator password")
	c := config.Default()
	c.AAA.Enabled = true
	c.AAA.SSH = true
	c.AAA.Users = map[string]config.User{"admin": {Secret: hash, Privilege: 15}}
	s := &loginSession{cli: &cli.Session{Running: c, Candidate: c.Clone(), EnforcePrivilege: true}}
	r, _ := s.authenticate(managementRequest{Action: "connect", Transport: "ssh", User: "unknown"}, c, time.Now())
	if !r.Close {
		t.Fatal("unknown principal")
	}
	s.authenticate(managementRequest{Action: "connect", Transport: "ssh", User: "admin", Verifier: hash}, c, time.Now())
	delete(c.AAA.Users, "admin")
	r, _ = s.authenticate(managementRequest{Action: "tick"}, c, time.Now())
	if !r.Close || s.authenticated {
		t.Fatal("deleted principal retained access")
	}
}

func TestBootstrapConsoleDoesNotExpireIntoAnUnconfiguredLogin(t *testing.T) {
	c := config.Default()
	s := &loginSession{cli: &cli.Session{Running: c, Candidate: c.Clone(), EnforcePrivilege: true}}
	now := time.Now()
	s.authenticate(managementRequest{Action: "connect"}, c, now)
	r, _ := s.authenticate(managementRequest{Action: "tick"}, c, now.Add(24*time.Hour))
	if r.Challenge || !s.authenticated || s.cli.Privilege != 15 {
		t.Fatal("bootstrap console locked without configured AAA", r)
	}
}

func TestBootstrapGrantEndsAtLogoutAfterEnablingAAA(t *testing.T) {
	oldCheck, oldAudit := checkDatastore, auditPath
	checkDatastore = func() error { return nil }
	auditPath = filepath.Join(t.TempDir(), "audit.jsonl")
	defer func() { checkDatastore = oldCheck; auditPath = oldAudit }()
	c := config.Default()
	s := &loginSession{cli: &cli.Session{Running: c, Candidate: c.Clone(), EnforcePrivilege: true}}
	now := time.Now()
	s.authenticate(managementRequest{Action: "connect"}, c, now)
	hash, _ := config.HashSecret("synthetic-console-password")
	c.AAA.Users = map[string]config.User{"admin": {Privilege: 15, Secret: hash}}
	c.AAA.Enabled = true
	r, _ := s.authenticate(managementRequest{Action: "tick"}, c, now)
	if r.Challenge || !s.authenticated || !s.bootstrap {
		t.Fatal("enabling login interrupted configuration")
	}
	s.logout()
	if s.bootstrap || s.authenticated || s.cli.Privilege != 0 {
		t.Fatal("logout retained bootstrap privilege")
	}
	r, _ = s.authenticate(managementRequest{Action: "connect"}, c, now)
	if !r.Challenge || s.authenticated {
		t.Fatal("new session bypassed local login")
	}
	s.authenticate(managementRequest{Command: "admin"}, c, now)
	r, _ = s.authenticate(managementRequest{Command: "synthetic-console-password"}, c, now)
	if r.Challenge || !s.authenticated || s.bootstrap {
		t.Fatal("local login failed")
	}
	delete(c.AAA.Users, "admin")
	r, _ = s.authenticate(managementRequest{Action: "tick"}, c, now)
	if !r.Challenge || s.authenticated {
		t.Fatal("authenticated account revocation bypassed")
	}
}
