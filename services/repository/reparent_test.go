// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/stretchr/testify/assert"
)

func TestReparentService(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	ctx := t.Context()
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})

	// Start reparenting as user5 (who doesn't own repo1)
	assert.NoError(t, StartRepositoryReparent(ctx, user5, repo1, repo2))

	// Verify it's pending
	reparent, err := repo_model.GetPendingReparentByRepo(ctx, repo1.ID)
	assert.NoError(t, err)
	assert.NotNil(t, reparent)
	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	assert.Equal(t, repo_model.RepositoryPendingReparent, repo1.Status)

	// Reject reparenting as user2 (owner)
	assert.NoError(t, RejectReparent(ctx, user2, repo1))
	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	assert.Equal(t, repo_model.RepositoryReady, repo1.Status)
	_, err = repo_model.GetPendingReparentByRepo(ctx, repo1.ID)
	assert.Error(t, err)

	// Start again and accept
	assert.NoError(t, StartRepositoryReparent(ctx, user5, repo1, repo2))
	assert.NoError(t, AcceptReparent(ctx, user2, repo1))
	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	assert.Equal(t, repo_model.RepositoryReady, repo1.Status)
	
	// Verify database changes for reparenting
	repo1Updated := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo2Updated := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	assert.Equal(t, repo2Updated.ID, repo1Updated.ForkID)
	assert.True(t, repo1Updated.IsFork)
	assert.Equal(t, int64(0), repo2Updated.ForkID)
	assert.False(t, repo2Updated.IsFork)
}
