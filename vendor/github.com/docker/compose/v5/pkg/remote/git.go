/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package remote

import (
	"context"
 "fractalcr/pkg/gitapi"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/docker/cli/cli/command"
	gitutil "github.com/moby/buildkit/frontend/dockerfile/dfgitutil"

	"github.com/docker/compose/v5/pkg/api"
)

const GIT_REMOTE_ENABLED = "COMPOSE_EXPERIMENTAL_GIT_REMOTE"

func gitRemoteLoaderEnabled() (bool, error) {
	if v := os.Getenv(GIT_REMOTE_ENABLED); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("COMPOSE_EXPERIMENTAL_GIT_REMOTE environment variable expects boolean value: %w", err)
		}
		return enabled, err
	}
	return true, nil
}

func NewGitRemoteLoader(dockerCli command.Cli, offline bool) loader.ResourceLoader {
	return gitRemoteLoader{
		dockerCli: dockerCli,
		offline:   offline,
		known:     map[string]string{},
	}
}

type gitRemoteLoader struct {
	dockerCli command.Cli
	offline   bool
	known     map[string]string
}

func (g gitRemoteLoader) Accept(path string) bool {
	_, _, err := gitutil.ParseGitRef(path)
	return err == nil
}

var commitSHA = regexp.MustCompile(`^[a-f0-9]{40}$`)

func (g gitRemoteLoader) Load(ctx context.Context, path string) (string, error) {
	enabled, err := gitRemoteLoaderEnabled()
	if err != nil {
		return "", err
	}
	if !enabled {
		return "", fmt.Errorf("git remote resource is disabled by %q", GIT_REMOTE_ENABLED)
	}

	ref, _, err := gitutil.ParseGitRef(path)
	if err != nil {
		return "", err
	}

	local, ok := g.known[path]
	if !ok {
		if ref.Ref == "" {
			ref.Ref = "HEAD" // default branch
		}

		err = g.resolveGitRef(ctx, path, ref)
		if err != nil {
			return "", err
		}

		cache, err := cacheDir()
		if err != nil {
			return "", fmt.Errorf("initializing remote resource cache: %w", err)
		}

		local = filepath.Join(cache, ref.Ref)
		if _, err := os.Stat(local); os.IsNotExist(err) {
			if g.offline {
				return "", nil
			}
			err = g.checkout(ctx, local, ref)
			if err != nil {
				return "", err
			}
		}
		g.known[path] = local
	}
	if ref.SubDir != "" {
		if err := validateGitSubDir(local, ref.SubDir); err != nil {
			return "", err
		}
		local = filepath.Join(local, ref.SubDir)
	}
	stat, err := os.Stat(local)
	if err != nil {
		return "", err
	}
	if stat.IsDir() {
		local, err = findFile(cli.DefaultFileNames, local)
	}
	return local, err
}

func (g gitRemoteLoader) Dir(path string) string {
	return g.known[path]
}

// validateGitSubDir ensures a subdirectory path is contained within the base directory
// and doesn't escape via path traversal. Unlike validatePathInBase for OCI artifacts,
// this allows nested directories but prevents traversal outside the base.
func validateGitSubDir(base, subDir string) error {
	cleanSubDir := filepath.Clean(subDir)

	if filepath.IsAbs(cleanSubDir) {
		return fmt.Errorf("git subdirectory must be relative, got: %s", subDir)
	}

	if cleanSubDir == ".." || strings.HasPrefix(cleanSubDir, "../") || strings.HasPrefix(cleanSubDir, "..\\") {
		return fmt.Errorf("git subdirectory path traversal detected: %s", subDir)
	}

	if len(cleanSubDir) >= 2 && cleanSubDir[1] == ':' {
		return fmt.Errorf("git subdirectory must be relative, got: %s", subDir)
	}

	targetPath := filepath.Join(base, cleanSubDir)
	cleanBase := filepath.Clean(base)
	cleanTarget := filepath.Clean(targetPath)

	// Ensure the target starts with the base path
	relPath, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return fmt.Errorf("invalid git subdirectory path: %w", err)
	}

	if relPath == ".." || strings.HasPrefix(relPath, "../") || strings.HasPrefix(relPath, "..\\") {
		return fmt.Errorf("git subdirectory escapes base directory: %s", subDir)
	}

	return nil
}

func (g gitRemoteLoader) resolveGitRef(ctx context.Context, path string, ref *gitutil.GitRef) error {
 if commitSHA.MatchString(ref.Ref) {return nil}
 resolved,err:=gitapi.Resolve(ctx,ref.Remote,ref.Ref)
 if err==nil {ref.Ref=resolved}
 return err
}
func (g gitRemoteLoader) checkout(ctx context.Context, path string, ref *gitutil.GitRef) error {
 return gitapi.Checkout(ctx,path,ref.Remote,ref.Ref)
}

func findFile(names []string, pwd string) (string, error) {
	for _, n := range names {
		f := filepath.Join(pwd, n)
		if fi, err := os.Stat(f); err == nil && !fi.IsDir() {
			return f, nil
		}
	}
	return "", api.ErrNotFound
}

var _ loader.ResourceLoader = gitRemoteLoader{}
