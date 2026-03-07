// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package config

import (
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/honeydipper/honeydipper/v4/pkg/tokenhelper"
)

// _GHAppInfo stores the GH App info used for cloning the code.
var _GHAppInfo map[string]any = nil

// GitClient interface for mocking git operations in tests.
type GitClient interface {
	PlainClone(directory string, isBare bool, o *git.CloneOptions) (*git.Repository, error)
}

// DefaultGitClient implements GitClient using the actual git library.
type DefaultGitClient struct{}

func (d *DefaultGitClient) PlainClone(directory string, isBare bool, o *git.CloneOptions) (*git.Repository, error) {
	//nolint:wrapcheck
	return git.PlainClone(directory, isBare, o)
}

// TempDirCreator interface for mocking temp directory creation.
type TempDirCreator interface {
	MkdirTemp(dir, pattern string) (string, error)
}

// DefaultTempDirCreator implements TempDirCreator using os.MkdirTemp.
type DefaultTempDirCreator struct{}

func (d *DefaultTempDirCreator) MkdirTemp(dir, pattern string) (string, error) {
	//nolint:wrapcheck
	return os.MkdirTemp(dir, pattern)
}

// DefaultHeadGetter returns the HEAD hash from a git repository.
func DefaultHeadGetter(r *git.Repository) string {
	return dipper.Must(r.Head()).(*plumbing.Reference).Hash().String()
}

// BuildCloneOptions builds the git clone options including authentication.
func BuildCloneOptions(repo RepoInfo) *git.CloneOptions {
	branch := plumbing.Main.Short()
	if repo.Branch != "" {
		branch = repo.Branch
	}
	opts := &git.CloneOptions{
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		URL:           repo.Repo,
	}

	opts.Auth = SetupAuth(&repo)

	return opts
}

// BuildPullOptions builds the git Pull options including authentication.
func BuildPullOptions(repo RepoInfo) *git.PullOptions {
	branch := plumbing.Main.Short()
	if repo.Branch != "" {
		branch = repo.Branch
	}
	opts := &git.PullOptions{
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		RemoteName:    "origin",
		RemoteURL:     repo.Repo,
		Force:         true,
		SingleBranch:  true,
	}

	opts.Auth = SetupAuth(&repo)

	return opts
}

// SetupAuth configures authentication for the git clone options.
func SetupAuth(repo *RepoInfo) transport.AuthMethod {
	var auth transport.AuthMethod
	switch {
	case strings.HasPrefix(repo.Repo, "git@"):
		auth = GetGitSSHAuth(repo.KeyFile, repo.KeyPassEnv)
	case repo.TokenSource == "github":
		if _GHAppInfo == nil || _GHAppInfo["mock"] != nil {
			newAppInfo := map[string]any{
				"app_id":          os.Getenv("GH_APP_ID"),
				"installation_id": os.Getenv("GH_APP_INSTALLATION_ID"),
				"key":             os.Getenv("GH_APP_KEY"),
				"permissions":     map[string]any{"contents": "read"},
				"mock":            _GHAppInfo["mock"],
			}
			_GHAppInfo = newAppInfo
		}
		token := tokenhelper.GetGitHubToken(_GHAppInfo)
		auth = &http.BasicAuth{Username: "x-access-token", Password: token}
		dipper.Logger.Infof("using github app token for %s", repo.Repo)
	case repo.PassEnv != "":
		pass := os.Getenv(repo.PassEnv)
		username := repo.Username
		if username == "" {
			username = "x-access-token"
		}
		auth = &http.BasicAuth{Username: username, Password: pass}
		dipper.Logger.Infof("using %s as password for %s", repo.PassEnv, repo.Repo)
	case repo.Username != "":
		pass := os.Getenv("DIPPER_GIT_PASS_" + strings.ReplaceAll(strings.ToUpper(repo.Username), "-", "_"))
		if pass == "" {
			pass = os.Getenv("DIPPER_GIT_PASS")
		}
		auth = &http.BasicAuth{Username: repo.Username, Password: pass}
		dipper.Logger.Infof("using DIPPER_GIT_PASS_* as password for %s", repo.Repo)
	}

	return auth
}
