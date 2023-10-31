// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"bytes"
	"io"
	"strings"
)

// ObjectType git object type
type ObjectType string

const (
	// ObjectCommit commit object type
	ObjectCommit ObjectType = "commit"
	// ObjectTree tree object type
	ObjectTree ObjectType = "tree"
	// ObjectBlob blob object type
	ObjectBlob ObjectType = "blob"
	// ObjectTag tag object type
	ObjectTag ObjectType = "tag"
	// ObjectBranch branch object type
	ObjectBranch ObjectType = "branch"
)

// Bytes returns the byte array for the Object Type
func (o ObjectType) Bytes() []byte {
	return []byte(o)
}

type EmptyReader struct{}

func (EmptyReader) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func (repo *Repository) HashType() (HashType, error) {
	if repo != nil && repo.Hash != nil {
		return repo.Hash, nil
	}

	str, err := repo.hashObject(EmptyReader{}, false)
	if err != nil {
		return nil, err
	}
	hash, err := HashFromString(str)
	if err != nil {
		return nil, err
	}

	return hash.Type(), nil
}

// HashObject takes a reader and returns hash for that reader
func (repo *Repository) HashObject(reader io.Reader) (Hash, error) {
	idStr, err := repo.hashObject(reader, true)
	if err != nil {
		return nil, err
	}
	return NewIDFromString(repo.Hash, idStr)
}

func (repo *Repository) hashObject(reader io.Reader, save bool) (string, error) {
	var cmd *Command
	if save {
		cmd = NewCommand(repo.Ctx, "hash-object", "-w", "--stdin")
	} else {
		cmd = NewCommand(repo.Ctx, "hash-object", "--stdin")
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := cmd.Run(&RunOpts{
		Dir:    repo.Path,
		Stdin:  reader,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// GetRefType gets the type of the ref based on the string
func (repo *Repository) GetRefType(ref string) ObjectType {
	if repo.IsTagExist(ref) {
		return ObjectTag
	} else if repo.IsBranchExist(ref) {
		return ObjectBranch
	} else if repo.IsCommitExist(ref) {
		return ObjectCommit
	} else if _, err := repo.GetBlob(ref); err == nil {
		return ObjectBlob
	}
	return ObjectType("invalid")
}
