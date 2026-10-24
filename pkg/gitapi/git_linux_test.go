package gitapi

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"fractalcr/internal/nativeguard"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestRepositoryWithoutExecutables(t *testing.T) {
	if os.Getenv("FCR_GIT_TEST_CHILD") != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestRepositoryWithoutExecutables$")
		command.Env = append(os.Environ(), "FCR_GIT_TEST_CHILD=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("native Git: %v: %s", err, output)
		}
		return
	}
	if err := nativeguard.DenyExec(); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	repository, err := git.PlainInit(source, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "payload"), []byte("native git"), 0600); err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = worktree.Add("payload"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, ".gitignore"), []byte("ignored\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = worktree.Add(".gitignore"); err != nil {
		t.Fatal(err)
	}
	hash, err := worktree.Commit("fixture", &git.CommitOptions{Author: &object.Signature{Name: "Fixture", Email: "fixture@example.invalid", When: time.Unix(1, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	revision, err := Resolve(ctx, source, "HEAD")
	if err != nil || revision != hash.String() {
		t.Fatal(revision, err)
	}
	target := filepath.Join(directory, "checkout")
	if err = Checkout(ctx, target, source, revision); err != nil {
		t.Fatal(err)
	}
	api, err := Open(target)
	if err != nil {
		t.Fatal(err)
	}
	if commit, e := api.Commit(); e != nil || commit != revision || api.Dirty() {
		t.Fatal(commit, e, "unexpected inventory")
	}
	if err = os.WriteFile(filepath.Join(target, "ignored"), []byte("build input"), 0600); err != nil {
		t.Fatal(err)
	}
	if !api.Dirty() {
		t.Fatal("ignored build input reported clean")
	}
	if err = os.Remove(filepath.Join(target, "ignored")); err != nil {
		t.Fatal(err)
	}
	if api.Dirty() {
		t.Fatal("clean checkout reported dirty")
	}
	if err = Checkout(ctx, target, source, revision); err == nil {
		t.Fatal("existing checkout replaced")
	}
	if err = Checkout(ctx, filepath.Join(directory, "failed"), source, "0000000000000000000000000000000000000001"); err == nil {
		t.Fatal("missing object accepted")
	}
	if _, err = os.Stat(filepath.Join(directory, "failed")); !os.IsNotExist(err) {
		t.Fatal("failed checkout entered cache")
	}
	if partials, err := filepath.Glob(filepath.Join(directory, ".fractal-git-*")); err != nil || len(partials) != 0 {
		t.Fatal("partial checkout leaked", partials, err)
	}
	if err = os.WriteFile(filepath.Join(target, "payload"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if !api.Dirty() {
		t.Fatal("changed worktree reported clean")
	}
}
