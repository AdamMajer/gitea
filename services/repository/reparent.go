// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"

	"gitea.dev/models/db"
	"gitea.dev/models/organization"
	perm_model "gitea.dev/models/perm"
	access_model "gitea.dev/models/perm/access"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unit"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/globallock"
	"gitea.dev/modules/log"
	"gitea.dev/modules/util"
	notify_service "gitea.dev/services/notify"
)

func isTargetAdmin(ctx context.Context, doer, targetOwner *user_model.User) (bool, error) {
	if doer.IsAdmin {
		return true, nil
	}
	if doer.ID == targetOwner.ID {
		return true, nil
	}
	if targetOwner.IsOrganization() {
		org := organization.OrgFromUser(targetOwner)
		return org.IsOwnedBy(ctx, doer.ID)
	}
	return false, nil
}

// StartRepositoryReparent starts the reparenting process for a repository
func StartRepositoryReparent(ctx context.Context, doer *user_model.User, source, target *repo_model.Repository, targetOwner *user_model.User, targetName string) error {
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

	// The initiator must be owner of the current repository (or admin)
	if !doer.IsAdmin && source.OwnerID != doer.ID {
		return util.ErrPermissionDenied
	}

	if target == nil {
		// Target Parent does NOT exist: Create a reverse fork!
		if targetOwner == nil || targetName == "" {
			return fmt.Errorf("target owner and name must be specified when target does not exist")
		}

		// Check if doer is target admin
		isDirect, err := isTargetAdmin(ctx, doer, targetOwner)
		if err != nil {
			return err
		}

		if isDirect {
			// Create the fork directly under targetOwner
			target, err = ForkRepository(ctx, doer, targetOwner, ForkRepoOptions{
				BaseRepo:    source,
				Name:        targetName,
				Description: source.Description,
			})
			if err != nil {
				return err
			}

			if err := repo_model.ReparentFork(ctx, target.ID, source.ID); err != nil {
				return err
			}

			notify_service.ReparentRepository(ctx, doer, source, target)
			return nil
		}

		// Prevent unauthorized creation of dummy repositories in another namespace
		return util.ErrPermissionDenied
	}

	if err := target.LoadOwner(ctx); err != nil {
		return err
	}

	// Verify that the initiator has read access to the target repository
	if hasAccess, err := access_model.HasAccessUnit(ctx, doer, target, unit.TypeCode, perm_model.AccessModeRead); err != nil {
		return err
	} else if !hasAccess {
		return util.ErrPermissionDenied
	}

	// Enforce visibility constraint: a public repository cannot be a fork of a private repository
	if target.IsPrivate && !source.IsPrivate {
		return fmt.Errorf("a public repository cannot be a fork of a private repository")
	}

	// Check if target is currently a fork of source (Swap relationship)
	if target.ForkID == source.ID && target.IsFork {
		isDirect := false
		if doer.IsAdmin || (source.OwnerID == doer.ID && target.OwnerID == doer.ID) {
			isDirect = true
		}

		if isDirect {
			if err := repo_model.ReparentFork(ctx, target.ID, source.ID); err != nil {
				return err
			}
			notify_service.ReparentRepository(ctx, doer, source, target)
			return nil
		}

		if err := repo_model.CreatePendingReparent(ctx, doer, source.ID, target.ID); err != nil {
			return err
		}

		notify_service.RepoPendingReparent(ctx, doer, source, target)
		return nil
	}

	// Standard Reparenting (target is not a fork of source)
	// We reparent source to point to target as parent directly without target's approval
	if err := repo_model.ReparentToExistingParent(ctx, target.ID, source.ID, source.ForkID); err != nil {
		return err
	}

	notify_service.ReparentRepository(ctx, doer, source, target)
	return nil
}

// AcceptReparent accepts a reparenting request
func AcceptReparent(ctx context.Context, doer *user_model.User, source *repo_model.Repository) error {
	var targetRepo *repo_model.Repository

	if err := db.WithTx(ctx, func(ctx context.Context) error {
		reparent, err := repo_model.GetPendingReparentByRepo(ctx, source.ID)
		if err != nil {
			return err
		}

		if !reparent.CanUserAcceptOrRejectReparent(ctx, doer) {
			return util.ErrPermissionDenied
		}

		var errGet error
		targetRepo, errGet = repo_model.GetRepositoryByID(ctx, reparent.TargetParentID)
		if errGet != nil {
			return errGet
		}

		if err := repo_model.ReparentFork(ctx, reparent.TargetParentID, source.ID); err != nil {
			return err
		}

		// Unlock target if it was pending
		if targetRepo.Status == repo_model.RepositoryPendingReparent {
			targetRepo.Status = repo_model.RepositoryReady
			if err := repo_model.UpdateRepositoryColsNoAutoTime(ctx, targetRepo, "status"); err != nil {
				return err
			}
		}

		source.Status = repo_model.RepositoryReady
		if err := repo_model.UpdateRepositoryColsNoAutoTime(ctx, source, "status"); err != nil {
			return err
		}

		return repo_model.DeleteReparent(ctx, source.ID)
	}); err != nil {
		return err
	}

	notify_service.ReparentRepository(ctx, doer, source, targetRepo)
	return nil
}

// RejectReparent rejects a reparenting request
func RejectReparent(ctx context.Context, doer *user_model.User, source *repo_model.Repository) error {
	var targetRepoID int64

	if err := db.WithTx(ctx, func(ctx context.Context) error {
		reparent, err := repo_model.GetPendingReparentByRepo(ctx, source.ID)
		if err != nil {
			return err
		}

		if !reparent.CanUserAcceptOrRejectReparent(ctx, doer) && source.OwnerID != doer.ID {
			return util.ErrPermissionDenied
		}

		targetRepoID = reparent.TargetParentID

		source.Status = repo_model.RepositoryReady
		if err := repo_model.UpdateRepositoryColsNoAutoTime(ctx, source, "status"); err != nil {
			return err
		}

		return repo_model.DeleteReparent(ctx, source.ID)
	}); err != nil {
		return err
	}

	if targetRepoID > 0 {
		targetRepo, err := repo_model.GetRepositoryByID(ctx, targetRepoID)
		if err == nil && targetRepo.Status == repo_model.RepositoryPendingReparent {
			if err := DeleteRepositoryDirectly(ctx, targetRepoID); err != nil {
				log.Error("DeleteRepositoryDirectly: %v", err)
			}
		}
	}

	return nil
}
