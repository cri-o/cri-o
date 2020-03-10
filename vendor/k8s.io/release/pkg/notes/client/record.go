/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/go-github/v29/github"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type gitHubAPI string

const (
	gitHubAPIGetCommit                  gitHubAPI = "GetCommit"
	gitHubAPIListCommits                gitHubAPI = "ListCommits"
	gitHubAPIListPullRequestsWithCommit gitHubAPI = "ListPullRequestsWithCommit"
	gitHubAPIGetPullRequest             gitHubAPI = "GetPullRequest"
	gitHubAPIGetRepoCommit              gitHubAPI = "GetRepoCommit"
)

type apiRecord struct {
	Result   interface{}
	LastPage int
}

func (a *apiRecord) response() *github.Response {
	return &github.Response{LastPage: a.LastPage}
}

func NewRecorder(c Client, recordDir string) Client {
	return &githubNotesRecordClient{
		client:      c,
		recordDir:   recordDir,
		recordState: map[gitHubAPI]int{},
	}
}

type githubNotesRecordClient struct {
	client      Client
	recordDir   string
	recordMutex sync.Mutex
	recordState map[gitHubAPI]int
}

func (c *githubNotesRecordClient) GetCommit(ctx context.Context, owner, repo, sha string) (*github.Commit, *github.Response, error) {
	commit, resp, err := c.client.GetCommit(ctx, owner, repo, sha)
	if err != nil {
		return nil, nil, err
	}
	if err := c.recordAPICall(gitHubAPIGetCommit, commit, resp); err != nil {
		return nil, nil, err
	}
	return commit, resp, nil
}

func (c *githubNotesRecordClient) ListCommits(ctx context.Context, owner, repo string, opt *github.CommitsListOptions) ([]*github.RepositoryCommit, *github.Response, error) {
	commits, resp, err := c.client.ListCommits(ctx, owner, repo, opt)
	if err != nil {
		return nil, nil, err
	}
	if err := c.recordAPICall(gitHubAPIListCommits, commits, resp); err != nil {
		return nil, nil, err
	}
	return commits, resp, nil
}

func (c *githubNotesRecordClient) ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string, opt *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error) {
	prs, resp, err := c.client.ListPullRequestsWithCommit(ctx, owner, repo, sha, opt)
	if err != nil {
		return nil, nil, err
	}
	if err := c.recordAPICall(gitHubAPIListPullRequestsWithCommit, prs, resp); err != nil {
		return nil, nil, err
	}
	return prs, resp, nil
}

func (c *githubNotesRecordClient) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, *github.Response, error) {
	pr, resp, err := c.client.GetPullRequest(ctx, owner, repo, number)
	if err != nil {
		return nil, nil, err
	}
	if err := c.recordAPICall(gitHubAPIGetPullRequest, pr, resp); err != nil {
		return nil, nil, err
	}
	return pr, resp, nil
}

func (c *githubNotesRecordClient) GetRepoCommit(ctx context.Context, owner, repo, sha string) (*github.RepositoryCommit, *github.Response, error) {
	commit, resp, err := c.client.GetRepoCommit(ctx, owner, repo, sha)
	if err != nil {
		return nil, nil, err
	}
	if err := c.recordAPICall(gitHubAPIGetRepoCommit, commit, resp); err != nil {
		return nil, nil, err
	}
	return commit, resp, nil
}

// recordAPICall records a single GitHub API call into a JSON file by ensuring
// naming conventions
func (c *githubNotesRecordClient) recordAPICall(
	api gitHubAPI, result interface{}, response *github.Response,
) error {
	if result == nil {
		return errors.New("no result to record")
	}
	logrus.Debugf("recording API call %s to %s", api, c.recordDir)

	c.recordMutex.Lock()
	defer c.recordMutex.Unlock()

	i := 0
	if j, ok := c.recordState[api]; ok {
		i = j + 1
	}
	c.recordState[api] = i

	fileName := fmt.Sprintf("%s-%d.json", api, i)

	lastPage := 0
	if response != nil {
		lastPage = response.LastPage
	}

	file, err := json.MarshalIndent(&apiRecord{result, lastPage}, "", " ")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(
		filepath.Join(c.recordDir, fileName), file, os.FileMode(0644),
	); err != nil {
		return err
	}

	return nil
}
