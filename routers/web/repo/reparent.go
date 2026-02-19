// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"gitea.dev/services/context"
	repo_service "gitea.dev/services/repository"
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
