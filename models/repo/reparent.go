// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"errors"
	"fmt"

	"gitea.dev/models/db"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/log"
	"gitea.dev/modules/timeutil"
	"gitea.dev/modules/util"
)

// ErrNoPendingRepoReparent is an error type for repositories without a pending
// reparenting request
type ErrNoPendingRepoReparent struct {
	RepoID int64
}

func (err ErrNoPendingRepoReparent) Error() string {
	return fmt.Sprintf("repository doesn't have a pending reparenting request [repo_id: %d]", err.RepoID)
}

// IsErrNoPendingReparent is an error type when a repository has no pending
// reparenting requests
func IsErrNoPendingReparent(err error) bool {
	_, ok := err.(ErrNoPendingRepoReparent)
	return ok
}

func (err ErrNoPendingRepoReparent) Unwrap() error {
	return util.ErrNotExist
}

// ErrRepoReparentInProgress represents the state of a repository that has an
// ongoing reparenting request
type ErrRepoReparentInProgress struct {
	Uname string
	Name  string
}

// IsErrRepoReparentInProgress checks if an error is a ErrRepoReparentInProgress.
func IsErrRepoReparentInProgress(err error) bool {
	_, ok := err.(ErrRepoReparentInProgress)
	return ok
}

func (err ErrRepoReparentInProgress) Error() string {
	return fmt.Sprintf("repository is already being reparented [uname: %s, name: %s]", err.Uname, err.Name)
}

func (err ErrRepoReparentInProgress) Unwrap() error {
	return util.ErrAlreadyExist
}

// RepoReparent is used to manage repository reparenting requests
type RepoReparent struct {
	ID             int64 `xorm:"pk autoincr"`
	DoerID         int64
	Doer           *user_model.User `xorm:"-"`
	SourceRepoID   int64            `xorm:"UNIQUE(s) INDEX"` // The repo to be demoted to a fork
	SourceRepo     *Repository      `xorm:"-"`
	TargetParentID int64            `xorm:"INDEX"` // The fork that will become the parent
	TargetParent   *Repository      `xorm:"-"`

	CreatedUnix timeutil.TimeStamp `xorm:"INDEX NOT NULL created"`
	UpdatedUnix timeutil.TimeStamp `xorm:"INDEX NOT NULL updated"`
}

func init() {
	db.RegisterModel(new(RepoReparent))
}

func (r *RepoReparent) LoadSourceRepo(ctx context.Context) error {
	if r.SourceRepo == nil {
		repo, err := GetRepositoryByID(ctx, r.SourceRepoID)
		if err != nil {
			return err
		}
		r.SourceRepo = repo
	}

	return nil
}

func (r *RepoReparent) LoadTargetParent(ctx context.Context) error {
	if r.TargetParent == nil {
		repo, err := GetRepositoryByID(ctx, r.TargetParentID)
		if err != nil {
			return err
		}
		r.TargetParent = repo
	}

	return nil
}

func (r *RepoReparent) LoadDoer(ctx context.Context) error {
	if r.Doer == nil {
		u, err := user_model.GetUserByID(ctx, r.DoerID)
		if err != nil {
			return err
		}
		r.Doer = u
	}

	return nil
}

// LoadAttributes fetches all related attributes from the database
func (r *RepoReparent) LoadAttributes(ctx context.Context) error {
	if err := r.LoadSourceRepo(ctx); err != nil {
		return err
	}
	if err := r.LoadTargetParent(ctx); err != nil {
		return err
	}
	if err := r.LoadDoer(ctx); err != nil {
		return err
	}
	return nil
}

// CanUserAcceptOrRejectReparent checks if the user has the rights to accept/decline a repo reparenting.
// The user must be the owner of the target repository (or admin of the org).
func (r *RepoReparent) CanUserAcceptOrRejectReparent(ctx context.Context, u *user_model.User) bool {
	if err := r.LoadTargetParent(ctx); err != nil {
		log.Error("LoadTargetParent: %v", err)
		return false
	}

	if err := r.TargetParent.LoadOwner(ctx); err != nil {
		log.Error("LoadOwner: %v", err)
		return false
	}

	if r.TargetParent.OwnerID == u.ID {
		return true
	}

	if u.IsAdmin {
		return true
	}

	return false
}

// GetPendingReparentByRepo fetches the pending reparenting request for a repository
func GetPendingReparentByRepo(ctx context.Context, repoID int64) (*RepoReparent, error) {
	reparent := new(RepoReparent)
	has, err := db.GetEngine(ctx).Where("source_repo_id = ?", repoID).Get(reparent)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrNoPendingRepoReparent{RepoID: repoID}
	}
	return reparent, nil
}

// IsReparentExist checks if a reparenting request already exists for the repository
func IsReparentExist(ctx context.Context, repoID int64) (bool, error) {
	return db.GetEngine(ctx).Where("source_repo_id = ?", repoID).Exist(new(RepoReparent))
}

// DeleteReparent deletes a reparenting request and resets repository status if needed
func DeleteReparent(ctx context.Context, repoID int64) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		repo, err := GetRepositoryByID(ctx, repoID)
		if err != nil {
			if !IsErrRepoNotExist(err) {
				return err
			}
		} else if repo.Status == RepositoryPendingReparent {
			repo.Status = RepositoryReady
			if err := UpdateRepositoryColsNoAutoTime(ctx, repo, "status"); err != nil {
				return err
			}
		}

		_, err = db.GetEngine(ctx).Where("source_repo_id = ?", repoID).Delete(&RepoReparent{})
		return err
	})
}

// CreatePendingReparent marks the repository reparenting as "pending"
func CreatePendingReparent(ctx context.Context, doer *user_model.User, sourceID, targetID int64) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		source, err := GetRepositoryByID(ctx, sourceID)
		if err != nil {
			return err
		}

		// Make sure repo is ready for operations
		if source.Status == RepositoryBeingMigrated {
			return errors.New("repo is not ready, currently migrating")
		}
		if source.Status == RepositoryPendingTransfer {
			return ErrRepoTransferInProgress{Uname: source.OwnerName, Name: source.Name}
		}
		if source.Status == RepositoryPendingReparent {
			return ErrRepoReparentInProgress{Uname: source.OwnerName, Name: source.Name}
		}

		exist, err := IsReparentExist(ctx, source.ID)
		if err != nil {
			return err
		}
		if exist {
			return ErrRepoReparentInProgress{Uname: source.OwnerName, Name: source.Name}
		}

		source.Status = RepositoryPendingReparent
		if err := UpdateRepositoryColsNoAutoTime(ctx, source, "status"); err != nil {
			return err
		}

		reparent := &RepoReparent{
			DoerID:         doer.ID,
			SourceRepoID:   source.ID,
			TargetParentID: targetID,
			CreatedUnix:    timeutil.TimeStampNow(),
			UpdatedUnix:    timeutil.TimeStampNow(),
		}

		return db.Insert(ctx, reparent)
	})
}
