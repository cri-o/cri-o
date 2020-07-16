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

package git

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/release/pkg/command"
	"k8s.io/release/pkg/util"
)

const (
	DefaultGithubOrg         = "kubernetes"
	DefaultGithubRepo        = "kubernetes"
	DefaultGithubReleaseRepo = "sig-release"
	DefaultRemote            = "origin"
	DefaultMasterRef         = "HEAD"
	Master                   = "master"

	branchRE              = `master|release-([0-9]{1,})\.([0-9]{1,})(\.([0-9]{1,}))*$`
	defaultGithubAuthRoot = "git@github.com:"
	gitExecutable         = "git"
)

// GetDefaultKubernetesRepoURL returns the default HTTPS repo URL for Kubernetes.
// Expected: https://github.com/kubernetes/kubernetes
func GetDefaultKubernetesRepoURL() string {
	return GetKubernetesRepoURL(DefaultGithubOrg, false)
}

// GetKubernetesRepoURL takes a GitHub org and repo, and useSSH as a boolean and
// returns a repo URL for Kubernetes.
// Expected result is one of the following:
// - https://github.com/<org>/kubernetes
// - git@github.com:<org>/kubernetes
func GetKubernetesRepoURL(org string, useSSH bool) string {
	if org == "" {
		org = DefaultGithubOrg
	}

	return GetRepoURL(org, DefaultGithubRepo, useSSH)
}

// GetRepoURL takes a GitHub org and repo, and useSSH as a boolean and
// returns a repo URL for the specified repo.
// Expected result is one of the following:
// - https://github.com/<org>/<repo>
// - git@github.com:<org>/<repo>
func GetRepoURL(org, repo string, useSSH bool) (repoURL string) {
	slug := filepath.Join(org, repo)

	if useSSH {
		repoURL = fmt.Sprintf("%s%s", defaultGithubAuthRoot, slug)
	} else {
		repoURL = (&url.URL{
			Scheme: "https",
			Host:   "github.com",
			Path:   slug,
		}).String()
	}

	return repoURL
}

// DiscoverResult is the result of a revision discovery
type DiscoverResult struct {
	startSHA, startRev, endSHA, endRev string
}

// StartSHA returns the start SHA for the DiscoverResult
func (d *DiscoverResult) StartSHA() string {
	return d.startSHA
}

// StartRev returns the start revision for the DiscoverResult
func (d *DiscoverResult) StartRev() string {
	return d.startRev
}

// EndSHA returns the end SHA for the DiscoverResult
func (d *DiscoverResult) EndSHA() string {
	return d.endSHA
}

// EndRev returns the end revision for the DiscoverResult
func (d *DiscoverResult) EndRev() string {
	return d.endRev
}

// Remote is a representation of a git remote location
type Remote struct {
	name string
	urls []string
}

// NewRemote creates a new remote for the provided name and URLs
func NewRemote(name string, urls []string) *Remote {
	return &Remote{name, urls}
}

// Name returns the name of the remote
func (r *Remote) Name() string {
	return r.name
}

// URLs returns all available URLs of the remote
func (r *Remote) URLs() []string {
	return r.urls
}

