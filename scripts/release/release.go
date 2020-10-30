package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/release/pkg/command"
	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/github"
	"k8s.io/release/pkg/util"

	"github.com/cri-o/cri-o/internal/version"
)

const (
	githubTokenEnvKey = "GITHUB_TOKEN"
	gitRemoteEnvKey   = "REMOTE"
	orgEnvKey         = "ORG"
	versionFile       = "internal/version/version.go"
	branchPrefix      = "release-"
)

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	if err := run(); err != nil {
		logrus.Fatalf("Unable to %v", err)
	}
}

func run() error {
	if !util.IsEnvSet(githubTokenEnvKey) {
		return errors.Errorf(
			"run: $%s environemt variable is not set", githubTokenEnvKey,
		)
	}
	if !util.IsEnvSet(orgEnvKey) {
		return errors.Errorf(
			"run: $%s environemt variable is not set %s",
			orgEnvKey,
			"(should be set to your CRI-O fork organization, like 'gh-name')",
		)
	}
	remote := util.EnvDefault(gitRemoteEnvKey, "origin")
	logrus.Infof("Using repository fork remote: %s", remote)

	org := os.Getenv(orgEnvKey)
	logrus.Infof("Using repository fork organization: %s", org)

	repo, err := git.OpenRepo(".")
	if err != nil {
		return errors.Wrap(err, "open local repo")
	}

	currentBranch, err := repo.CurrentBranch()
	if err != nil {
		return errors.Wrap(err, "get current branch")
	}
	logrus.Infof("Using current branch: %s", currentBranch)

	newVersion, err := incVersion(version.Version, currentBranch)
	if err != nil {
		return errors.Wrap(err, "increment version")
	}
	logrus.Infof("Using new version: %s", util.SemverToTagString(newVersion))

	if err := updateVersionAndCreatePR(
		repo, newVersion, currentBranch, org, remote,
	); err != nil {
		return errors.Wrap(err, "update version in local repository")
	}

	return nil
}

func incVersion(tag, branch string) (res semver.Version, err error) {
	sv, err := util.TagStringToSemver(strings.TrimSpace(tag))
	if err != nil {
		return res, errors.Wrapf(err, "convert tag string %s to semver", tag)
	}

	// clear any suffix like `-dev`
	sv.Pre = nil

	if branch == git.Master {
		// New minor version
		sv.Minor++
		sv.Patch = 0
	} else {
		// New patch version
		sv.Patch++
	}

	return sv, nil
}

func updateVersionAndCreatePR(
	repo *git.Repo, newVersion semver.Version, branch, org, remote string,
) error {
	logrus.Info("Updating repository")

	newBranch := branchPrefix + newVersion.String()
	logrus.Infof("Switching to branch: %s", newBranch)
	if err := repo.Checkout("-B", newBranch); err != nil {
		return errors.Wrapf(err, "checkout branch %s", newBranch)
	}

	logrus.Infof("Updating new tag in file %s", versionFile)
	if err := command.
		New(
			"sed", "-i",
			fmt.Sprintf("s/%s/%s/", version.Version, newVersion),
			versionFile,
		).RunSilentSuccess(); err != nil {
		return errors.Wrap(err, "update version file")
	}

	logrus.Info("Committing changes")
	if err := repo.Add(versionFile); err != nil {
		return errors.Wrapf(err, "add file %s to repo", versionFile)
	}
	if err := repo.UserCommit(
		"Bump version to " + newVersion.String(),
	); err != nil {
		return errors.Wrap(err, "commit changes")
	}

	logrus.Info("Pushing changes")
	if err := repo.PushToRemote(remote, newBranch); err != nil {
		return errors.Wrapf(err, "pushing to remote: %s", remote)
	}

	logrus.Info("Creating PR")
	gh := github.New()

	pr, err := gh.CreatePullRequest("cri-o", "cri-o", branch,
		fmt.Sprintf("%s:%s", org, newBranch),
		fmt.Sprintf("Bump version for %s", newVersion),
		fmt.Sprintf(
			"Automated version bump to version `%s`\n\n%s",
			newVersion, "/release-note-none",
		),
	)
	if err != nil {
		return errors.Wrap(err, "create pull request")
	}
	logrus.Infof("Created PR #%d", pr.GetNumber())

	return nil
}
