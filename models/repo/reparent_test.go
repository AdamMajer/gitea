// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"testing"

	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"github.com/stretchr/testify/assert"
)

func TestRepoReparent(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	ctx := t.Context()
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	repo1 := unittest.AssertExistsAndLoadBean(t, &Repository{ID: 1})
	repo2 := unittest.AssertExistsAndLoadBean(t, &Repository{ID: 2})

	// Create pending reparent
	assert.NoError(t, CreatePendingReparent(ctx, user2, repo1.ID, repo2.ID))

	// Should not be able to create another one
	err := CreatePendingReparent(ctx, user2, repo1.ID, repo2.ID)
	assert.Error(t, err)
	assert.True(t, IsErrRepoReparentInProgress(err))

	// Get pending reparent
	reparent, err := GetPendingReparentByRepo(ctx, repo1.ID)
	assert.NoError(t, err)
	assert.NotNil(t, reparent)
	assert.Equal(t, repo1.ID, reparent.SourceRepoID)
	assert.Equal(t, repo2.ID, reparent.TargetParentID)

	// Load attributes
	assert.NoError(t, reparent.LoadAttributes(ctx))
	assert.NotNil(t, reparent.SourceRepo)
	assert.NotNil(t, reparent.TargetParent)
	assert.NotNil(t, reparent.Doer)

	// Check permissions
	assert.True(t, reparent.CanUserAcceptOrRejectReparent(ctx, user2))
	user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
	assert.False(t, reparent.CanUserAcceptOrRejectReparent(ctx, user5))

	// Verify repo1 status is RepositoryPendingReparent before deletion
	repo1, err = GetRepositoryByID(ctx, repo1.ID)
	assert.NoError(t, err)
	assert.Equal(t, RepositoryPendingReparent, repo1.Status)

	// Delete reparent
	assert.NoError(t, DeleteReparent(ctx, repo1.ID))
	_, err = GetPendingReparentByRepo(ctx, repo1.ID)
	assert.Error(t, err)
	assert.True(t, IsErrNoPendingReparent(err))

	// Repo status should be back to ready after deletion
	repo1, err = GetRepositoryByID(ctx, repo1.ID)
	assert.NoError(t, err)
	assert.Equal(t, RepositoryReady, repo1.Status)
}