// Wrapper type for a Kubernetes repository instance
type Repo struct {
	inner    Repository
	worktree Worktree
	dir      string
	dryRun   bool
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

// Repository is the main interface to the git.Repository functionality
//counterfeiter:generate . Repository
type Repository interface {
	Branches() (storer.ReferenceIter, error)
	CommitObject(plumbing.Hash) (*object.Commit, error)
	Head() (*plumbing.Reference, error)
	Remote(string) (*git.Remote, error)
	Remotes() ([]*git.Remote, error)
	ResolveRevision(plumbing.Revision) (*plumbing.Hash, error)
	Tags() (storer.ReferenceIter, error)
}

// Worktree is the main interface to the git.Worktree functionality
//counterfeiter:generate . Worktree
type Worktree interface {
	Add(string) (plumbing.Hash, error)
	Commit(string, *git.CommitOptions) (plumbing.Hash, error)
	Checkout(*git.CheckoutOptions) error
	Status() (git.Status, error)
}

// Dir returns the directory where the repository is stored on disk
func (r *Repo) Dir() string {
	return r.dir
}

// Set the repo into dry run mode, which does not modify any remote locations
// at all.
func (r *Repo) SetDry() {
	r.dryRun = true
}

// SetWorktree can be used to manually set the repository worktree
func (r *Repo) SetWorktree(worktree Worktree) {
	r.worktree = worktree
}

// SetInnerRepo can be used to manually set the inner repository
func (r *Repo) SetInnerRepo(repo Repository) {
	r.inner = repo
}

// CloneOrOpenDefaultGitHubRepoSSH clones the default Kubernetes GitHub
// repository via SSH if the repoPath is empty, otherwise updates it at the
// expected repoPath.
func CloneOrOpenDefaultGitHubRepoSSH(repoPath string) (*Repo, error) {
	return CloneOrOpenGitHubRepo(
		repoPath, DefaultGithubOrg, DefaultGithubRepo, true,
	)
}

// CleanCloneGitHubRepo creates a guaranteed fresh checkout of a given repository. The returned *Repo has a Cleanup()
// method that should be used to delete the repository on-disk afterwards.
func CleanCloneGitHubRepo(owner, repo string, useSSH bool) (*Repo, error) {
	repoURL := GetRepoURL(owner, repo, useSSH)
	// The use of a blank string for the repo path triggers special behaviour in CloneOrOpenRepo that causes a true
	// temporary directory with a random name to be created.
	return CloneOrOpenRepo("", repoURL, useSSH)
}

// CloneOrOpenGitHubRepo works with a repository in the given directory, or creates one if the directory is empty. The
// repo uses the provided GitHub repository via the owner and repo. If useSSH is true, then it will clone the
// repository using the defaultGithubAuthRoot.
func CloneOrOpenGitHubRepo(repoPath, owner, repo string, useSSH bool) (*Repo, error) {
	repoURL := GetRepoURL(owner, repo, useSSH)
	return CloneOrOpenRepo(repoPath, repoURL, useSSH)
}

// CloneOrOpenRepo creates a temp directory containing the provided
// GitHub repository via the url.
//
// If a repoPath is given, then the function tries to update the repository.
//
// The function returns the repository if cloning or updating of the repository
// was successful, otherwise an error.
func CloneOrOpenRepo(repoPath, repoURL string, useSSH bool) (*Repo, error) {
	logrus.Debugf("Using repository url %q", repoURL)
	targetDir := ""
	if repoPath != "" {
		logrus.Debugf("Using existing repository path %q", repoPath)
		_, err := os.Stat(repoPath)

		if err == nil {
			// The file or directory exists, just try to update the repo
			return updateRepo(repoPath)
		} else if os.IsNotExist(err) {
			// The directory does not exists, we still have to clone it
			targetDir = repoPath
		} else {
			// Something else bad happened
			return nil, errors.Wrap(err, "unable to update repo")
		}
	} else {
		// No repoPath given, use a random temp dir instead
		t, err := ioutil.TempDir("", "k8s-")
		if err != nil {
			return nil, errors.Wrap(err, "unable to create temp dir")
		}
		targetDir = t
	}

	if _, err := git.PlainClone(targetDir, false, &git.CloneOptions{
		URL:      repoURL,
		Progress: os.Stdout,
	}); err != nil {
		return nil, errors.Wrap(err, "unable to clone repo")
	}
	return updateRepo(targetDir)
}

// updateRepo tries to open the provided repoPath and fetches the latest
// changes from the configured remote location
func updateRepo(repoPath string) (*Repo, error) {
	r, err := OpenRepo(repoPath)
	if err != nil {
		return nil, err
	}

	// Update the repo
	if err := command.NewWithWorkDir(
		r.Dir(), gitExecutable, "pull", "--rebase",
	).RunSilentSuccess(); err != nil {
		return nil, errors.Wrap(err, "unable to pull from remote")
	}

	return r, nil
}

// OpenRepo tries to open the provided repoPath
func OpenRepo(repoPath string) (*Repo, error) {
	if !command.Available(gitExecutable) {
		return nil, errors.Errorf(
			"%s executable is not available in $PATH", gitExecutable,
		)
	}

	if strings.HasPrefix(repoPath, "~/") {
		repoPath = os.Getenv("HOME") + repoPath[1:]
		logrus.Warnf("Normalizing repository to: %s", repoPath)
	}

	r, err := git.PlainOpenWithOptions(
		repoPath, &git.PlainOpenOptions{DetectDotGit: true},
	)
	if err != nil {
		return nil, errors.Wrap(err, "opening repo")
	}

	worktree, err := r.Worktree()
	if err != nil {
		return nil, errors.Wrap(err, "getting repository worktree")
	}

	return &Repo{
		inner:    r,
		worktree: worktree,
		dir:      worktree.Filesystem.Root(),
	}, nil
}

func (r *Repo) Cleanup() error {
	logrus.Debugf("Deleting %s", r.dir)
	return os.RemoveAll(r.dir)
}

// RevParse parses a git revision and returns a SHA1 on success, otherwise an
// error.
func (r *Repo) RevParse(rev string) (string, error) {
	matched, err := regexp.MatchString(`v\d+\.\d+\.\d+.*`, rev)
	if err != nil {
		return "", err
	}
	if !matched {
		// Prefix all non-tags the default remote "origin"
		rev = Remotify(rev)
	}

	// Try to resolve the rev
	ref, err := r.inner.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return "", err
	}

	return ref.String(), nil
}

