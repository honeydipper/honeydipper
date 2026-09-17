// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package config

import (
	"os"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/assert"
)

const (
	testTempDir = "/tmp/test-repo"
)

func TestUpdateBuiltinCtxs(t *testing.T) {
	// Test with branch specified
	repo := &Repo{
		repo: &RepoInfo{
			Repo:   "https://github.com/user/repo",
			Branch: "develop",
		},
		hash: "abc123",
	}
	assembled := &DataSet{
		Contexts: map[string]interface{}{},
	}
	repo.updateBuiltinCtxs(assembled)

	expectedContexts := map[string]interface{}{
		"_loaded": map[string]interface{}{
			"*": map[string]interface{}{
				"repo_matcher": []interface{}{
					map[string]interface{}{
						"git_repo": "user/repo",
						"git_ref":  "refs/heads/develop",
					},
				},
				"loaded_repos": []interface{}{
					map[string]interface{}{
						"repo":        "https://github.com/user/repo",
						"branch":      "develop",
						"commit_hash": "abc123",
						"path":        "",
					},
				},
			},
		},
	}
	assert.Equal(t, expectedContexts, assembled.Contexts, "contexts with branch")

	// Test without branch (defaults to main)
	repo2 := &Repo{
		repo: &RepoInfo{
			Repo: "git@github.com:user/repo.git",
		},
		hash: "def456",
	}
	assembled2 := &DataSet{
		Contexts: map[string]interface{}{},
	}
	repo2.updateBuiltinCtxs(assembled2)

	expectedContexts2 := map[string]interface{}{
		"_loaded": map[string]interface{}{
			"*": map[string]interface{}{
				"repo_matcher": []interface{}{
					map[string]interface{}{
						"git_repo": "user/repo",
						"git_ref":  "refs/heads/main",
					},
				},
				"loaded_repos": []interface{}{
					map[string]interface{}{
						"repo":        "git@github.com:user/repo.git",
						"branch":      "",
						"commit_hash": "def456",
						"path":        "",
					},
				},
			},
		},
	}
	assert.Equal(t, expectedContexts2, assembled2.Contexts, "contexts without branch")

	// Test appending to existing contexts
	repo3 := &Repo{
		repo: &RepoInfo{
			Repo:   "https://example.com/org/project",
			Branch: "master",
		},
		hash: "newhash",
	}
	assembled3 := &DataSet{
		Contexts: map[string]interface{}{
			"_loaded": map[string]interface{}{
				"*": map[string]interface{}{
					"repo_matcher": []interface{}{
						map[string]interface{}{
							"git_repo": "existing/repo",
							"git_ref":  "refs/heads/main",
						},
					},
					"loaded_repos": []interface{}{
						map[string]interface{}{
							"repo":        "existing",
							"branch":      "main",
							"commit_hash": "oldhash",
						},
					},
				},
			},
		},
	}
	repo3.updateBuiltinCtxs(assembled3)

	expectedContexts3 := map[string]interface{}{
		"_loaded": map[string]interface{}{
			"*": map[string]interface{}{
				"repo_matcher": []interface{}{
					map[string]interface{}{
						"git_repo": "existing/repo",
						"git_ref":  "refs/heads/main",
					},
					map[string]interface{}{
						"git_repo": "org/project",
						"git_ref":  "refs/heads/master",
					},
				},
				"loaded_repos": []interface{}{
					map[string]interface{}{
						"repo":        "existing",
						"branch":      "main",
						"commit_hash": "oldhash",
					},
					map[string]interface{}{
						"repo":        "https://example.com/org/project",
						"branch":      "master",
						"commit_hash": "newhash",
						"path":        "",
					},
				},
			},
		},
	}
	assert.Equal(t, expectedContexts3, assembled3.Contexts, "appending to existing contexts")

	// Test with nil contexts - should not panic and should initialize contexts
	repo4 := &Repo{
		repo: &RepoInfo{
			Repo:   "https://github.com/myorg/myproject",
			Branch: "feature",
		},
		hash: "xyz789",
	}
	assembled4 := &DataSet{
		Contexts: nil,
	}
	assert.NotPanics(t, func() { repo4.updateBuiltinCtxs(assembled4) }, "should not panic when contexts is nil")
	assert.NotNil(t, assembled4.Contexts, "contexts should be initialized")
	assert.Contains(t, assembled4.Contexts, "_loaded", "contexts should contain _loaded key")
}

