// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT
//go:build gogit

package git

import (
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/hash"
)

// type Sha1Hash struct {
// 	val plumbing.Hash
// }

func ParseGogitHash(h plumbing.Hash) Hash {
	switch hash.Size {
	case 20:
		return HashTypeFromID(Sha1).MustID(h[:])
	case 32:
		return HashTypeFromID(Sha256).MustID(h[:])
	}

	return nil
}

func ParseGogitHashArray(hashes []plumbing.Hash) []Hash {
	ret := make([]Hash, len(hashes))
	for i, h := range hashes {
		ret[i] = ParseGogitHash(h)
	}

	return ret
}

/*
func defaultHashType() HashTypeInterface {
	switch hash.Size {
	case 20:
		return NewSha1()
	case 32:
		return NewSha256()
	}

	return nil
}
*/