// RevParseShort parses a git revision and returns a SHA1 trimmed to the length
// 10 on success, otherwise an error.
func (r *Repo) RevParseShort(rev string) (string, error) {
	fullRev, err := r.RevParse(rev)
	if err != nil {
		return "", err
	}

	return fullRev[:10], nil
}

// LatestReleaseBranchMergeBaseToLatest tries to discover the start (latest
// v1.x.0 merge base) and end (release-1.(x+1) or master) revision inside the
// repository.
func (r *Repo) LatestReleaseBranchMergeBaseToLatest() (DiscoverResult, error) {
	// Find the last non patch version tag, then resolve its revision
	versions, err := r.latestNonPatchFinalVersions()
	if err != nil {
		return DiscoverResult{}, err
	}
	version := versions[0]
	versionTag := util.SemverToTagString(version)
	logrus.Debugf("latest non patch version %s", versionTag)

	base, err := r.MergeBase(
		Master,
		fmt.Sprintf("release-%d.%d", version.Major, version.Minor),
	)
	if err != nil {
		return DiscoverResult{}, err
	}

	// If a release branch exists for the next version, we use it. Otherwise we
	// fallback to the master branch.
	end, branch, err := r.releaseBranchOrMasterRev(version.Major, version.Minor+1)
	if err != nil {
		return DiscoverResult{}, err
	}

	return DiscoverResult{
		startSHA: base,
		startRev: versionTag,
		endSHA:   end,
		endRev:   branch,
	}, nil
}

func (r *Repo) LatestNonPatchFinalToMinor() (DiscoverResult, error) {
	// Find the last non patch version tag, then resolve its revision
	versions, err := r.latestNonPatchFinalVersions()
	if err != nil {
		return DiscoverResult{}, err
	}
	if len(versions) < 2 {
		return DiscoverResult{}, errors.New("unable to find two latest non patch versions")
	}

	latestVersion := versions[0]
	latestVersionTag := util.SemverToTagString(latestVersion)
	logrus.Debugf("latest non patch version %s", latestVersionTag)
	end, err := r.RevParse(latestVersionTag)
	if err != nil {
		return DiscoverResult{}, err
	}

	previousVersion := versions[1]
	previousVersionTag := util.SemverToTagString(previousVersion)
	logrus.Debugf("previous non patch version %s", previousVersionTag)
	start, err := r.RevParse(previousVersionTag)
	if err != nil {
		return DiscoverResult{}, err
	}

	return DiscoverResult{
		startSHA: start,
		startRev: previousVersionTag,
		endSHA:   end,
		endRev:   latestVersionTag,
	}, nil
}

