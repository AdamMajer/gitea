// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

func TestAPIRepoReparent(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Use user2 (owner of both repo1 and repo2)
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	session := loginUser(t, user2.Name)
	token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeWriteRepository)

	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})

	// Start reparenting repo1 to become a fork of repo2
	req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/reparent", user2.Name, repo1.Name), &api.ReparentRepoOption{
		NewParent: user2.Name,
		NewName:   repo2.Name,
	}).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusAccepted)

	// Verify database state
	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	assert.True(t, repo1.IsFork)
	assert.Equal(t, repo2.ID, repo1.ForkID)
}

func TestAPIRepoReparentPermissions(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	// user10 (unrelated) tries to initiate reparent of user2/repo1 to repo2
	user10 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 10})
	session10 := loginUser(t, user10.Name)
	token10 := getTokenForLoggedInUser(t, session10, auth_model.AccessTokenScopeWriteRepository)

	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})

	req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/reparent", user2.Name, repo1.Name), &api.ReparentRepoOption{
		NewParent: user2.Name,
		NewName:   repo2.Name,
	}).AddTokenAuth(token10)
	MakeRequest(t, req, http.StatusForbidden)
}

func TestAPIRepoReparentNotExist(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	session2 := loginUser(t, user2.Name)
	token2 := getTokenForLoggedInUser(t, session2, auth_model.AccessTokenScopeWriteRepository)

	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	// Now try to reparent user2/repo1 to a non-existent parent repository
	req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/reparent", user2.Name, repo1.Name), &api.ReparentRepoOption{
		NewParent: "non-existent-owner",
		NewName:   "non-existent-repo",
	}).AddTokenAuth(token2)
	MakeRequest(t, req, http.StatusNotFound)
}
