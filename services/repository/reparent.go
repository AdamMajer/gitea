// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/globallock"
	"code.gitea.io/gitea/modules/util"
	notify_service "code.gitea.io/gitea/services/notify"
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

	if err := source.LoadOwner(ctx); err != nil {
		return err
	}

	isDirect := false
	if doer.IsAdmin || source.OwnerID == doer.ID {
		isDirect = true
	}

	if isDirect {
		oldOwnerName := ""
		if source.ForkID > 0 {
			if oldParent, err := repo_model.GetRepositoryByID(ctx, source.ForkID); err == nil {
				oldOwnerName = oldParent.OwnerName
			}
		} else if source.BaseRepo != nil {
			oldOwnerName = source.BaseRepo.OwnerName
		}

		if err := repo_model.ReparentFork(ctx, target.ID, source.ID); err != nil {
			return err
		}
		notify_service.ReparentRepository(ctx, doer, source, oldOwnerName)
		return nil
	}

	if err := repo_model.CreatePendingReparent(ctx, doer, source.ID, target.ID); err != nil {
		return err
	}

	notify_service.RepoPendingReparent(ctx, doer, source, target)
	return nil
}

// AcceptReparent accepts a reparenting request
func AcceptReparent(ctx context.Context, doer *user_model.User, source *repo_model.Repository) error {
	oldOwnerName := ""
	if source.ForkID > 0 {
		if oldParent, err := repo_model.GetRepositoryByID(ctx, source.ForkID); err == nil {
			oldOwnerName = oldParent.OwnerName
		}
	} else if source.BaseRepo != nil {
		oldOwnerName = source.BaseRepo.OwnerName
	}

	if err := db.WithTx(ctx, func(ctx context.Context) error {
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
	}); err != nil {
		return err
	}

	notify_service.ReparentRepository(ctx, doer, source, oldOwnerName)
	return nil
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
