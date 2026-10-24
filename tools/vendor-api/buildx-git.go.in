// Native API implementation for the pinned Buildx Git inventory interface.
package gitutil
import (
 "context"
 "path/filepath"
 "strings"
 "fractalcr/pkg/gitapi"
 bkgitutil "github.com/moby/buildkit/util/gitutil"
 bkurlutil "github.com/moby/buildkit/util/urlutil"
)
type GitCLI struct{repository *gitapi.Repository}
func New(opts ...bkgitutil.Option)(*GitCLI,error){
 options:=bkgitutil.NewGitCLI(opts...)
 repository,err:=gitapi.Open(options.Dir())
 if err!=nil{return nil,err};return &GitCLI{repository},nil
}
func(c *GitCLI)IsInsideWorkTree(context.Context)bool{return c.repository!=nil}
func(c *GitCLI)IsDirty(context.Context)bool{return c.repository.Dirty()}
func(c *GitCLI)WorkTree(context.Context)(string,error){return c.repository.Root(),nil}
func(c *GitCLI)GitDir(context.Context)(string,error){return filepath.Join(c.repository.Root(),".git"),nil}
func(c *GitCLI)RemoteURL(context.Context)(string,error){value,err:=c.repository.Remote();return bkurlutil.RedactCredentials(value),err}
func(c *GitCLI)FullCommit(context.Context)(string,error){return c.repository.Commit()}
func(c *GitCLI)ShortCommit(context.Context)(string,error){commit,err:=c.repository.Commit();if len(commit)>7{commit=commit[:7]};return commit,err}
func(c *GitCLI)Tag(context.Context)(string,error){return c.repository.Tag()}
func IsUnknownRevision(err error)bool{return err!=nil && (strings.Contains(err.Error(),"reference not found") || strings.Contains(err.Error(),"unknown revision"))}
