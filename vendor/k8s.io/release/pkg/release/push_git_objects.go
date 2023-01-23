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

package release

import (
	"errors"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-sdk/git"
	"sigs.k8s.io/release-utils/util"
)

// GitObjectPusher is an object that pushes things to a gitrepo
type GitObjectPusher struct {
	repo git.Repo
	opts *GitObjectPusherOptions
}

var dryRunLabel = map[bool]string{true: " --dry-run", false: ""}

// GitObjectPusherOptions struct to hold the pusher options
type GitObjectPusherOptions struct {
	// Flago simulate pushes, passes --dry-run to git
	DryRun bool

	// Number of times to retry pushes
	MaxRetries int

	// Path to the repository
	RepoPath string
}

// NewGitPusher returns a new git object pusher
func NewGitPusher(opts *GitObjectPusherOptions) (*GitObjectPusher, error) {
	repo, err := git.OpenRepo(opts.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("while opening repository: %w", err)
	}

	logrus.Infof("Checkout %s branch to push objects", git.DefaultBranch)
	if err := repo.Checkout(git.DefaultBranch); err != nil {
		return nil, fmt.Errorf("checking out %s branch: %w", git.DefaultBranch, err)
	}

	// Pass the dry-run flag to the repo
	if opts.DryRun {
		logrus.Debug("Setting dry run flag to repository, pushing will be simuluated")
		repo.SetDry()
	}

	// Set the number of retries for the git operations:
	repo.SetMaxRetries(opts.MaxRetries)

	return &GitObjectPusher{
		repo: *repo,
		opts: opts,
	}, nil
}

// PushBranches Convenience method to push a list of branches
func (gp *GitObjectPusher) PushBranches(branchList []string) error {
	for _, branchName := range branchList {
		if err := gp.PushBranch(branchName); err != nil {
			return fmt.Errorf("pushing %s branch: %w", branchName, err)
		}
	}
	logrus.Infof("Successfully pushed %d branches", len(branchList))
	return nil
}

// PushBranch pushes a branch to the repository
//
//	this function is idempotent.
func (gp *GitObjectPusher) PushBranch(branchName string) error {
	// Check if the branch name is correct
	if err := gp.checkBranchName(branchName); err != nil {
		return fmt.Errorf("checking branch name: %w", err)
	}

	// To be able to push a branch the ref has to exist in the local repo:
	branchExists, err := gp.repo.HasBranch(branchName)
	if err != nil {
		return fmt.Errorf("checking if branch already exists locally: %w", err)
	}
	if !branchExists {
		return fmt.Errorf("unable to push branch %s, it does not exist in the local repo", branchName)
	}

	// Checkout the branch before merging
	logrus.Infof("Checking out branch %s to merge upstream changes", branchName)
	if err := gp.repo.Checkout(branchName); err != nil {
		return fmt.Errorf("checking out branch %s: %w", git.Remotify(branchName), err)
	}

	if err := gp.mergeRemoteIfRequired(branchName); err != nil {
		return fmt.Errorf("merge remote if required: %w", err)
	}

	logrus.Infof("Pushing%s %s branch:", dryRunLabel[gp.opts.DryRun], branchName)
	if err := gp.repo.Push(branchName); err != nil {
		return fmt.Errorf("pushing branch %s: %w", branchName, err)
	}
	logrus.Infof("Branch %s pushed successfully", branchName)
	return nil
}

// PushTags convenience method to push a list of tags to the remote repo
func (gp *GitObjectPusher) PushTags(tagList []string) (err error) {
	for _, tag := range tagList {
		if err := gp.PushTag(tag); err != nil {
			return fmt.Errorf("while pushing %s tag: %w", tag, err)
		}
	}
	logrus.Infof("Pushed %d tags to the remote repo", len(tagList))
	return nil
}