func (r *Repo) latestNonPatchFinalVersions() ([]semver.Version, error) {
	latestVersions := []semver.Version{}

	tags, err := r.inner.Tags()
	if err != nil {
		return nil, err
	}

	_ = tags.ForEach(func(t *plumbing.Reference) error { // nolint: errcheck
		tag := util.TrimTagPrefix(t.Name().Short())
		ver, err := semver.Make(tag)

		if err == nil {
			// We're searching for the latest, non patch final tag
			if ver.Patch == 0 && len(ver.Pre) == 0 {
				if len(latestVersions) == 0 || ver.GT(latestVersions[0]) {
					latestVersions = append([]semver.Version{ver}, latestVersions...)
				}
			}
		}
		return nil
	})
	if len(latestVersions) == 0 {
		return nil, fmt.Errorf("unable to find latest non patch release")
	}
	return latestVersions, nil
}

func (r *Repo) releaseBranchOrMasterRev(major, minor uint64) (sha, rev string, err error) {
	relBranch := fmt.Sprintf("release-%d.%d", major, minor)
	sha, err = r.RevParse(relBranch)
	if err == nil {
		logrus.Debugf("found release branch %s", relBranch)
		return sha, relBranch, nil
	}

	sha, err = r.RevParse(Master)
	if err == nil {
		logrus.Debug("no release branch found, using master")
		return sha, Master, nil
	}

	return "", "", err
}

// HasRemoteBranch takes a branch string and verifies that it exists
// on the default remote
func (r *Repo) HasRemoteBranch(branch string) error {
	logrus.Infof("Verifying %s branch exists on the remote", branch)

	remote, err := r.inner.Remote(DefaultRemote)
	if err != nil {
		return err
	}

	// We can then use every Remote functions to retrieve wanted information
	refs, err := remote.List(&git.ListOptions{})
	if err != nil {
		logrus.Warn("Could not list references on the remote repository.")
		return err
	}

	for _, ref := range refs {
		if ref.Name().IsBranch() {
			if ref.Name().Short() == branch {
				logrus.Infof("Found branch %s", ref.Name().Short())
				return nil
			}
		}
	}
	return errors.Errorf("branch %v not found", branch)
}

// Checkout can be used to checkout any revision inside the repository
func (r *Repo) Checkout(rev string, args ...string) error {
	cmdArgs := append([]string{"checkout", rev}, args...)
	return command.
		NewWithWorkDir(r.Dir(), gitExecutable, cmdArgs...).
		RunSilentSuccess()
}

// IsReleaseBranch returns true if the provided branch is a Kubernetes release
// branch
func IsReleaseBranch(branch string) bool {
	re := regexp.MustCompile(branchRE)
	if !re.MatchString(branch) {
		logrus.Warnf("%s is not a release branch", branch)
		return false
	}

	return true
}

func (r *Repo) MergeBase(from, to string) (string, error) {
	masterRef := Remotify(from)
	releaseRef := Remotify(to)

	logrus.Debugf("masterRef: %s, releaseRef: %s", masterRef, releaseRef)

	commitRevs := []string{masterRef, releaseRef}
	var res []*object.Commit

	hashes := []*plumbing.Hash{}
	for _, rev := range commitRevs {
		hash, err := r.inner.ResolveRevision(plumbing.Revision(rev))
		if err != nil {
			return "", err
		}
		hashes = append(hashes, hash)
	}

	commits := []*object.Commit{}
	for _, hash := range hashes {
		commit, err := r.inner.CommitObject(*hash)
		if err != nil {
			return "", err
		}
		commits = append(commits, commit)
	}

	res, err := commits[0].MergeBase(commits[1])
	if err != nil {
		return "", err
	}

	if len(res) == 0 {
		return "", errors.Errorf("could not find a merge base between %s and %s", from, to)
	}

	mergeBase := res[0].Hash.String()
	logrus.Infof("merge base is %s", mergeBase)

	return mergeBase, nil
}

