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

package options

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/github"
)

// Options is the global options structure which can be used to build release
// notes generator options
type Options struct {
	// GithubOrg specifies the GitHub organization from which will be
	// cloned/pulled if Pull is true.
	GithubOrg string

	// GithubRepo specifies the GitHub repository from which will be
	// cloned/pulled if Pull is true.
	GithubRepo string

	// RepoPath specifies the git repository location for doing an update if
	// Pull is true.
	RepoPath string

	// Branch will be used for discovering the latest patch version if
	// DiscoverMode is RevisionDiscoveryModePatchToPatch.
	Branch string

	// StartSHA can be used to set the release notes start revision to an
	// exact git SHA. Should not be used together with StartRev.
	StartSHA string

	// EndSHA can be used to set the release notes end revision to an
	// exact git SHA. Should not be used together with EndRev.
	EndSHA string

	// StartRev can be used to set the release notes start revision to any
	// valid git revision. Should not be used together with StartSHA.
	StartRev string

	// EndRev can be used to set the release notes end revision to any
	// valid git revision. Should not be used together with EndSHA.
	EndRev string

	// ReleaseVersion is the version of the release. This option is just passed
	// through into the resulting ReleaseNote struct for identifying releases
	// on JSON output.
	ReleaseVersion string

	// Format specifies the format of the release notes. Can be either
	// FormatSpecNone, FormatSpecJSON, or FormatSpecDefaultGoTemplate
	Format string

	// RequiredAuthor can be used to filter the release notes by the commit
	// author
	RequiredAuthor string

	// DiscoverMode can be used to automatically discover StartSHA and EndSHA.
	// Can be either RevisionDiscoveryModeNONE (default),
	// RevisionDiscoveryModeMergeBaseToLatest,
	// RevisionDiscoveryModePatchToPatch, or RevisionDiscoveryModeMinorToMinor.
	// Should not be used together with StartRev, EndRev, StartSHA or EndSHA.
	DiscoverMode string

	// ReleaseTars specifies the directory where the release tarballs are
	// located.
	ReleaseTars string

	// ReleaseBucket specifies the Google Cloud bucket where the ReleaseTars
	// are linked to. This option is used for generating the links inside the
	// release downloads table.
	ReleaseBucket string

	// If true, then the release notes generator will pull in latest changes
	// from the default git remote
	Pull bool

	// If true, then the release notes generator will print messages in debug
	// log level
	Debug bool

	// RecordDir specifies the directory for API call recordings. Cannot be
	// used together with ReplayDir.
	RecordDir string

	// ReplayDir specifies the directory for replaying a previously recorded
	// API. Cannot be used together with RecordDir.
	ReplayDir string

	githubToken string
	gitCloneFn  func(string, string, string, bool) (*git.Repo, error)
}

type RevisionDiscoveryMode string

const (
	RevisionDiscoveryModeNONE              = "none"
	RevisionDiscoveryModeMergeBaseToLatest = "mergebase-to-latest"
	RevisionDiscoveryModePatchToPatch      = "patch-to-patch"
	RevisionDiscoveryModePatchToLatest     = "patch-to-latest"
	RevisionDiscoveryModeMinorToMinor      = "minor-to-minor"
)

const (
	FormatSpecNone              = ""
	FormatSpecJSON              = "json"
	FormatSpecDefaultGoTemplate = GoTemplatePrefix + "default"
	FormatSpecGoTemplateInline  = GoTemplatePrefix + "inline:"
	GoTemplatePrefix            = "go-template:"
)

// New creates a new Options instance with the default values
func New() *Options {
	return &Options{
		DiscoverMode: RevisionDiscoveryModeNONE,
		GithubOrg:    git.DefaultGithubOrg,
		GithubRepo:   git.DefaultGithubRepo,
		Pull:         true,
		gitCloneFn:   git.CloneOrOpenGitHubRepo,
	}
}