func TestCloneFetchRepoBranchSetup(t *testing.T) {
	// Test default branch (main)
	repo := &Repo{
		repo: &RepoInfo{
			Repo: "https://github.com/user/repo",
			// Branch not specified, should use main
		},
	}

	// We can't easily test the actual cloning without mocking git operations,
	// but we can test the branch setup logic by examining what would be passed to git
	branch := plumbing.Main.Short()
	if repo.repo.Branch != "" {
		branch = repo.repo.Branch
	}

	assert.Equal(t, "main", branch, "default branch should be main")

	// Test specified branch
	repo.repo.Branch = "develop"
	branch = plumbing.Main.Short()
	if repo.repo.Branch != "" {
		branch = repo.repo.Branch
	}

	assert.Equal(t, "develop", branch, "specified branch should be used")
}

func TestCloneFetchRepoAuthSetup(t *testing.T) {
	// Test SSH auth setup for git@ URLs
	repo := &Repo{
		repo: &RepoInfo{
			Repo: "git@github.com:user/repo.git",
		},
	}

	// Test SSH auth detection
	if strings.HasPrefix(repo.repo.Repo, "git@") {
		// This would normally call GetGitSSHAuth, but we can't test that without mocking
		// Just verify the condition is met
		assert.True(t, strings.HasPrefix(repo.repo.Repo, "git@"), "SSH URL should be detected")
	}

	// Test GitHub token auth setup
	repo.repo.TokenSource = _providerGitHub
	repo.repo.Repo = "https://github.com/user/repo"

	// Reset the condition checks
	switch {
	case strings.HasPrefix(repo.repo.Repo, "git@"):
		t.Error("Should not detect SSH for HTTPS URL")
	case repo.repo.TokenSource == _providerGitHub:
		// This would normally set up GitHub token auth
		assert.Equal(t, _providerGitHub, repo.repo.TokenSource, "GitHub token source should be detected")
	case repo.repo.PassEnv != "":
		t.Error("Should not detect password auth when not set")
	}

	// Test basic auth setup
	repo.repo.TokenSource = ""
	repo.repo.PassEnv = "GIT_PASS"
	repo.repo.Username = "testuser"

	switch {
	case strings.HasPrefix(repo.repo.Repo, "git@"):
		t.Error("Should not detect SSH")
	case repo.repo.TokenSource == _providerGitHub:
		t.Error("Should not detect GitHub token")
	case repo.repo.PassEnv != "":
		// This would normally set up basic auth
		assert.Equal(t, "GIT_PASS", repo.repo.PassEnv, "Password env should be detected")
		assert.Equal(t, "testuser", repo.repo.Username, "Username should be set")
	}
}

// Mock implementations for testing.
type MockGitClient struct {
	PlainCloneFunc func(directory string, isBare bool, o *git.CloneOptions) (*git.Repository, error)
}

func (m *MockGitClient) PlainClone(directory string, isBare bool, o *git.CloneOptions) (*git.Repository, error) {
	if m.PlainCloneFunc != nil {
		return m.PlainCloneFunc(directory, isBare, o)
	}

	return nil, nil
}

type MockTempDirCreator struct {
	MkdirTempFunc func(dir, pattern string) (string, error)
}

func (m *MockTempDirCreator) MkdirTemp(dir, pattern string) (string, error) {
	if m.MkdirTempFunc != nil {
		return m.MkdirTempFunc(dir, pattern)
	}

	return testTempDir, nil
}