// Remotify returns the name prepended with the default remote
func Remotify(name string) string {
	split := strings.Split(name, "/")
	if len(split) > 1 {
		return name
	}
	return fmt.Sprintf("%s/%s", DefaultRemote, name)
}

// Merge does a git merge into the current branch from the provided one
func (r *Repo) Merge(from string) error {
	return command.NewWithWorkDir(
		r.Dir(), gitExecutable, "merge", "-X", "ours", from,
	).RunSuccess()
}

// Push does push the specified branch to the default remote, but only if the
// repository is not in dry run mode
func (r *Repo) Push(remoteBranch string) error {
	args := []string{"push"}
	if r.dryRun {
		logrus.Infof("Won't push due to dry run repository")
		args = append(args, "--dry-run")
	}
	args = append(args, DefaultRemote, remoteBranch)

	return command.NewWithWorkDir(r.Dir(), gitExecutable, args...).RunSuccess()
}

// Head retrieves the current repository HEAD as a string
func (r *Repo) Head() (string, error) {
	ref, err := r.inner.Head()
	if err != nil {
		return "", err
	}
	return ref.Hash().String(), nil
}

// LatestPatchToPatch tries to discover the start (latest v1.x.[x-1]) and
// end (latest v1.x.x) revision inside the repository for the specified release
// branch.
func (r *Repo) LatestPatchToPatch(branch string) (DiscoverResult, error) {
	latestTag, err := r.LatestTagForBranch(branch)
	if err != nil {
		return DiscoverResult{}, err
	}

	if len(latestTag.Pre) > 0 && latestTag.Patch > 0 {
		latestTag.Patch--
		latestTag.Pre = nil
	}

	if latestTag.Patch == 0 {
		return DiscoverResult{}, errors.Errorf(
			"found non-patch version %v as latest tag on branch %s",
			latestTag, branch,
		)
	}

	prevTag := semver.Version{
		Major: latestTag.Major,
		Minor: latestTag.Minor,
		Patch: latestTag.Patch - 1,
	}

	logrus.Debugf("parsing latest tag %s%v", util.TagPrefix, latestTag)
	latestVersionTag := util.SemverToTagString(latestTag)
	end, err := r.RevParse(latestVersionTag)
	if err != nil {
		return DiscoverResult{}, errors.Wrapf(err, "parsing version %v", latestTag)
	}

	logrus.Debugf("parsing previous tag %s%v", util.TagPrefix, prevTag)
	previousVersionTag := util.SemverToTagString(prevTag)
	start, err := r.RevParse(previousVersionTag)
	if err != nil {
		return DiscoverResult{}, errors.Wrapf(err, "parsing previous version %v", prevTag)
	}

	return DiscoverResult{
		startSHA: start,
		startRev: previousVersionTag,
		endSHA:   end,
		endRev:   latestVersionTag,
	}, nil
}

// LatestPatchToLatest tries to discover the start (latest v1.x.x]) and
// end (release-1.x or master) revision inside the repository for the specified release
// branch.
func (r *Repo) LatestPatchToLatest(branch string) (DiscoverResult, error) {
	latestTag, err := r.LatestTagForBranch(branch)
	if err != nil {
		return DiscoverResult{}, err
	}

	if len(latestTag.Pre) > 0 && latestTag.Patch > 0 {
		latestTag.Patch--
		latestTag.Pre = nil
	}

	logrus.Debugf("parsing latest tag %s%v", util.TagPrefix, latestTag)
	latestVersionTag := util.SemverToTagString(latestTag)
	start, err := r.RevParse(latestVersionTag)
	if err != nil {
		return DiscoverResult{}, errors.Wrapf(err, "parsing version %v", latestTag)
	}

	// If a release branch exists for the latest version, we use it. Otherwise we
	// fallback to the master branch.
	end, branch, err := r.releaseBranchOrMasterRev(latestTag.Major, latestTag.Minor)
	if err != nil {
		return DiscoverResult{}, errors.Wrapf(err, "getting release branch for %v", latestTag)
	}

	return DiscoverResult{
		startSHA: start,
		startRev: latestVersionTag,
		endSHA:   end,
		endRev:   branch,
	}, nil
}

