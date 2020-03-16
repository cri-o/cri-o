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

type Options struct {
	GithubOrg       string
	GithubRepo      string
	Output          string
	Branch          string
	StartSHA        string
	EndSHA          string
	StartRev        string
	EndRev          string
	RepoPath        string
	ReleaseVersion  string
	Format          string
	RequiredAuthor  string
	DiscoverMode    string
	ReleaseBucket   string
	ReleaseTars     string
	TableOfContents bool
	Debug           bool
	Pull            bool
	RecordDir       string
	ReplayDir       string
	githubToken     string
	gitCloneFn      func(string, string, string, bool) (*git.Repo, error)
}

type RevisionDiscoveryMode string

const (
	RevisionDiscoveryModeNONE              = "none"
	RevisionDiscoveryModeMergeBaseToLatest = "mergebase-to-latest"
	RevisionDiscoveryModePatchToPatch      = "patch-to-patch"
	RevisionDiscoveryModeMinorToMinor      = "minor-to-minor"
)

const (
	FormatSpecNone              = ""
	FormatSpecJSON              = "json"
	FormatSpecDefaultGoTemplate = "go-template:default"

	// Deprecated: This option is internaly translated to `FormatSpecDefaultGoTemplate`
	FormatSpecMarkdown = "markdown"
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
				return err
			}
			logrus.Infof("using found start SHA: %s", sha)
			o.StartSHA = sha
		}
		if o.EndRev != "" && o.EndSHA == "" {
			sha, err := repo.RevParse(o.EndRev)
			if err != nil {
				return err
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
	// TODO: Remove "markdown" after some time as it is deprecated in PR#1008
	if o.Format == FormatSpecMarkdown || o.Format == FormatSpecNone {
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
