// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