// LatestTagForBranch returns the latest available semver tag for a given branch
func (r *Repo) LatestTagForBranch(branch string) (tag semver.Version, err error) {
	tags, err := r.TagsForBranch(branch)
	if err != nil {
		return tag, err
	}
	if len(tags) == 0 {
		return tag, errors.New("no tags found on branch")
	}

	tag, err = util.TagStringToSemver(tags[0])
	if err != nil {
		return tag, err
	}

	return tag, nil
}

// PreviousTag tries to find the previous tag for a provided branch and errors
// on any failure
func (r *Repo) PreviousTag(tag, branch string) (string, error) {
	tags, err := r.TagsForBranch(branch)
	if err != nil {
		return "", err
	}

	idx := -1
	for i, t := range tags {
		if t == tag {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "", errors.New("could not find specified tag in branch")
	}
	if len(tags) < idx+1 {
		return "", errors.New("unable to find previous tag")
	}

	return tags[idx+1], nil
}

// TagsForBranch returns a list of tags for the provided branch sorted by
// creation date
func (r *Repo) TagsForBranch(branch string) (res []string, err error) {
	previousBranch, err := r.CurrentBranch()
	if err != nil {
		return nil, errors.Wrap(err, "retrieving current branch")
	}
	if err := r.Checkout(branch); err != nil {
		return nil, errors.Wrapf(err, "checking out %s", branch)
	}
	defer func() { err = r.Checkout(previousBranch) }()

	status, err := command.NewWithWorkDir(
		r.Dir(), gitExecutable, "tag", "--sort=-creatordate", "--merged",
	).RunSilentSuccessOutput()
	if err != nil {
		return nil, errors.Wrapf(err, "retrieving merged tags for branch %s", branch)
	}

	return strings.Fields(status.Output()), nil
}

// Add adds a file to the staging area of the repo
func (r *Repo) Add(filename string) error {
	return errors.Wrapf(
		command.NewWithWorkDir(
			r.Dir(), gitExecutable, "add", filename,
		).RunSilentSuccess(),
		"adding file %s to repository", filename,
	)
}

// UserCommit makes a commit using the local user's config
func (r *Repo) UserCommit(msg string) error {
	// amend the latest commit
	userName, err := command.New("git", "config", "--get", "user.name").RunSuccessOutput()
	if err != nil {
		return errors.Wrap(err, "while trying to get the user's name")
	}

	userEmail, err := command.New("git", "config", "--get", "user.email").RunSuccessOutput()
	if err != nil {
		return errors.Wrap(err, "while trying to get the user's name")
	}

	if err := r.CommitWithOptions(msg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  userName.OutputTrimNL(),
			Email: userEmail.OutputTrimNL(),
			When:  time.Now(),
		},
	}); err != nil {
		return err
	}

	return nil
}

// Commit commits the current repository state
func (r *Repo) Commit(msg string) error {
	if err := r.CommitWithOptions(msg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Anago GCB",
			Email: "nobody@k8s.io",
			When:  time.Now(),
		},
	}); err != nil {
		return err
	}
	return nil
}

// CommitWithOptions commits the current repository state
func (r *Repo) CommitWithOptions(msg string, options *git.CommitOptions) error {
	if _, err := r.worktree.Commit(msg, options); err != nil {
		return err
	}
	return nil
}

