/*
Copyright 2019 The Kubernetes Authors.

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

	"github.com/google/go-github/v29/github"
	"k8s.io/release/pkg/notes/internal"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

// Client is used to talk to GitHub and query the repo/commit information
// needed to assemble the release notes
//counterfeiter:generate . Client
type Client interface {
	GetCommit(ctx context.Context, owner string, repo string, sha string) (*github.Commit, *github.Response, error)
	ListCommits(ctx context.Context, owner, repo string, opt *github.CommitsListOptions) ([]*github.RepositoryCommit, *github.Response, error)
	ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string, opt *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error)
	GetPullRequest(ctx context.Context, owner string, repo string, number int) (*github.PullRequest, *github.Response, error)

	// TODO: get rid of that method, currently only used in some test case
	GetRepoCommit(ctx context.Context, owner, repo, sha string) (*github.RepositoryCommit, *github.Response, error)
}

func New(ghc *github.Client) Client {
	return &githubNotesClient{ghc}
}

type githubNotesClient struct {
	*github.Client
}

var _ Client = &githubNotesClient{}

func (c *githubNotesClient) GetCommit(ctx context.Context, owner, repo, sha string) (*github.Commit, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		commit, resp, err := c.Git.GetCommit(ctx, owner, repo, sha)
		if !shouldRetry(err) {
			return commit, resp, err
		}
	}
}

func (c *githubNotesClient) ListCommits(ctx context.Context, owner, repo string, opt *github.CommitsListOptions) ([]*github.RepositoryCommit, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		commits, resp, err := c.Repositories.ListCommits(ctx, owner, repo, opt)
		if !shouldRetry(err) {
			return commits, resp, err
		}
	}
}

func (c *githubNotesClient) ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string, opt *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		prs, resp, err := c.PullRequests.ListPullRequestsWithCommit(ctx, owner, repo, sha, opt)
		if !shouldRetry(err) {
			return prs, resp, err
		}
	}
}

func (c *githubNotesClient) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		pr, resp, err := c.PullRequests.Get(ctx, owner, repo, number)
		if !shouldRetry(err) {
			return pr, resp, err
		}
	}
}

func (c *githubNotesClient) GetRepoCommit(ctx context.Context, owner, repo, sha string) (*github.RepositoryCommit, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		commit, resp, err := c.Repositories.GetCommit(ctx, owner, repo, sha)
		if !shouldRetry(err) {
			return commit, resp, err
		}
	}
}
