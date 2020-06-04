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

package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sync"

	"github.com/google/go-github/v29/github"
)

func NewReplayer(replayDir string) Client {
	return &githubNotesReplayClient{
		replayDir:   replayDir,
		replayState: map[gitHubAPI]int{},
	}
}

type githubNotesReplayClient struct {
	replayDir   string
	replayMutex sync.Mutex
	replayState map[gitHubAPI]int
}

func (c *githubNotesReplayClient) GetCommit(ctx context.Context, owner, repo, sha string) (*github.Commit, *github.Response, error) {
	data, err := c.readRecordedData(gitHubAPIGetCommit)
	if err != nil {
		return nil, nil, err
	}
	result := &github.Commit{}
	record := apiRecord{Result: result}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, nil, err
	}
	return result, record.response(), nil
}

func (c *githubNotesReplayClient) ListCommits(ctx context.Context, owner, repo string, opt *github.CommitsListOptions) ([]*github.RepositoryCommit, *github.Response, error) {
	data, err := c.readRecordedData(gitHubAPIListCommits)
	if err != nil {
		return nil, nil, err
	}
	result := []*github.RepositoryCommit{}
	record := apiRecord{Result: &result}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, nil, err
	}
	return result, record.response(), nil
}

func (c *githubNotesReplayClient) ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string, opt *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error) {
	data, err := c.readRecordedData(gitHubAPIListPullRequestsWithCommit)
	if err != nil {
		return nil, nil, err
	}
	result := []*github.PullRequest{}
	record := apiRecord{Result: &result}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, nil, err
	}
	return result, record.response(), nil
}

func (c *githubNotesReplayClient) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, *github.Response, error) {
	data, err := c.readRecordedData(gitHubAPIGetPullRequest)
	if err != nil {
		return nil, nil, err
	}
	result := &github.PullRequest{}
	record := apiRecord{Result: result}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, nil, err
	}
	return result, record.response(), nil
}

func (c *githubNotesReplayClient) GetRepoCommit(ctx context.Context, owner, repo, sha string) (*github.RepositoryCommit, *github.Response, error) {
	data, err := c.readRecordedData(gitHubAPIGetRepoCommit)
	if err != nil {
		return nil, nil, err
	}
	result := &github.RepositoryCommit{}
	record := apiRecord{Result: result}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, nil, err
	}
	return result, record.response(), nil
}

func (c *githubNotesReplayClient) ListReleases(
	ctx context.Context, owner, repo string, opt *github.ListOptions,
) ([]*github.RepositoryRelease, *github.Response, error) {
	data, err := c.readRecordedData(gitHubAPIListReleases)
	if err != nil {
		return nil, nil, err
	}
	result := []*github.RepositoryRelease{}
	record := apiRecord{Result: result}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, nil, err
	}
	return result, record.response(), nil
}

func (c *githubNotesReplayClient) ListTags(
	ctx context.Context, owner, repo string, opt *github.ListOptions,
) ([]*github.RepositoryTag, *github.Response, error) {
	data, err := c.readRecordedData(gitHubAPIListTags)
	if err != nil {
		return nil, nil, err
	}
	result := []*github.RepositoryTag{}
	record := apiRecord{Result: result}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, nil, err
	}
	return result, record.response(), nil
}

func (c *githubNotesReplayClient) CreatePullRequest(
	ctx context.Context, owner, repo, baseBranchName, headBranchName, title, body string,
) (*github.PullRequest, error) {
	return &github.PullRequest{}, nil
}

func (c *githubNotesReplayClient) GetRepository(
	ctx context.Context, owner, repo string,
) (*github.Repository, *github.Response, error) {
	data, err := c.readRecordedData(gitHubAPIGetRepository)
	if err != nil {
		return nil, nil, err
	}
	repository := &github.Repository{}
	record := apiRecord{Result: repository}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, nil, err
	}
	return repository, record.response(), nil
}

func (c *githubNotesReplayClient) readRecordedData(api gitHubAPI) ([]byte, error) {
	c.replayMutex.Lock()
	defer c.replayMutex.Unlock()

	i := 0
	if j, ok := c.replayState[api]; ok {
		i = j
	}

	path := filepath.Join(c.replayDir, fmt.Sprintf("%s-%d.json", api, i))
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	c.replayState[api]++
	return file, nil
}
