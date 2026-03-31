// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"errors"
	"net/http"

	"code.gitea.io/gitea/models/perm"
	access_model "code.gitea.io/gitea/models/perm/access"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/log"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/modules/web"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/convert"
	repo_service "code.gitea.io/gitea/services/repository"
)

// Reparent initiates a reparenting request
func Reparent(ctx *context.APIContext) {
	// swagger:operation POST /repos/{owner}/{repo}/reparent repository repoReparent
	// ---
	// summary: Reparent a repository
	// produces:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo to be demoted to fork
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo to be demoted to fork
	//   type: string
	//   required: true
	// - name: body
	//   in: body
	//   description: "Reparent Options"
	//   required: true
	//   schema:
	//     "$ref": "#/definitions/ReparentRepoOption"
	// responses:
	//   "201":
	//     "$ref": "#/responses/Repository"
	//   "202":
	//     "$ref": "#/responses/Repository"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"
	//   "422":
	//     "$ref": "#/responses/validationError"

	opts := web.GetForm(ctx).(*api.ReparentRepoOption)

	parentName := opts.NewName
	if parentName == "" {
		parentName = ctx.Repo.Repository.Name
	}

	targetParent, err := repo_model.GetRepositoryByOwnerAndName(ctx, opts.NewParent, parentName)
	if err != nil {
		if repo_model.IsErrRepoNotExist(err) {
			ctx.APIError(http.StatusNotFound, "The new parent repository does not exist")
			return
		}
		ctx.APIErrorInternal(err)
		return
	}

	if err := repo_service.StartRepositoryReparent(ctx, ctx.Doer, ctx.Repo.Repository, targetParent); err != nil {
		switch {
		case repo_model.IsErrRepoReparentInProgress(err):
			ctx.APIError(http.StatusConflict, err)
		case repo_model.IsErrRepoAlreadyExist(err):
			ctx.APIError(http.StatusConflict, err)
		default:
			ctx.APIErrorInternal(err)
		}
		return
	}

	if ctx.Repo.Repository.Status == repo_model.RepositoryPendingReparent {
		log.Trace("Repository reparenting initiated: %s", ctx.Repo.Repository.FullName())
		ctx.JSON(http.StatusCreated, convert.ToRepo(ctx, ctx.Repo.Repository, access_model.Permission{AccessMode: perm.AccessModeAdmin}))
		return
	}

	log.Trace("Repository reparented: %s", ctx.Repo.Repository.FullName())
	ctx.JSON(http.StatusAccepted, convert.ToRepo(ctx, ctx.Repo.Repository, access_model.Permission{AccessMode: perm.AccessModeAdmin}))
}

// AcceptReparent accepts a reparenting request
func AcceptReparent(ctx *context.APIContext) {
	// swagger:operation POST /repos/{owner}/{repo}/reparent/accept repository acceptRepoReparent
	// ---
	// summary: Accept a repo reparenting
	// produces:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo
	//   type: string
	//   required: true
	// responses:
	//   "202":
	//     "$ref": "#/responses/Repository"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	err := repo_service.AcceptReparent(ctx, ctx.Doer, ctx.Repo.Repository)
	if err != nil {
		switch {
		case repo_model.IsErrNoPendingReparent(err):
			ctx.APIError(http.StatusNotFound, err)
		case errors.Is(err, util.ErrPermissionDenied):
			ctx.APIError(http.StatusForbidden, err)
		default:
			ctx.APIErrorInternal(err)
		}
		return
	}

	ctx.JSON(http.StatusAccepted, convert.ToRepo(ctx, ctx.Repo.Repository, ctx.Repo.Permission))
}

// RejectReparent rejects a reparenting request
func RejectReparent(ctx *context.APIContext) {
	// swagger:operation POST /repos/{owner}/{repo}/reparent/reject repository rejectRepoReparent
	// ---
	// summary: Reject a repo reparenting
	// produces:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo
	//   type: string
	//   required: true
	// responses:
	//   "200":
	//     "$ref": "#/responses/Repository"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	err := repo_service.RejectReparent(ctx, ctx.Doer, ctx.Repo.Repository)
	if err != nil {
		switch {
		case repo_model.IsErrNoPendingReparent(err):
			ctx.APIError(http.StatusNotFound, err)
		case errors.Is(err, util.ErrPermissionDenied):
			ctx.APIError(http.StatusForbidden, err)
		default:
			ctx.APIErrorInternal(err)
		}
		return
	}

	ctx.JSON(http.StatusOK, convert.ToRepo(ctx, ctx.Repo.Repository, ctx.Repo.Permission))
}
