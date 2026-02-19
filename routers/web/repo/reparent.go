// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"code.gitea.io/gitea/services/context"
	repo_service "code.gitea.io/gitea/services/repository"
)

func acceptReparent(ctx *context.Context) {
	err := repo_service.AcceptReparent(ctx, ctx.Doer, ctx.Repo.Repository)
	if err == nil {
		ctx.Flash.Success(ctx.Tr("repo.reparent.success"))
		ctx.Redirect(ctx.Repo.Repository.Link())
		return
	}
	ctx.ServerError("AcceptReparent", err)
}

func rejectReparent(ctx *context.Context) {
	err := repo_service.RejectReparent(ctx, ctx.Doer, ctx.Repo.Repository)
	if err == nil {
		ctx.Flash.Success(ctx.Tr("repo.reparent.rejected"))
		ctx.Redirect(ctx.Repo.Repository.Link())
		return
	}
	ctx.ServerError("RejectReparent", err)
}

func ActionReparent(ctx *context.Context) {
	switch ctx.PathParam("action") {
	case "accept_reparent":
		acceptReparent(ctx)
	case "reject_reparent":
		rejectReparent(ctx)
	}
}
