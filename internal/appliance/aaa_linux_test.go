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
	r, _ := s.authenticate(managementRequest{Command: "__connect"}, c, now)
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
	r, _ = s.authenticate(managementRequest{Command: "__tick"}, c, now.Add(601*time.Second))
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
	r, _ := s.authenticate(managementRequest{Command: "__connect", Transport: "ssh", User: "unknown"}, c, time.Now())
	if !r.Close {
		t.Fatal("unknown principal")
	}
	s.authenticate(managementRequest{Command: "__connect", Transport: "ssh", User: "admin", Verifier: hash}, c, time.Now())
	delete(c.AAA.Users, "admin")
	r, _ = s.authenticate(managementRequest{Command: "__tick"}, c, time.Now())
	if !r.Close || s.authenticated {
		t.Fatal("deleted principal retained access")
	}
}

func TestBootstrapConsoleDoesNotExpireIntoAnUnconfiguredLogin(t *testing.T) {
	c := config.Default()
	s := &loginSession{cli: &cli.Session{Running: c, Candidate: c.Clone(), EnforcePrivilege: true}}
	now := time.Now()
	s.authenticate(managementRequest{Command: "__connect"}, c, now)
	r, _ := s.authenticate(managementRequest{Command: "__tick"}, c, now.Add(24*time.Hour))
	if r.Challenge || !s.authenticated || s.cli.Privilege != 15 {
		t.Fatal("bootstrap console locked without configured AAA", r)
	}
}