// CurrentBranch returns the current branch of the repository or an error in
// case of any failure
func (r *Repo) CurrentBranch() (branch string, err error) {
	branches, err := r.inner.Branches()
	if err != nil {
		return "", err
	}

	head, err := r.inner.Head()
	if err != nil {
		return "", err
	}

	if err := branches.ForEach(func(ref *plumbing.Reference) error {
		if ref.Hash() == head.Hash() {
			branch = ref.Name().Short()
			return nil
		}

		return nil
	}); err != nil {
		return "", err
	}

	return branch, nil
}

// Rm removes files from the repository
func (r *Repo) Rm(force bool, files ...string) error {
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, files...)

	return command.
		NewWithWorkDir(r.Dir(), gitExecutable, args...).
		RunSilentSuccess()
}

// Remotes lists the currently available remotes for the repository
func (r *Repo) Remotes() (res []*Remote, err error) {
	remotes, err := r.inner.Remotes()
	if err != nil {
		return nil, errors.Wrap(err, "unable to list remotes")
	}

	// Sort the remotes by their name which is not always the case
	sort.Slice(remotes, func(i, j int) bool {
		return remotes[i].Config().Name < remotes[j].Config().Name
	})

	for _, remote := range remotes {
		config := remote.Config()
		res = append(res, &Remote{name: config.Name, urls: config.URLs})
	}

	return res, nil
}

// HasRemote checks if the provided remote `name` is available and matches the
// expected `url`
func (r *Repo) HasRemote(name, expectedURL string) bool {
	remotes, err := r.Remotes()
	if err != nil {
		logrus.Warnf("Unable to get repository remotes: %v", err)
		return false
	}

	for _, remote := range remotes {
		if remote.Name() == name {
			for _, url := range remote.URLs() {
				if url == expectedURL {
					return true
				}
			}
		}
	}

	return false
}

// AddRemote adds a new remote to the current working tree
func (r *Repo) AddRemote(name, owner, repo string) error {
	repoURL := GetRepoURL(owner, repo, true)
	args := []string{"remote", "add", name, repoURL}
	return command.
		NewWithWorkDir(r.Dir(), gitExecutable, args...).
		RunSilentSuccess()
}

// PushToRemote push the current branch to a spcified remote, but only if the
// repository is not in dry run mode
func (r *Repo) PushToRemote(remote, remoteBranch string) error {
	args := []string{"push", "--set-upstream"}
	if r.dryRun {
		logrus.Infof("Won't push due to dry run repository")
		args = append(args, "--dry-run")
	}
	args = append(args, remote, remoteBranch)

	return command.NewWithWorkDir(r.Dir(), gitExecutable, args...).RunSuccess()
}

// LsRemote can be used to run `git ls-remote` with the provided args on the
// repository
func (r *Repo) LsRemote(args ...string) (string, error) {
	return r.runGitCmd("ls-remote", args...)
}

// Branch can be used to run `git branch` with the provided args on the
// repository
func (r *Repo) Branch(args ...string) (string, error) {
	return r.runGitCmd("branch", args...)
}

// runGitCmd runs the provided command in the repository root and appends the
// args. The command will run silently and return the captured output or an
// error in case of any failure.
func (r *Repo) runGitCmd(cmd string, args ...string) (string, error) {
	cmdArgs := append([]string{cmd}, args...)
	res, err := command.NewWithWorkDir(
		r.Dir(), gitExecutable, cmdArgs...,
	).RunSilentSuccessOutput()
	if err != nil {
		return "", errors.Wrapf(err, "running git %s", cmd)
	}
	return res.OutputTrimNL(), nil
}

// IsDirty returns true if the worktree status is not clean. It can also error
// if the worktree status is not retrievable.
func (r *Repo) IsDirty() (bool, error) {
	status, err := r.worktree.Status()
	if err != nil {
		return false, errors.Wrap(err, "retrieving worktree status")
	}
	return !status.IsClean(), nil
}
