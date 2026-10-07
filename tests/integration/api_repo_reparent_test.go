// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"testing"

	auth_model "gitea.dev/models/auth"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	api "gitea.dev/modules/structs"
	"gitea.dev/tests"

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
	
	// Make repo2 public to satisfy visibility constraints
	repo2.IsPrivate = false
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo2, "is_private"))

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

func TestAPIRepoReparentPendingAndAccept(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	session2 := loginUser(t, user2.Name)
	token2 := getTokenForLoggedInUser(t, session2, auth_model.AccessTokenScopeWriteRepository)

	user13 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 13})
	session13 := loginUser(t, user13.Name)
	token13 := getTokenForLoggedInUser(t, session13, auth_model.AccessTokenScopeWriteRepository)

	// Reset repo1 to be a root repository
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo1.IsFork = false
	repo1.ForkID = 0
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo1, "is_fork", "fork_id"))

	// user2 initiates reparenting to a non-existent parent under user13
	req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/reparent", user2.Name, repo1.Name), &api.ReparentRepoOption{
		NewParent: user13.Name,
		NewName:   "new-parent-api-pending",
	}).AddTokenAuth(token2)
	MakeRequest(t, req, http.StatusCreated)

	// Target fork should not exist in target namespace prior to authorization
	unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerID: 13, Name: "new-parent-api-pending"})

	// Initiator user2 cannot accept (unauthorized for target namespace)
	req = NewRequest(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/reparent/accept", user2.Name, repo1.Name)).AddTokenAuth(token2)
	MakeRequest(t, req, http.StatusForbidden)
	unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerID: 13, Name: "new-parent-api-pending"})

	// Target owner user13 accepts, creating the fork and completing reparenting
	req = NewRequest(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/reparent/accept", user2.Name, repo1.Name)).AddTokenAuth(token13)
	MakeRequest(t, req, http.StatusAccepted)

	// Verify target parent repository now exists and repo1 is its fork
	newParent := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerID: 13, Name: "new-parent-api-pending"})
	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	assert.True(t, repo1.IsFork)
	assert.Equal(t, newParent.ID, repo1.ForkID)
	assert.False(t, newParent.IsFork)
	assert.Equal(t, repo_model.RepositoryReady, newParent.Status)
	assert.Equal(t, repo_model.RepositoryReady, repo1.Status)
}

func TestAPIRepoReparentOldParentForkCount(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	session2 := loginUser(t, user2.Name)
	token2 := getTokenForLoggedInUser(t, session2, auth_model.AccessTokenScopeWriteRepository)

	user13 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 13})
	session13 := loginUser(t, user13.Name)
	token13 := getTokenForLoggedInUser(t, session13, auth_model.AccessTokenScopeWriteRepository)

	// repo11 is owned by user13, and is a fork of repo10
	repo10 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	repo11 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11})
	initialForks := repo10.NumForks

	// user13 initiates reparenting to a new repo under user2
	req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/reparent", user13.Name, repo11.Name), &api.ReparentRepoOption{
		NewParent: user2.Name,
		NewName:   "new-parent-api-forkcount",
	}).AddTokenAuth(token13)
	MakeRequest(t, req, http.StatusCreated)

	// user2 accepts reparent
	req = NewRequest(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/reparent/accept", user13.Name, repo11.Name)).AddTokenAuth(token2)
	MakeRequest(t, req, http.StatusAccepted)

	// Verify old parent's fork count is decremented
	repo10 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	assert.Equal(t, initialForks-1, repo10.NumForks)
}
