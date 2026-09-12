// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v28

import (
	"context"

	"gitea.dev/modelmigration/base"
	"gitea.dev/modules/timeutil"
)

func AddRepoReparentTable(_ context.Context, x base.EngineMigration) error {
	type RepoReparent struct {
		ID             int64 `xorm:"pk autoincr"`
		DoerID         int64
		SourceRepoID   int64 `xorm:"UNIQUE(s) INDEX"`
		TargetParentID int64 `xorm:"INDEX"`

		CreatedUnix timeutil.TimeStamp `xorm:"INDEX NOT NULL created"`
		UpdatedUnix timeutil.TimeStamp `xorm:"INDEX NOT NULL updated"`
	}
	return x.Sync(new(RepoReparent))
}