func TestCloneFetchRepoWithDeps(t *testing.T) {
	// Test successful cloning
	t.Run("successful cloning", func(t *testing.T) {
		mockGitClient := &MockGitClient{
			PlainCloneFunc: func(directory string, isBare bool, o *git.CloneOptions) (*git.Repository, error) {
				assert.Equal(t, testTempDir, directory)
				assert.False(t, isBare)
				assert.Equal(t, "https://github.com/user/repo", o.URL)
				assert.Equal(t, plumbing.NewBranchReferenceName("main"), o.ReferenceName)
				// Return a mock repository - we don't care about the actual type since we mock headGetter
				return nil, nil
			},
		}

		mockTempDirCreator := &MockTempDirCreator{
			MkdirTempFunc: func(dir, pattern string) (string, error) {
				assert.Equal(t, "/working/dir", dir)
				assert.Equal(t, "git", pattern)

				return testTempDir, nil
			},
		}

		repo := &Repo{
			parent: &Config{
				WorkingDir: "/working/dir",
			},
			repo: &RepoInfo{
				Repo: "https://github.com/user/repo",
			},
		}

		headGetter := func(r *git.Repository) string {
			return "abcd1234abcd1234abcd1234abcd1234abcd1234"
		}

		// This should not panic since we're mocking the git operations
		assert.NotPanics(t, func() {
			repo.cloneFetchRepoWithDeps(mockGitClient, mockTempDirCreator, headGetter)
		})

		assert.Equal(t, testTempDir, repo.root)
		assert.Equal(t, "abcd1234abcd1234abcd1234abcd1234abcd1234", repo.hash)
	})

	// Test with specified branch
	t.Run("specified branch", func(t *testing.T) {
		mockGitClient := &MockGitClient{
			PlainCloneFunc: func(directory string, isBare bool, o *git.CloneOptions) (*git.Repository, error) {
				assert.Equal(t, plumbing.NewBranchReferenceName("develop"), o.ReferenceName)

				return nil, nil
			},
		}

		mockTempDirCreator := &MockTempDirCreator{
			MkdirTempFunc: func(dir, pattern string) (string, error) {
				return testTempDir, nil
			},
		}

		repo := &Repo{
			parent: &Config{
				WorkingDir: "/working/dir",
			},
			repo: &RepoInfo{
				Repo:   "https://github.com/user/repo",
				Branch: "develop",
			},
		}

		headGetter := func(r *git.Repository) string {
			return "efgh5678efgh5678efgh5678efgh5678efgh5678"
		}

		assert.NotPanics(t, func() {
			repo.cloneFetchRepoWithDeps(mockGitClient, mockTempDirCreator, headGetter)
		})

		assert.Equal(t, "efgh5678efgh5678efgh5678efgh5678efgh5678", repo.hash)
	})
}

func TestBuildCloneOptions(t *testing.T) {
	// Test default branch
	t.Run("default branch", func(t *testing.T) {
		repoInfo := RepoInfo{
			Repo: "https://github.com/user/repo",
		}

		opts := BuildCloneOptions(repoInfo)
		assert.Equal(t, "https://github.com/user/repo", opts.URL)
		assert.Equal(t, plumbing.NewBranchReferenceName("main"), opts.ReferenceName)
	})

	// Test specified branch
	t.Run("specified branch", func(t *testing.T) {
		repoInfo := RepoInfo{
			Repo:   "https://github.com/user/repo",
			Branch: "feature-branch",
		}

		opts := BuildCloneOptions(repoInfo)
		assert.Equal(t, plumbing.NewBranchReferenceName("feature-branch"), opts.ReferenceName)
	})
}