// PushTag pushes a tag to the master repo
func (gp *GitObjectPusher) PushTag(newTag string) (err error) {
	// Verify that the tag is a valid tag
	if err := gp.checkTagName(newTag); err != nil {
		return fmt.Errorf("parsing version tag: %w", err)
	}

	// Check if tag already exists
	currentTags, err := gp.repo.Tags()
	if err != nil {
		return fmt.Errorf("checking if tag exists: %w", err)
	}

	// verify that the tag exists locally before trying to push
	tagExists := false
	for _, tag := range currentTags {
		if tag == newTag {
			tagExists = true
			break
		}
	}
	if !tagExists {
		return fmt.Errorf("unable to push tag %s, it does not exist in the repo yet", newTag)
	}

	// CHeck if tag already exists in the remote repo
	tagExists, err = gp.repo.HasRemoteTag(newTag)
	if err != nil {
		return fmt.Errorf("checking of tag %s exists: %w", newTag, err)
	}

	// If the tag already exists in the remote, we return success
	if tagExists {
		logrus.Infof("Tag %s already exists in remote. Noop.", newTag)
		return nil
	}

	logrus.Infof("Pushing%s tag for version %s", dryRunLabel[gp.opts.DryRun], newTag)

	// Push the new tag, retrying up to opts.MaxRetries times
	if err := gp.repo.Push(newTag); err != nil {
		return fmt.Errorf("pushing tag %s: %w", newTag, err)
	}

	logrus.Infof("Successfully pushed tag %s", newTag)
	return nil
}

// checkTagName verifies that the specified tag name is valid
func (gp *GitObjectPusher) checkTagName(tagName string) error {
	_, err := util.TagStringToSemver(tagName)
	if err != nil {
		return fmt.Errorf("tranforming tag into semver: %w", err)
	}
	return nil
}

// checkBranchName verifies that the branch name is valid
func (gp *GitObjectPusher) checkBranchName(branchName string) error {
	if !strings.HasPrefix(branchName, "release-") {
		return errors.New("branch name has to start with release-")
	}
	versionTag := strings.TrimPrefix(branchName, "release-")
	// Add .0 and check is we get a valid semver
	_, err := semver.Parse(versionTag + ".0")
	if err != nil {
		return fmt.Errorf("parsing semantic version in branchname: %w", err)
	}
	return nil
}

// PushMain pushes the main branch to the origin
func (gp *GitObjectPusher) PushMain() error {
	logrus.Infof("Checkout %s branch to push objects", git.DefaultBranch)
	if err := gp.repo.Checkout(git.DefaultBranch); err != nil {
		return fmt.Errorf("checking out %s branch: %w", git.DefaultBranch, err)
	}

	// logrun -v git status -s || return 1
	status, err := gp.repo.Status()
	if err != nil {
		return fmt.Errorf("while reading the repository status: %w", err)
	}
	if status.String() == "" {
		logrus.Info("Repository status: no modified paths")
	} else {
		logrus.Info(status.String())
	}

	// logrun -v git show || return 1
	lastLog, err := gp.repo.ShowLastCommit()
	if err != nil {
		return fmt.Errorf("getting last commit data from log: %w", err)
	}
	logrus.Info(lastLog)

	logrus.Info("Rebase master branch")

	_, err = gp.repo.FetchRemote(git.DefaultRemote)
	if err != nil {
		return fmt.Errorf("while fetching origin repository: %w", err)
	}

	if err := gp.repo.Rebase(fmt.Sprintf("%s/%s", git.DefaultRemote, git.DefaultBranch)); err != nil {
		return fmt.Errorf("rebasing repository: %w", err)
	}

	logrus.Infof("Pushing%s %s branch", dryRunLabel[gp.opts.DryRun], git.DefaultBranch)

	// logrun -s git push$dryrun_flag origin master || return 1
	if err := gp.repo.Push(git.DefaultBranch); err != nil {
		return fmt.Errorf("pushing %s branch: %w", git.DefaultBranch, err)
	}
	return nil
}

func (gp *GitObjectPusher) mergeRemoteIfRequired(branch string) error {
	branch = git.Remotify(branch)
	branchParts := strings.Split(branch, "/")
	logrus.Infof("Merging %s branch if required", branch)

	logrus.Infof("Fetching from %s", git.DefaultRemote)
	if _, err := gp.repo.FetchRemote(git.DefaultRemote); err != nil {
		return fmt.Errorf("fetch remote: %w", err)
	}

	branchExists, err := gp.repo.HasRemoteBranch(branchParts[1])
	if err != nil {
		return fmt.Errorf(
			"checking if branch %s exists in repo remote: %w",
			branch,
			err,
		)
	}
	if !branchExists {
		logrus.Infof(
			"Git repository does not have remote branch %s, not attempting merge", branch,
		)
		return nil
	}

	logrus.Infof("Merging %s branch", branch)
	if err := gp.repo.Merge(branch); err != nil {
		return fmt.Errorf(
			"merging remote branch %s to local repo: %w",
			branch,
			err,
		)
	}

	logrus.Info("Local branch is now up to date")
	return nil
}
