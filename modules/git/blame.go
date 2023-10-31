// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/util"
)

// BlamePart represents block of blame - continuous lines with one sha
type BlamePart struct {
	Sha   string
	Lines []string
}

// BlameReader returns part of file blame one by one
type BlameReader struct {
	output         io.WriteCloser
	reader         io.ReadCloser
	bufferedReader *bufio.Reader
	done           chan error
	lastSha        *string
	ignoreRevsFile *string
	hash           HashType
}

func (r *BlameReader) UsesIgnoreRevs() bool {
	return r.ignoreRevsFile != nil
}

// NextPart returns next part of blame (sequential code lines with the same commit)
func (r *BlameReader) NextPart() (*BlamePart, error) {
	var blamePart *BlamePart

	if r.lastSha != nil {
		blamePart = &BlamePart{*r.lastSha, make([]string, 0)}
	}

	var line []byte
	var isPrefix bool
	var err error

	for err != io.EOF {
		line, isPrefix, err = r.bufferedReader.ReadLine()
		if err != nil && err != io.EOF {
			return blamePart, err
		}

		if len(line) == 0 {
			// isPrefix will be false
			continue
		}

		var hash string
		hashLen := r.hash.FullLength()

		if len(line) > hashLen && line[hashLen] == ' ' && r.hash.IsValid(string(line[0:hashLen])) {
			hash = string(line[0:hashLen])
		}
		if len(hash) > 0 {
			if blamePart == nil {
				blamePart = &BlamePart{hash, make([]string, 0)}
			}

			if blamePart.Sha != hash {
				r.lastSha = &hash
				// need to munch to end of line...
				for isPrefix {
					_, isPrefix, err = r.bufferedReader.ReadLine()
					if err != nil && err != io.EOF {
						return blamePart, err
					}
				}
				return blamePart, nil
			}
		} else if line[0] == '\t' {
			code := line[1:]

			blamePart.Lines = append(blamePart.Lines, string(code))
		}

		// need to munch to end of line...
		for isPrefix {
			_, isPrefix, err = r.bufferedReader.ReadLine()
			if err != nil && err != io.EOF {
				return blamePart, err
			}
		}
	}

	r.lastSha = nil

	return blamePart, nil
}

// Close BlameReader - don't run NextPart after invoking that
func (r *BlameReader) Close() error {
	err := <-r.done
	r.bufferedReader = nil
	_ = r.reader.Close()
	_ = r.output.Close()
	if r.ignoreRevsFile != nil {
		_ = util.Remove(*r.ignoreRevsFile)
	}
	return err
}

// CreateBlameReader creates reader for given repository, commit and file
func CreateBlameReader(ctx context.Context, hash HashType, repoPath string, commit *Commit, file string, bypassBlameIgnore bool) (*BlameReader, error) {
	var ignoreRevsFile *string
	if CheckGitVersionAtLeast("2.23") == nil && !bypassBlameIgnore {
		ignoreRevsFile = tryCreateBlameIgnoreRevsFile(commit)
	}

	cmd := NewCommandContextNoGlobals(ctx, "blame", "--porcelain")
	if ignoreRevsFile != nil {
		// Possible improvement: use --ignore-revs-file /dev/stdin on unix
		// There is no equivalent on Windows. May be implemented if Gitea uses an external git backend.
		cmd.AddOptionValues("--ignore-revs-file", *ignoreRevsFile)
	}
	cmd.AddDynamicArguments(commit.ID.String()).
		AddDashesAndList(file).
		SetDescription(fmt.Sprintf("GetBlame [repo_path: %s]", repoPath))
	reader, stdout, err := os.Pipe()
	if err != nil {
		if ignoreRevsFile != nil {
			_ = util.Remove(*ignoreRevsFile)
		}
		return nil, err
	}

	done := make(chan error, 1)

	go func() {
		stderr := bytes.Buffer{}
		// TODO: it doesn't work for directories (the directories shouldn't be "blamed"), and the "err" should be returned by "Read" but not by "Close"
		err := cmd.Run(&RunOpts{
			UseContextTimeout: true,
			Dir:               repoPath,
			Stdout:            stdout,
			Stderr:            &stderr,
		})
		done <- err
		_ = stdout.Close()
		if err != nil {
			log.Error("Error running git blame (dir: %v): %v, stderr: %v", repoPath, err, stderr.String())
		}
	}()

	bufferedReader := bufio.NewReader(reader)

	return &BlameReader{
		output:         stdout,
		reader:         reader,
		bufferedReader: bufferedReader,
		done:           done,
		ignoreRevsFile: ignoreRevsFile,
		hash:           hash,
	}, nil
}

func tryCreateBlameIgnoreRevsFile(commit *Commit) *string {
	entry, err := commit.GetTreeEntryByPath(".git-blame-ignore-revs")
	if err != nil {
		return nil
	}

	r, err := entry.Blob().DataAsync()
	if err != nil {
		return nil
	}
	defer r.Close()

	f, err := os.CreateTemp("", "gitea_git-blame-ignore-revs")
	if err != nil {
		return nil
	}

	_, err = io.Copy(f, r)
	_ = f.Close()
	if err != nil {
		_ = util.Remove(f.Name())
		return nil
	}

	return util.ToPointer(f.Name())
}