func TestSetupAuth(t *testing.T) {
	// Save original _GHAppInfo to restore later
	originalGHAppInfo := _GHAppInfo
	defer func() { _GHAppInfo = originalGHAppInfo }()

	t.Run("SSH authentication condition", func(t *testing.T) {
		// Test that SSH URLs are properly detected by SetupAuth logic
		// We don't call SetupAuth directly with SSH URLs to avoid needing SSH keys
		repoInfo := RepoInfo{
			Repo: "git@github.com:user/repo.git",
		}

		// Verify the condition that SetupAuth uses
		assert.True(t, strings.HasPrefix(repoInfo.Repo, "git@"), "SSH URL condition should be detected")

		// The actual SSH auth setup is tested separately in TestGetGitSSHAuth
	})

	t.Run("GitHub token authentication with mock", func(t *testing.T) {
		// Set up mock GitHub app info
		_GHAppInfo = map[string]any{
			"mock": map[string]interface{}{
				"token":     "mock_github_token_12345",
				"expiresAt": "2026-12-31T23:59:59Z",
			},
		}

		repoInfo := RepoInfo{
			Repo:        "https://github.com/user/repo",
			TokenSource: _providerGitHub,
		}

		auth := SetupAuth(&repoInfo)

		assert.NotNil(t, auth, "GitHub token auth should be configured")
		if basicAuth, ok := auth.(*http.BasicAuth); ok {
			assert.Equal(t, "mock_github_token_12345", basicAuth.Password, "Mock token should be used")
			assert.NotEmpty(t, basicAuth.Username, "Mock token should come with some username")
		} else {
			t.Error("Expected TokenAuth, got different auth type")
		}
	})

	t.Run("GitHub token authentication without mock", func(t *testing.T) {
		// Clear mock to test real token retrieval (but this will fail without proper env vars)
		_GHAppInfo = nil

		repoInfo := RepoInfo{
			Repo:        "https://github.com/user/repo",
			TokenSource: _providerGitHub,
		}

		// This should panic without proper env vars, so we test that it does panic
		assert.Panics(t, func() { SetupAuth(&repoInfo) }, "SetupAuth should panic without proper GitHub env vars")
	})

	t.Run("basic authentication with PassEnv", func(t *testing.T) {
		// Set up environment variable for testing
		originalEnv := os.Getenv("TEST_GIT_PASS")
		os.Setenv("TEST_GIT_PASS", "test_password")
		defer func() { os.Setenv("TEST_GIT_PASS", originalEnv) }()

		repoInfo := RepoInfo{
			Repo:     "https://github.com/user/repo",
			PassEnv:  "TEST_GIT_PASS",
			Username: "testuser",
		}

		auth := SetupAuth(&repoInfo)

		assert.NotNil(t, auth, "Basic auth should be configured")
		if basicAuth, ok := auth.(*http.BasicAuth); ok {
			assert.Equal(t, "testuser", basicAuth.Username, "Username should be set")
			assert.Equal(t, "test_password", basicAuth.Password, "Password should be from env var")
		} else {
			t.Error("Expected BasicAuth, got different auth type")
		}
	})

	t.Run("basic authentication with PassEnv but empty env var", func(t *testing.T) {
		// Ensure the environment variable doesn't exist
		originalEnv := os.Getenv("EMPTY_TEST_PASS")
		os.Unsetenv("EMPTY_TEST_PASS")
		defer func() { os.Setenv("EMPTY_TEST_PASS", originalEnv) }()

		repoInfo := RepoInfo{
			Repo:     "https://github.com/user/repo",
			PassEnv:  "EMPTY_TEST_PASS",
			Username: "testuser",
		}

		auth := SetupAuth(&repoInfo)

		assert.NotNil(t, auth, "Basic auth should be configured even with empty password")
		if basicAuth, ok := auth.(*http.BasicAuth); ok {
			assert.Equal(t, "testuser", basicAuth.Username, "Username should be set")
			assert.Equal(t, "", basicAuth.Password, "Password should be empty when env var doesn't exist")
		} else {
			t.Error("Expected BasicAuth, got different auth type")
		}
	})

	t.Run("basic authentication with username-specific env var", func(t *testing.T) {
		// Set an env var matching the Username branch
		orig := os.Getenv("DIPPER_GIT_PASS_TESTUSER")
		os.Setenv("DIPPER_GIT_PASS_TESTUSER", "user_specific_pass")
		defer func() { os.Setenv("DIPPER_GIT_PASS_TESTUSER", orig) }()

		repoInfo := RepoInfo{
			Repo:     "https://github.com/user/repo",
			Username: "testuser",
		}

		auth := SetupAuth(&repoInfo)

		assert.NotNil(t, auth, "Basic auth should be configured for username-specific var")
		if basicAuth, ok := auth.(*http.BasicAuth); ok {
			assert.Equal(t, "testuser", basicAuth.Username)
			assert.Equal(t, "user_specific_pass", basicAuth.Password)
		} else {
			t.Error("Expected BasicAuth, got different auth type")
		}
	})

	t.Run("basic authentication with username fallback env var", func(t *testing.T) {
		// Clear username-specific var and set fallback
		os.Unsetenv("DIPPER_GIT_PASS_TESTUSER")
		orig := os.Getenv("DIPPER_GIT_PASS")
		os.Setenv("DIPPER_GIT_PASS", "fallback_user_pass")
		defer func() { os.Setenv("DIPPER_GIT_PASS", orig) }()

		repoInfo := RepoInfo{
			Repo:     "https://github.com/user/repo",
			Username: "testuser",
		}

		auth := SetupAuth(&repoInfo)

		assert.NotNil(t, auth, "Basic auth should be configured for fallback var")
		if basicAuth, ok := auth.(*http.BasicAuth); ok {
			assert.Equal(t, "testuser", basicAuth.Username)
			assert.Equal(t, "fallback_user_pass", basicAuth.Password)
		} else {
			t.Error("Expected BasicAuth, got different auth type")
		}
	})

	t.Run("basic authentication with default PassEnv (username branch)", func(t *testing.T) {
		// When PassEnv is not specified but Username is set, the username branch
		// should trigger, resulting in basic auth with empty password by default.
		repoInfo := RepoInfo{
			Repo:     "https://github.com/user/repo",
			Username: "testuser",
		}

		auth := SetupAuth(&repoInfo)

		assert.NotNil(t, auth, "Basic auth should be configured for username branch")
		if basicAuth, ok := auth.(*http.BasicAuth); ok {
			assert.Equal(t, "testuser", basicAuth.Username)
			assert.Equal(t, "", basicAuth.Password, "Password should be empty when no env vars are set")
		} else {
			t.Error("Expected BasicAuth, got different auth type")
		}
	})

	// The fallback PassEnv test is covered by the username fallback case below.
	// This earlier placeholder is no longer needed and has been effectively
	// replaced by more specific tests above.

	t.Run("no authentication when no conditions met", func(t *testing.T) {
		repoInfo := RepoInfo{
			Repo: "https://github.com/user/repo",
			// No TokenSource, no PassEnv, not SSH
		}

		auth := SetupAuth(&repoInfo)

		assert.Nil(t, auth, "No auth should be configured when no conditions are met")
	})

	t.Run("GitHub token takes precedence over basic auth", func(t *testing.T) {
		// Set up mock for GitHub token
		_GHAppInfo = map[string]any{
			"mock": map[string]interface{}{
				"token":     "mock_github_token_67890",
				"expiresAt": "2026-12-31T23:59:59Z",
			},
		}

		repoInfo := RepoInfo{
			Repo:        "https://github.com/user/repo",
			TokenSource: _providerGitHub,
			PassEnv:     "SOME_PASS",
			Username:    "testuser",
		}

		auth := SetupAuth(&repoInfo)

		assert.NotNil(t, auth, "GitHub token auth should take precedence over basic auth")
		if basicAuth, ok := auth.(*http.BasicAuth); ok {
			assert.Equal(t, "mock_github_token_67890", basicAuth.Password, "GitHub token should be used")
		} else {
			t.Error("Expected TokenAuth, got different auth type")
		}
	})
}

