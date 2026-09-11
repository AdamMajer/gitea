// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"testing"

	activities_model "gitea.dev/models/activities"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"github.com/stretchr/testify/assert"
)

func TestReparentService(t *testing.T) {
	registerNotifier()

	assert.NoError(t, unittest.PrepareTestDatabase())

	ctx := t.Context()
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user13 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 13})

	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})   // Owned by user2 (ID 2), root repo, NumForks=0
	repo10 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10}) // Owned by user12 (ID 12), root repo, NumForks=1 (repotest shows repo11 is fork)
	repo11 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11}) // Owned by user13 (ID 13), fork of repo10 (ID 10)

	// 1. Test standard reparenting when target exists: repo1 (user2) reparents to repo10 (user12)
	// Since user2 is owner of repo1, and repo10 is NOT a fork of repo1, this should be direct (no pending request)
	assert.NoError(t, StartRepositoryReparent(ctx, user2, repo1, repo10, nil, ""))

	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo10 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	assert.True(t, repo1.IsFork)
	assert.Equal(t, repo10.ID, repo1.ForkID)
	assert.Equal(t, 2, repo10.NumForks) // repo10 had 1 fork (repo11), now has 2

	// 2. Test standard reparenting from an existing fork when target exists:
	// repo11 is a fork of repo10. We reparent repo11 to repo1.
	// Initiator is user13 (owner of repo11).
	assert.NoError(t, StartRepositoryReparent(ctx, user13, repo11, repo1, nil, ""))

	repo11 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11})
	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo10 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})

	assert.True(t, repo11.IsFork)
	assert.Equal(t, repo1.ID, repo11.ForkID)
	assert.Equal(t, 1, repo1.NumForks)  // repo1 had 0, now has 1 (repo11)
	assert.Equal(t, 1, repo10.NumForks) // repo10 had 2, now has 1 (repo1)

	// 3. Test swap direct when target exists:
	// Let's create a scenario where user2 owns both repo1 and another repo (repo2 ID 2).
	// We make repo2 a fork of repo1 first.
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	assert.NoError(t, repo_model.ReparentToExistingParent(ctx, repo1.ID, repo2.ID, 0)) // repo2 becomes fork of repo1

	repo2 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	assert.True(t, repo2.IsFork)
	assert.Equal(t, repo1.ID, repo2.ForkID)

	// Now we swap them: user2 reparents repo1 to point to repo2 as parent.
	// Since user2 owns both repo1 and repo2, it should be direct.
	assert.NoError(t, StartRepositoryReparent(ctx, user2, repo1, repo2, nil, ""))

	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo2 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	assert.True(t, repo1.IsFork)
	assert.Equal(t, repo2.ID, repo1.ForkID)
	assert.False(t, repo2.IsFork)
	assert.Equal(t, int64(0), repo2.ForkID)
	assert.Equal(t, 1, repo1.NumForks)
	assert.Equal(t, 1, repo2.NumForks)

	// 4. Test swap pending when target exists:
	// We reset repo2 to be a fork of repo1 again.
	assert.NoError(t, repo_model.ReparentToExistingParent(ctx, repo1.ID, repo2.ID, 0))

	// Now user2 (owner of repo1) reparents repo1 to repo11 (owned by user13).
	// First make repo11 a fork of repo1.
	assert.NoError(t, repo_model.ReparentToExistingParent(ctx, repo1.ID, repo11.ID, repo11.ForkID))

	repo11 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11})
	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	assert.True(t, repo11.IsFork)
	assert.Equal(t, repo1.ID, repo11.ForkID)

	// Now user2 (owner of repo1) reparents repo1 to repo11 (owned by user13).
	// Since user2 does not own repo11, this must be pending!
	assert.NoError(t, StartRepositoryReparent(ctx, user2, repo1, repo11, nil, ""))

	// Verify repo1 is locked (pending status)
	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	assert.Equal(t, repo_model.RepositoryPendingReparent, repo1.Status)

	reparent, err := repo_model.GetPendingReparentByRepo(ctx, repo1.ID)
	assert.NoError(t, err)
	assert.NotNil(t, reparent)

	// Reject reparenting as user13 (owner of repo11)
	assert.NoError(t, RejectReparent(ctx, user13, repo1))
	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	assert.Equal(t, repo_model.RepositoryReady, repo1.Status)

	// Start again and accept
	assert.NoError(t, StartRepositoryReparent(ctx, user2, repo1, repo11, nil, ""))
	assert.NoError(t, AcceptReparent(ctx, user13, repo1))

	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo11 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11})
	assert.True(t, repo1.IsFork)
	assert.Equal(t, repo11.ID, repo1.ForkID)
	assert.False(t, repo11.IsFork)
	assert.Equal(t, int64(0), repo11.ForkID)

	// 5. Test reverse fork direct when target does NOT exist:
	// Unfork repo2 so user2 has no existing forks of repo1
	repo2 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	repo2.IsFork = false
	repo2.ForkID = 0
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo2, "is_fork", "fork_id"))

	// We reset repo1 to be root.
	repo1.IsFork = false
	repo1.ForkID = 0
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo1, "is_fork", "fork_id"))

	// user2 reparents repo1 to point to a non-existent parent owned by user2 (user2.Name, "new-parent-direct").
	// Since user2 is target owner and initiator, it should create the new-parent-direct repository and swap directly.
	assert.NoError(t, StartRepositoryReparent(ctx, user2, repo1, nil, user2, "new-parent-direct"))

	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	newParentDirect := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerID: 2, Name: "new-parent-direct"})
	assert.True(t, repo1.IsFork)
	assert.Equal(t, newParentDirect.ID, repo1.ForkID)
	assert.False(t, newParentDirect.IsFork)
	assert.Equal(t, int64(0), newParentDirect.ForkID)

	// Clean up newParentDirect
	assert.NoError(t, DeleteRepositoryDirectly(ctx, newParentDirect.ID))

	// 6. Test reverse fork pending when target does NOT exist:
	// We reset repo1 to be root.
	repo1.IsFork = false
	repo1.ForkID = 0
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo1, "is_fork", "fork_id"))

	// user2 reparents repo1 to point to a non-existent parent owned by user13 (user13.Name, "new-parent-pending").
	// Since user2 is not an admin/owner of user13, this must be pending!
	assert.NoError(t, StartRepositoryReparent(ctx, user2, repo1, nil, user13, "new-parent-pending"))

	// Verify repo1 is locked (pending status)
	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	assert.Equal(t, repo_model.RepositoryPendingReparent, repo1.Status)

	// Verify new-parent-pending exists but is locked as RepositoryPendingReparent
	newParentPending := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerID: 13, Name: "new-parent-pending"})
	assert.Equal(t, repo_model.RepositoryPendingReparent, newParentPending.Status)

	reparent, err = repo_model.GetPendingReparentByRepo(ctx, repo1.ID)
	assert.NoError(t, err)
	assert.NotNil(t, reparent)
	assert.Equal(t, newParentPending.ID, reparent.TargetParentID)

	// Reject reparenting as user13 (owner of target)
	assert.NoError(t, RejectReparent(ctx, user13, repo1))

	// Verify repo1 is ready and new-parent-pending has been DELETED
	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	assert.Equal(t, repo_model.RepositoryReady, repo1.Status)
	unittest.AssertNotExistsBean(t, &repo_model.Repository{ID: newParentPending.ID})

	// Start again and accept
	assert.NoError(t, StartRepositoryReparent(ctx, user2, repo1, nil, user13, "new-parent-pending"))
	newParentPending = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerID: 13, Name: "new-parent-pending"})

	assert.NoError(t, AcceptReparent(ctx, user13, repo1))

	repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	newParentPending = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: newParentPending.ID})

	assert.True(t, repo1.IsFork)
	assert.Equal(t, newParentPending.ID, repo1.ForkID)
	assert.False(t, newParentPending.IsFork)
	assert.Equal(t, int64(0), newParentPending.ForkID)
	assert.Equal(t, repo_model.RepositoryReady, newParentPending.Status)

	// Clean up newParentPending
	assert.NoError(t, DeleteRepositoryDirectly(ctx, newParentPending.ID))

	// Verify that the timeline action was created correctly
	unittest.AssertExistsAndLoadBean(t, &activities_model.Action{
		OpType:    activities_model.ActionReparentRepo,
		ActUserID: 13,
		RepoID:    1,
		Content:   "user13/new-parent-pending",
	})
}
