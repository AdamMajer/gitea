// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/globallock"
	"gitea.dev/modules/util"
)

// StartRepositoryReparent starts the reparenting process for a repository
func StartRepositoryReparent(ctx context.Context, doer *user_model.User, source, target *repo_model.Repository) error {
	releaser, err := globallock.Lock(ctx, getRepoWorkingLockKey(source.ID))
	if err != nil {
		return fmt.Errorf("lock.Lock: %w", err)
	}
	defer releaser()

	if source.Status == repo_model.RepositoryBeingMigrated {
		return fmt.Errorf("repo is not ready, currently migrating")
	}

	// For reparenting, we always require acceptance unless the doer is admin and owner of both?
	// Actually, the TODO says "use the same mechanism to ask the source repository if it should be reparented".
	// So we always create a pending request if the doer is not the owner of the source.

	if err := source.LoadOwner(ctx); err != nil {
		return err
	}

	isDirect := false
	if doer.IsAdmin || source.OwnerID == doer.ID {
		isDirect = true
	}

	if isDirect {
		return repo_model.ReparentFork(ctx, target.ID, source.ID)
	}

	return repo_model.CreatePendingReparent(ctx, doer, source.ID, target.ID)
}

// AcceptReparent accepts a reparenting request
func AcceptReparent(ctx context.Context, doer *user_model.User, source *repo_model.Repository) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		reparent, err := repo_model.GetPendingReparentByRepo(ctx, source.ID)
		if err != nil {
			return err
		}

		if !reparent.CanUserAcceptOrRejectReparent(ctx, doer) {
			return util.ErrPermissionDenied
		}

		if err := repo_model.ReparentFork(ctx, reparent.TargetParentID, source.ID); err != nil {
			return err
		}

		source.Status = repo_model.RepositoryReady
		if err := repo_model.UpdateRepositoryColsNoAutoTime(ctx, source, "status"); err != nil {
			return err
		}

		return repo_model.DeleteReparent(ctx, source.ID)
	})
}

// RejectReparent rejects a reparenting request
func RejectReparent(ctx context.Context, doer *user_model.User, source *repo_model.Repository) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		reparent, err := repo_model.GetPendingReparentByRepo(ctx, source.ID)
		if err != nil {
			return err
		}

		if !reparent.CanUserAcceptOrRejectReparent(ctx, doer) {
			return util.ErrPermissionDenied
		}

		source.Status = repo_model.RepositoryReady
		if err := repo_model.UpdateRepositoryColsNoAutoTime(ctx, source, "status"); err != nil {
			return err
		}

		return repo_model.DeleteReparent(ctx, source.ID)
	})
}
