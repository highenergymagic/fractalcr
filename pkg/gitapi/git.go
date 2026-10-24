// Package gitapi provides Git operations without command interpreters,
// credential helper programs, hooks, or Git executables.
package gitapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	transportclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
	"github.com/go-git/go-git/v5/storage/memory"
)

type repositoryLoader struct{}

func (repositoryLoader) Load(endpoint *transport.Endpoint) (storer.Storer, error) {
	repository, err := git.PlainOpen(endpoint.Path)
	if err != nil {
		return nil, err
	}
	return repository.Storer, nil
}

func init() {
	// The default local transport runs git-upload-pack. Use the linked object
	// server instead, so even file-backed repositories need no executable.
	transportclient.InstallProtocol("file", server.NewClient(repositoryLoader{}))
}

func Resolve(ctx context.Context, remote, revision string) (string, error) {
	endpoint := git.NewRemote(memory.NewStorage(), &gitconfig.RemoteConfig{Name: "origin", URLs: []string{remote}})
	refs, err := endpoint.ListContext(ctx, &git.ListOptions{PeelingOption: git.AppendPeeled})
	if err != nil {
		return "", err
	}
	names := map[string]*plumbing.Reference{}
	for _, ref := range refs {
		names[ref.Name().String()] = ref
	}
	if revision == "" {
		revision = "HEAD"
	}
	for _, candidate := range []string{revision, "refs/heads/" + revision, "refs/tags/" + revision} {
		ref := names[candidate]
		if ref == nil {
			continue
		}
		if ref.Type() == plumbing.SymbolicReference {
			ref = names[ref.Target().String()]
		}
		if ref == nil {
			continue
		}
		if peeled := names[ref.Name().String()+"^{}"]; peeled != nil {
			ref = peeled
		}
		if !ref.Hash().IsZero() {
			return ref.Hash().String(), nil
		}
	}
	return "", fmt.Errorf("repository does not contain the requested reference")
}
func Checkout(ctx context.Context, path, remote, commit string) error {
	if len(commit) != 40 || strings.Trim(commit, "0123456789abcdef") != "" {
		return fmt.Errorf("invalid Git commit")
	}
	// A failed fetch must never leave a directory that the Compose cache
	// mistakes for a complete checkout on the next request.
	staging, err := os.MkdirTemp(filepath.Dir(path), ".fractal-git-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	repository, err := git.PlainInit(staging, false)
	if err != nil {
		return err
	}
	_, err = repository.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{remote}})
	if err != nil {
		return err
	}
	depth := 1
	refs := []gitconfig.RefSpec{gitconfig.RefSpec(commit + ":refs/heads/fractal-checkout")}
	if endpoint, e := transport.NewEndpoint(remote); e == nil && endpoint.Protocol == "file" {
		depth = 0
		refs = []gitconfig.RefSpec{"+refs/heads/*:refs/remotes/origin/*", "+refs/tags/*:refs/tags/*"}
	}
	err = repository.FetchContext(ctx, &git.FetchOptions{RemoteName: "origin", Depth: depth, RefSpecs: refs})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return err
	}
	worktree, err := repository.Worktree()
	if err != nil {
		return err
	}
	if err = worktree.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(commit)}); err != nil {
		return err
	}
	return publishCheckout(staging, path)
}

type Repository struct {
	repository *git.Repository
	root       string
}

func Open(path string) (*Repository, error) {
	repository, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, err
	}
	worktree, err := repository.Worktree()
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(worktree.Filesystem.Root())
	if err != nil {
		return nil, err
	}
	return &Repository{repository, root}, nil
}
func (r *Repository) Root() string { return r.root }
func (r *Repository) Commit() (string, error) {
	head, err := r.repository.Head()
	if err != nil {
		return "", err
	}
	return head.Hash().String(), nil
}
func (r *Repository) Dirty() bool {
	worktree, err := r.repository.Worktree()
	if err != nil {
		return true
	}
	status, err := worktree.Status()
	if err != nil || !status.IsClean() {
		return true
	}
	// Buildx provenance includes ignored files: they can still enter a build
	// context. A clean Git status alone is therefore insufficient.
	idx, err := r.repository.Storer.Index()
	if err != nil {
		return true
	}
	tracked := map[string]filemode.FileMode{}
	for _, entry := range idx.Entries {
		tracked[entry.Name] = entry.Mode
	}
	dirty := false
	err = filepath.WalkDir(r.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, e := filepath.Rel(r.root, path)
		if e != nil {
			return e
		}
		if rel == "." {
			return nil
		}
		if rel == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		mode, known := tracked[filepath.ToSlash(rel)]
		if entry.IsDir() {
			if mode == filemode.Submodule {
				return filepath.SkipDir
			}
			return nil
		}
		if !known {
			dirty = true
			return fs.SkipAll
		}
		return nil
	})
	return err != nil || dirty
}
func (r *Repository) Remote() (string, error) {
	configuration, err := r.repository.Config()
	if err != nil {
		return "", err
	}
	candidates := []string{"origin", "upstream"}
	head, err := r.repository.Head()
	if err == nil && head.Name().IsBranch() {
		if branch := configuration.Branches[head.Name().Short()]; branch != nil && branch.Remote != "" {
			candidates = append([]string{branch.Remote}, candidates...)
		}
	}
	for _, name := range candidates {
		if remote := configuration.Remotes[name]; remote != nil && len(remote.URLs) > 0 {
			return remote.URLs[0], nil
		}
	}
	return "", fmt.Errorf("repository has no configured remote")
}
func (r *Repository) Tag() (string, error) {
	head, err := r.repository.Head()
	if err != nil {
		return "", err
	}
	tags, err := r.repository.Tags()
	if err != nil {
		return "", err
	}
	defer tags.Close()
	var matches []string
	err = tags.ForEach(func(ref *plumbing.Reference) error {
		hash := ref.Hash()
		if tag, e := r.repository.TagObject(hash); e == nil {
			hash = tag.Target
		}
		if hash == head.Hash() {
			matches = append(matches, ref.Name().Short())
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return "", fmt.Errorf("no tag at current commit")
	}
	return matches[0], nil
}
