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
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v29/github"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	errorUtils "k8s.io/apimachinery/pkg/util/errors"
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

	GetReleaseByTag(
		context.Context, string, string, string,
	) (*github.RepositoryRelease, *github.Response, error)

	DownloadReleaseAsset(
		context.Context, string, string, int64,
	) (io.ReadCloser, string, error)

	ListTags(
		context.Context, string, string, *github.ListOptions,
	) ([]*github.RepositoryTag, *github.Response, error)

	ListBranches(
		context.Context, string, string, *github.BranchListOptions,
	) ([]*github.Branch, *github.Response, error)

	CreatePullRequest(
		context.Context, string, string, string, string, string, string,
	) (*github.PullRequest, error)

	GetRepository(
		context.Context, string, string,
	) (*github.Repository, *github.Response, error)
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

func (g *githubClient) GetReleaseByTag(
	ctx context.Context, owner, repo, tag string,
) (*github.RepositoryRelease, *github.Response, error) {
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		release, resp, err := g.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
		if !shouldRetry(err) {
			return release, resp, err
		}
	}
}

func (g *githubClient) DownloadReleaseAsset(
	ctx context.Context, owner, repo string, assetID int64,
) (io.ReadCloser, string, error) {
	// TODO: Should we be getting this http client from somewhere else?
	httpClient := http.DefaultClient
	for shouldRetry := internal.DefaultGithubErrChecker(); ; {
		assetBody, redirectURL, err := g.Repositories.DownloadReleaseAsset(ctx, owner, repo, assetID, httpClient)
		if !shouldRetry(err) {
			return assetBody, redirectURL, err
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

func (g *githubClient) ListBranches(
	ctx context.Context, owner, repo string, opt *github.BranchListOptions,
) ([]*github.Branch, *github.Response, error) {
	branches, response, err := g.Repositories.ListBranches(ctx, owner, repo, opt)
	if err != nil {
		return nil, nil, errors.Wrap(err, "fetching brnaches from repo")
	}

	return branches, response, nil
}

func (g *githubClient) CreatePullRequest(
	ctx context.Context, owner, repo, baseBranchName, headBranchName, title, body string,
) (*github.PullRequest, error) {
	newPullRequest := &github.NewPullRequest{
		Title:               &title,
		Head:                &headBranchName,
		Base:                &baseBranchName,
		Body:                &body,
		MaintainerCanModify: github.Bool(true),
	}

	pr, _, err := g.PullRequests.Create(ctx, owner, repo, newPullRequest)
	if err != nil {
		return pr, errors.Wrap(err, "creating pull request")
	}

	logrus.Infof("Successfully created PR #%d", pr.GetNumber())
	return pr, nil
}

func (g *githubClient) GetRepository(
	ctx context.Context, owner, repo string,
) (*github.Repository, *github.Response, error) {
	pr, resp, err := g.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return pr, resp, errors.Wrap(err, "getting repository")
	}

	return pr, resp, nil
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
	// List tags for all pages
	allTags := []*github.RepositoryTag{}
	opts := &github.ListOptions{PerPage: 100}
	for {
		tags, resp, err := g.client.ListTags(
			context.Background(), git.DefaultGithubOrg, git.DefaultGithubRepo,
			opts,
		)
		if err != nil {
			return nil, errors.Wrap(err, "unable to retrieve GitHub tags")
		}
		allTags = append(allTags, tags...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	releases := make(TagsPerBranch)
	for _, t := range allTags {
		tag := t.GetName()

		// alpha and beta releases are only available on the master branch
		if strings.Contains(tag, "beta") || strings.Contains(tag, "alpha") {
			releases.addIfNotExisting(git.Master, tag)
			continue
		}

		// We skip non-semver tags because k/k contains tags like `v0.5` which
		// are not valid
		semverTag, err := util.TagStringToSemver(tag)
		if err != nil {
			logrus.Debugf("Skipping tag %s because it is not valid semver", tag)
			continue
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
// TODO: Create a more descriptive method name and update references
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

// GetReleaseTags returns a list of GitHub release tags for the provided
// `owner` and `repo`. If `includePrereleases` is `true`, then the resulting
// slice will also contain pre/drafted releases.
func (g *GitHub) GetReleaseTags(owner, repo string, includePrereleases bool) ([]string, error) {
	releases, err := g.Releases(owner, repo, includePrereleases)
	if err != nil {
		return nil, errors.Wrap(err, "getting releases")
	}

	releaseTags := []string{}
	for _, release := range releases {
		releaseTags = append(releaseTags, *release.TagName)
	}

	return releaseTags, nil
}

// DownloadReleaseAssets downloads a set of GitHub release assets to an
// `outputDir`. Assets to download are derived from the `releaseTags`.
func (g *GitHub) DownloadReleaseAssets(owner, repo string, releaseTags []string, outputDir string) error {
	var releases []*github.RepositoryRelease

	if len(releaseTags) > 0 {
		for _, tag := range releaseTags {
			release, _, err := g.client.GetReleaseByTag(context.Background(), owner, repo, tag)
			if err != nil {
				return errors.Wrap(err, "getting release tags")
			}

			releases = append(releases, release)
		}
	} else {
		return errors.New("no release tags were populated")
	}

	funcs := []func() error{}
	for i := range releases {
		release := releases[i]
		funcs = append(funcs, func() error {
			releaseTag := release.GetTagName()
			logrus.Infof("Download assets for %s/%s@%s", owner, repo, releaseTag)

			assets := release.Assets
			if len(assets) == 0 {
				logrus.Infof("Skipping download for %s/%s@%s as no release assets were found", owner, repo, releaseTag)
				return nil
			}

			releaseDir := filepath.Join(outputDir, owner, repo, releaseTag)
			if err := os.MkdirAll(releaseDir, os.FileMode(0o775)); err != nil {
				return errors.Wrap(err, "creating output directory for release assets")
			}

			logrus.Infof("Writing assets to %s", releaseDir)
			err := g.downloadAssetsParallel(assets, owner, repo, releaseDir)
			if err != nil {
				return err
			}

			return nil
		})
	}

	return errorUtils.AggregateGoroutines(funcs...)
}

func (g *GitHub) downloadAssetsParallel(assets []github.ReleaseAsset, owner, repo, releaseDir string) error {
	funcs := []func() error{}
	for i := range assets {
		asset := assets[i]
		funcs = append(funcs, func() error {
			if asset.GetID() == 0 {
				return errors.New("asset ID should never be zero")
			}

			logrus.Infof("GitHub asset ID: %v, download URL: %s", *asset.ID, *asset.BrowserDownloadURL)
			assetBody, _, err := g.client.DownloadReleaseAsset(context.Background(), owner, repo, asset.GetID())
			if err != nil {
				return errors.Wrap(err, "downloading release assets")
			}

			absFile := filepath.Join(releaseDir, asset.GetName())
			defer assetBody.Close()
			assetFile, err := os.Create(absFile)
			if err != nil {
				return errors.Wrap(err, "creating release asset file")
			}

			defer assetFile.Close()
			if _, err := io.Copy(assetFile, assetBody); err != nil {
				return errors.Wrap(err, "copying release asset to file")
			}

			return nil
		})
	}

	return errorUtils.AggregateGoroutines(funcs...)
}

// CreatePullRequest Creates a new pull request in owner/repo:baseBranch to merge changes from headBranchName
// which is a string containing a branch in the same repository or a user:branch pair
func (g *GitHub) CreatePullRequest(
	owner, repo, baseBranchName, headBranchName, title, body string,
) (*github.PullRequest, error) {
	// Use the client to create a new PR
	pr, err := g.Client().CreatePullRequest(context.Background(), owner, repo, baseBranchName, headBranchName, title, body)
	if err != nil {
		return pr, err
	}

	return pr, nil
}

// GetRepository gets a repository using the current client
func (g *GitHub) GetRepository(
	owner, repo string,
) (*github.Repository, error) {
	repository, _, err := g.Client().GetRepository(context.Background(), owner, repo)
	if err != nil {
		return repository, err
	}

	return repository, nil
}

// ListBranches gets a repository using the current client
func (g *GitHub) ListBranches(
	owner, repo string,
) ([]*github.Branch, error) {
	branches, _, err := g.Client().ListBranches(context.Background(), owner, repo, &github.BranchListOptions{})
	if err != nil {
		return branches, errors.Wrap(err, "getting branches from client")
	}

	return branches, nil
}

// RepoIsForkOf Function that checks if a repository is a fork of another
func (g *GitHub) RepoIsForkOf(
	forkOwner, forkRepo, parentOwner, parentRepo string,
) (bool, error) {
	repository, _, err := g.Client().GetRepository(context.Background(), forkOwner, forkRepo)
	if err != nil {
		return false, errors.Wrap(err, "checking if repository is a fork")
	}

	// First, repo has to be an actual fork
	if !repository.GetFork() {
		logrus.Infof("Repository %s/%s is not a fork", forkOwner, forkRepo)
		return false, nil
	}

	// Check if the parent repo matches the owner/repo string
	if repository.GetParent().GetFullName() == fmt.Sprintf("%s/%s", parentOwner, parentRepo) {
		logrus.Debugf("%s/%s is a fork of %s/%s", forkOwner, forkRepo, parentOwner, parentRepo)
		return true, nil
	}

	logrus.Infof("%s/%s is not a fork of %s/%s", forkOwner, forkRepo, parentOwner, parentRepo)
	return false, nil
}

// BranchExists checks if a branch exists in a given repo
func (g *GitHub) BranchExists(
	owner, repo, branchname string,
) (isBranch bool, err error) {
	branches, err := g.ListBranches(owner, repo)
	if err != nil {
		return false, errors.Wrap(err, "while listing repository branches")
	}

	for _, branch := range branches {
		if branch.GetName() == branchname {
			logrus.Debugf("Branch %s already exists in %s/%s", branchname, owner, repo)
			return true, nil
		}
	}

	logrus.Debugf("Repository %s/%s does not have a branch named %s", owner, repo, branchname)
	return false, nil
}