// ValidateAndFinish checks if the options are set in a consistent way and
// adapts them if necessary. It returns an error if options are set to invalid
// values.
func (o *Options) ValidateAndFinish() (err error) {
	// Add appropriate log filtering
	if o.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	if o.ReplayDir != "" && o.RecordDir != "" {
		return errors.New("please do not use record and replay together")
	}

	// Recover for replay if needed
	if o.ReplayDir != "" {
		logrus.Info("using replay mode")
		return nil
	}

	// The GitHub Token is required if replay is not specified
	token, ok := os.LookupEnv(github.TokenEnvKey)
	if ok {
		o.githubToken = token
	} else if o.ReplayDir == "" {
		return errors.Errorf(
			"neither environment variable `%s` nor `replay` option is set",
			github.TokenEnvKey,
		)
	}

	// Check if we want to automatically discover the revisions
	if o.DiscoverMode != RevisionDiscoveryModeNONE {
		if err := o.resolveDiscoverMode(); err != nil {
			return err
		}
	}

	// The start SHA or rev is required.
	if o.StartSHA == "" && o.StartRev == "" {
		return errors.New("the starting commit hash must be set via --start-sha, $START_SHA, --start-rev or $START_REV")
	}

	// The end SHA or rev is required.
	if o.EndSHA == "" && o.EndRev == "" {
		return errors.New("the ending commit hash must be set via --end-sha, $END_SHA, --end-rev or $END_REV")
	}

	// Check if we have to parse a revision
	if (o.StartRev != "" && o.StartSHA == "") || (o.EndRev != "" && o.EndSHA == "") {
		repo, err := o.repo()
		if err != nil {
			return err
		}
		if o.StartRev != "" && o.StartSHA == "" {
			sha, err := repo.RevParse(o.StartRev)
			if err != nil {
				return errors.Wrapf(err, "resolving %s", o.StartRev)
			}
			logrus.Infof("using found start SHA: %s", sha)
			o.StartSHA = sha
		}
		if o.EndRev != "" && o.EndSHA == "" {
			sha, err := repo.RevParse(o.EndRev)
			if err != nil {
				return errors.Wrapf(err, "resolving %s", o.EndRev)
			}
			logrus.Infof("using found end SHA: %s", sha)
			o.EndSHA = sha
		}
	}

	// Create the record dir
	if o.RecordDir != "" {
		logrus.Info("using record mode")
		if err := os.MkdirAll(o.RecordDir, os.FileMode(0755)); err != nil {
			return err
		}
	}

	// Set the format
	if o.Format == FormatSpecNone {
		o.Format = FormatSpecDefaultGoTemplate
	}

	return nil
}

func (o *Options) resolveDiscoverMode() error {
	repo, err := o.repo()
	if err != nil {
		return err
	}

	var result git.DiscoverResult
	if o.DiscoverMode == RevisionDiscoveryModeMergeBaseToLatest {
		result, err = repo.LatestReleaseBranchMergeBaseToLatest()
	} else if o.DiscoverMode == RevisionDiscoveryModePatchToPatch {
		result, err = repo.LatestPatchToPatch(o.Branch)
	} else if o.DiscoverMode == RevisionDiscoveryModePatchToLatest {
		result, err = repo.LatestPatchToLatest(o.Branch)
	} else if o.DiscoverMode == RevisionDiscoveryModeMinorToMinor {
		result, err = repo.LatestNonPatchFinalToMinor()
	}
	if err != nil {
		return err
	}

	o.StartSHA = result.StartSHA()
	o.StartRev = result.StartRev()
	o.EndSHA = result.EndSHA()
	o.EndRev = result.EndRev()

	logrus.Infof("discovered start SHA %s", o.StartSHA)
	logrus.Infof("discovered end SHA %s", o.EndSHA)

	logrus.Infof("using start revision %s", o.StartRev)
	logrus.Infof("using end revision %s", o.EndRev)

	return nil
}

func (o *Options) repo() (repo *git.Repo, err error) {
	if o.Pull {
		logrus.Infof("cloning/updating repository %s/%s", o.GithubOrg, o.GithubRepo)
		repo, err = o.gitCloneFn(
			o.RepoPath,
			o.GithubOrg,
			o.GithubRepo,
			false,
		)
	} else {
		logrus.Infof("re-using local repo %s", o.RepoPath)
		repo, err = git.OpenRepo(o.RepoPath)
	}
	if err != nil {
		return nil, err
	}
	return repo, nil
}

// Client returns a Client to be used by the Gatherer. Depending on
// the provided options this is either a real client talking to the GitHub API,
// a Client which in addition records the responses from Github and stores them
// on disk, or a Client that replays those pre-recorded responses and does not
// talk to the GitHub API at all.
func (o *Options) Client() (github.Client, error) {
	if o.ReplayDir != "" {
		return github.NewReplayer(o.ReplayDir), nil
	}

	// Create a real GitHub API client
	gh, err := github.NewWithToken(o.githubToken)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create GitHub client")
	}

	if o.RecordDir != "" {
		return github.NewRecorder(gh.Client(), o.RecordDir), nil
	}

	return gh.Client(), nil
}
