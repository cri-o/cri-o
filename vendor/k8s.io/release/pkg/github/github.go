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
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/google/go-github/v29/github"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/github/internal"
	"k8s.io/release/pkg/util"
)

const (
	// TokenEnvKey is the default GitHub token environemt variable key
	TokenEnvKey = "GITHUB_TOKEN"
)

// GitHub is a wrapper around GitHub related functionality
type GitHub struct {
	client Client
}

type githubClient struct {
	*github.Client
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate . Client
type Client interface {
	GetCommit(
		context.Context, string, string, string,
	) (*github.Commit, *github.Response, error)

	GetPullRequest(
		context.Context, string, string, int,
	) (*github.PullRequest, *github.Response, error)

	GetRepoCommit(
		context.Context, string, string, string,
	) (*github.RepositoryCommit, *github.Response, error)

	ListCommits(
		context.Context, string, string, *github.CommitsListOptions,
	) ([]*github.RepositoryCommit, *github.Response, error)

	ListPullRequestsWithCommit(
		context.Context, string, string, string, *github.PullRequestListOptions,
	) ([]*github.PullRequest, *github.Response, error)

	ListReleases(
		context.Context, string, string, *github.ListOptions,
	) ([]*github.RepositoryRelease, *github.Response, error)

	ListTags(
		context.Context, string, string, *github.ListOptions,
	) ([]*github.RepositoryTag, *github.Response, error)
}

// New creates a new default GitHub client. Tokens set via the $GITHUB_TOKEN
// environment variable will result in an authenticated client.
// If the $GITHUB_TOKEN is not set, then the client will do unauthenticated
// GitHub requests.
func New() *GitHub {
	ctx := context.Background()
	token := util.EnvDefault(TokenEnvKey, "")
	client := http.DefaultClient
	state := "unauthenticated"
	if token != "" {
		state = strings.TrimPrefix(state, "un")
		client = oauth2.NewClient(ctx, oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		))
	}
	logrus.Debugf("Using %s GitHub client", state)
	return &GitHub{&githubClient{github.NewClient(client)}}
}

// NewWithToken can be used to specify a GITHUB_TOKEN before retrieving the
// client to enforce authenticated GitHub requests
func NewWithToken(token string) (*GitHub, error) {
	if err := os.Setenv(TokenEnvKey, token); err != nil {
		return nil, errors.Wrapf(err, "unable to export %s", TokenEnvKey)
	}
	return New(), nil
}

func (g *githubClient) GetCommit(
	ctx context.Context, owner, repo, sha string,
) (*github.Commit, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		commit, resp, err := g.Git.GetCommit(ctx, owner, repo, sha)
		if !shouldRetry(err) {
			return commit, resp, err
		}
	}
}

func (g *githubClient) GetPullRequest(
	ctx context.Context, owner, repo string, number int,
) (*github.PullRequest, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		pr, resp, err := g.PullRequests.Get(ctx, owner, repo, number)
		if !shouldRetry(err) {
			return pr, resp, err
		}
	}
}

func (g *githubClient) GetRepoCommit(
	ctx context.Context, owner, repo, sha string,
) (*github.RepositoryCommit, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		commit, resp, err := g.Repositories.GetCommit(ctx, owner, repo, sha)
		if !shouldRetry(err) {
			return commit, resp, err
		}
	}
}

func (g *githubClient) ListCommits(
	ctx context.Context, owner, repo string, opt *github.CommitsListOptions,
) ([]*github.RepositoryCommit, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		commits, resp, err := g.Repositories.ListCommits(ctx, owner, repo, opt)
		if !shouldRetry(err) {
			return commits, resp, err
		}
	}
}

func (g *githubClient) ListPullRequestsWithCommit(
	ctx context.Context, owner, repo, sha string,
	opt *github.PullRequestListOptions,
) ([]*github.PullRequest, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		prs, resp, err := g.PullRequests.ListPullRequestsWithCommit(
			ctx, owner, repo, sha, opt,
		)
		if !shouldRetry(err) {
			return prs, resp, err
		}
	}
}

func (g *githubClient) ListReleases(
	ctx context.Context, owner, repo string, opt *github.ListOptions,
) ([]*github.RepositoryRelease, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		releases, resp, err := g.Repositories.ListReleases(
			ctx, owner, repo, opt,
		)
		if !shouldRetry(err) {
			return releases, resp, err
		}
	}
}

func (g *githubClient) ListTags(
	ctx context.Context, owner, repo string, opt *github.ListOptions,
) ([]*github.RepositoryTag, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		tags, resp, err := g.Repositories.ListTags(ctx, owner, repo, opt)
		if !shouldRetry(err) {
			return tags, resp, err
		}
	}
}

// SetClient can be used to manually set the internal GitHub client
func (g *GitHub) SetClient(client Client) {
	g.client = client
}

// Client can be used to retrieve the Client type
func (g *GitHub) Client() Client {
	return g.client
}

// TagsPerBranch is an abstraction over a simple branch to latest tag association
type TagsPerBranch map[string]string

// LatestGitHubTagsPerBranch returns the latest GitHub available tag for each
// branch. The logic how releases are associates with branches is motivated by
// the changelog generation and bound to the default Kubernetes release
// strategy, which is also the reason why we do not provide a repo and org
// parameter here.
//
// Releases are associated in the following way:
// - x.y.0-alpha.z releases are only associated with the master branch
// - x.y.0-beta.z releases are only associated with their release-x.y branch
// - x.y.0 final releases are associated with the master and the release-x.y branch
func (g *GitHub) LatestGitHubTagsPerBranch() (TagsPerBranch, error) {
	allTags, _, err := g.client.ListTags(
		context.Background(), git.DefaultGithubOrg, git.DefaultGithubRepo, nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "unable to retrieve GitHub tags")
	}

	releases := make(TagsPerBranch)
	for _, t := range allTags {
		tag := t.GetName()

		// Alpha releases are only available on the master branch
		if strings.Contains(tag, "alpha") {
			releases.addIfNotExisting(git.Master, tag)
			continue
		}

		// This should always succeed
		semverTag, err := util.TagStringToSemver(tag)
		if err != nil {
			return nil, errors.Wrapf(err, "tag %s is not a vaild semver", tag)
		}

		// Latest vx.x.0 release are on both master and release branch
		if len(semverTag.Pre) == 0 {
			releases.addIfNotExisting(git.Master, tag)
		}

		branch := fmt.Sprintf("release-%d.%d", semverTag.Major, semverTag.Minor)
		releases.addIfNotExisting(branch, tag)
	}

	return releases, nil
}

// addIfNotExisting adds a new `tag` for the `branch` if not already existing
// in the map `TagsForBranch`
func (t TagsPerBranch) addIfNotExisting(branch, tag string) {
	if _, ok := t[branch]; !ok {
		t[branch] = tag
	}
}

// Releases returns a list of GitHub releases for the provided `owner` and
// `repo`. If `includePrereleases` is `true`, then the resulting slice will
// also contain pre/drafted releases.
func (g *GitHub) Releases(owner, repo string, includePrereleases bool) ([]*github.RepositoryRelease, error) {
	allReleases, _, err := g.client.ListReleases(
		context.Background(), owner, repo, nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "unable to retrieve GitHub releases")
	}

	releases := []*github.RepositoryRelease{}
	for _, release := range allReleases {
		if release.GetPrerelease() {
			if includePrereleases {
				releases = append(releases, release)
			}
		} else {
			releases = append(releases, release)
		}
	}

	return releases, nil
}