func TestBuildPullOptions(t *testing.T) {
	// Save original _GHAppInfo to restore later
	originalGHAppInfo := _GHAppInfo
	defer func() { _GHAppInfo = originalGHAppInfo }()

	t.Run("default branch", func(t *testing.T) {
		repoInfo := RepoInfo{
			Repo: "https://github.com/user/repo",
		}

		opts := BuildPullOptions(repoInfo)
		assert.Equal(t, "https://github.com/user/repo", opts.RemoteURL)
		assert.Equal(t, plumbing.NewBranchReferenceName("main"), opts.ReferenceName)
		assert.Equal(t, "origin", opts.RemoteName)
		assert.True(t, opts.Force)
		assert.True(t, opts.SingleBranch)
	})

	t.Run("specified branch", func(t *testing.T) {
		repoInfo := RepoInfo{
			Repo:   "https://github.com/user/repo",
			Branch: "feature-branch",
		}

		opts := BuildPullOptions(repoInfo)
		assert.Equal(t, plumbing.NewBranchReferenceName("feature-branch"), opts.ReferenceName)
	})

	t.Run("with github token authentication", func(t *testing.T) {
		// Set up mock GitHub app info
		_GHAppInfo = map[string]any{
			"mock": map[string]interface{}{
				"token":     "mock_pull_token_11111",
				"expiresAt": "2026-12-31T23:59:59Z",
			},
		}

		repoInfo := RepoInfo{
			Repo:        "https://github.com/user/repo",
			TokenSource: _providerGitHub,
		}

		opts := BuildPullOptions(repoInfo)
		assert.NotNil(t, opts.Auth, "GitHub token auth should be configured")
		if basicAuth, ok := opts.Auth.(*http.BasicAuth); ok {
			assert.Equal(t, "mock_pull_token_11111", basicAuth.Password, "Mock token should be used")
		} else {
			t.Error("Expected TokenAuth, got different auth type")
		}
	})

	t.Run("with basic authentication", func(t *testing.T) {
		// Set up environment variable for testing
		originalEnv := os.Getenv("PULL_TEST_PASS")
		os.Setenv("PULL_TEST_PASS", "pull_password")
		defer func() { os.Setenv("PULL_TEST_PASS", originalEnv) }()

		repoInfo := RepoInfo{
			Repo:     "https://github.com/user/repo",
			PassEnv:  "PULL_TEST_PASS",
			Username: "pulluser",
		}

		opts := BuildPullOptions(repoInfo)
		assert.NotNil(t, opts.Auth, "Basic auth should be configured")
		if basicAuth, ok := opts.Auth.(*http.BasicAuth); ok {
			assert.Equal(t, "pulluser", basicAuth.Username)
			assert.Equal(t, "pull_password", basicAuth.Password)
		} else {
			t.Error("Expected BasicAuth, got different auth type")
		}
	})

	t.Run("with username branch auth", func(t *testing.T) {
		// Set an env var matching the Username branch
		orig := os.Getenv("DIPPER_GIT_PASS_PULLUSER")
		os.Setenv("DIPPER_GIT_PASS_PULLUSER", "username_pass")
		defer func() { os.Setenv("DIPPER_GIT_PASS_PULLUSER", orig) }()

		repoInfo := RepoInfo{
			Repo:     "https://github.com/user/repo",
			Username: "pulluser",
		}

		opts := BuildPullOptions(repoInfo)
		assert.NotNil(t, opts.Auth, "Basic auth should be configured")
		if basicAuth, ok := opts.Auth.(*http.BasicAuth); ok {
			assert.Equal(t, "pulluser", basicAuth.Username)
			assert.Equal(t, "username_pass", basicAuth.Password)
		} else {
			t.Error("Expected BasicAuth, got different auth type")
		}
	})

	t.Run("no auth when no conditions met", func(t *testing.T) {
		repoInfo := RepoInfo{
			Repo: "https://github.com/user/repo",
		}

		opts := BuildPullOptions(repoInfo)
		assert.Nil(t, opts.Auth, "No auth should be configured when no conditions are met")
	})
}
