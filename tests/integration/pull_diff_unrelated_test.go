// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	auth_model "gitea.dev/models/auth"
	repo_model "gitea.dev/models/repo"
	unittest "gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/git/gitcmd"
	api "gitea.dev/modules/structs"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
)

func TestPullDiff_Unrelated(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		session := loginUser(t, "user1")
		
		// Create a repo
		token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteUser)
		repoName := "unrelated-diff"
		req := NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:          repoName,
			AutoInit:      true,
			DefaultBranch: "master",
		}).AddTokenAuth(token)
		session.MakeRequest(t, req, http.StatusCreated)

		user1 := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "user1"})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerID: user1.ID, Name: repoName})
		path := repo_model.RepoPath(user1.Name, repo.Name)

		// Create an orphan branch with unrelated history
		err := gitcmd.NewCommand("read-tree", "--empty").WithDir(path).Run(t.Context())
		assert.NoError(t, err)

		stdout, _, err := gitcmd.NewCommand("hash-object", "-w", "--stdin").
			WithDir(path).
			WithStdinBytes([]byte("Unrelated File Content")).
			RunStdString(t.Context())
		assert.NoError(t, err)
		sha := strings.TrimSpace(stdout)

		_, _, err = gitcmd.NewCommand("update-index", "--add", "--replace", "--cacheinfo").
			AddDynamicArguments("100644", sha, "unrelated.txt").
			WithDir(path).
			RunStdString(t.Context())
		assert.NoError(t, err)

		treeSha, _, err := gitcmd.NewCommand("write-tree").WithDir(path).RunStdString(t.Context())
		assert.NoError(t, err)
		treeSha = strings.TrimSpace(treeSha)

		commitTimeStr := time.Now().Format(time.RFC3339)
		doerSig := user1.NewGitSig()
		env := append(os.Environ(),
			"GIT_AUTHOR_NAME="+doerSig.Name,
			"GIT_AUTHOR_EMAIL="+doerSig.Email,
			"GIT_AUTHOR_DATE="+commitTimeStr,
			"GIT_COMMITTER_NAME="+doerSig.Name,
			"GIT_COMMITTER_EMAIL="+doerSig.Email,
			"GIT_COMMITTER_DATE="+commitTimeStr,
		)

		messageBytes := new(bytes.Buffer)
		_, _ = messageBytes.WriteString("Unrelated Branch Commit")
		_, _ = messageBytes.WriteString("\n")

		stdout, _, err = gitcmd.NewCommand("commit-tree").AddDynamicArguments(treeSha).
			WithEnv(env).
			WithDir(path).
			WithStdinBytes(messageBytes.Bytes()).
			RunStdString(t.Context())
		assert.NoError(t, err)
		commitSha := strings.TrimSpace(stdout)

		_, _, err = gitcmd.NewCommand("branch", "unrelated").
			AddDynamicArguments(commitSha).
			WithDir(path).
			RunStdString(t.Context())
		assert.NoError(t, err)

		// Create PR
		req = NewRequestWithJSON(t, http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/pulls", "user1", repoName), &api.CreatePullRequestOption{
			Head:  "unrelated",
			Base:  "master",
			Title: "Unrelated PR",
		}).AddTokenAuth(token)
		resp := session.MakeRequest(t, req, http.StatusCreated)
		var apiPull api.PullRequest
		DecodeJSON(t, resp, &apiPull)

		// Try to view diff
		req = NewRequest(t, "GET", fmt.Sprintf("/user1/%s/pulls/%d/files", repoName, apiPull.Index))
		resp = session.MakeRequest(t, req, http.StatusOK)
		
		doc := NewHTMLParser(t, resp.Body)
		// We expect to see 2 files: README.md (deleted) and unrelated.txt (added)
		// because we are diffing against the target branch (master) which has README.md
		assert.Equal(t, 2, doc.doc.Find(".file-content").Length())
		
		// Find unrelated.txt
		foundUnrelated := false
		doc.doc.Find(".file-content").Each(func(i int, s *goquery.Selection) {
			newFilename, _ := s.Attr("data-new-filename")
			if newFilename == "unrelated.txt" {
				foundUnrelated = true
			}
		})
		assert.True(t, foundUnrelated, "unrelated.txt should be found in diff")
		
		// Check the patch download as well
		req = NewRequest(t, "GET", fmt.Sprintf("/user1/%s/pulls/%d.patch", repoName, apiPull.Index))
		session.MakeRequest(t, req, http.StatusOK)
		
		// Check the diff download as well
		req = NewRequest(t, "GET", fmt.Sprintf("/user1/%s/pulls/%d.diff", repoName, apiPull.Index))
		session.MakeRequest(t, req, http.StatusOK)
	})
}
