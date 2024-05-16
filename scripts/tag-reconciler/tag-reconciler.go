package main

import (
	"fmt"
	"slices"

	"github.com/cri-o/cri-o/internal/version"
	"github.com/cri-o/cri-o/scripts/utils"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-sdk/git"
	"sigs.k8s.io/release-utils/command"
	"sigs.k8s.io/release-utils/env"
)

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	if err := run(); err != nil {
		logrus.Fatalf("Unable to run: %v", err)
	}
}

func run() error {
	if !env.IsSet(utils.GithubTokenEnvKey) {
		return fmt.Errorf("environment variable %q is not set", utils.GithubTokenEnvKey)
	}

	remote := env.Default(utils.GitRemoteEnvKey, "origin")
	logrus.Infof("Using repository remote: %s", remote)

	org := env.Default(utils.OrgEnvKey, utils.CrioOrgRepo)
	logrus.Infof("Using repository organization: %s", org)

	if err := git.ConfigureGlobalDefaultUserAndEmail(); err != nil {
		return fmt.Errorf("unable to configure global default user and email: %w", err)
	}

	repo, err := git.OpenRepo(".")
	if err != nil {
		return fmt.Errorf("unable to open local repository: %w", err)
	}

	for _, minorVersion := range version.ReleaseMinorVersions {
		baseBranchName := utils.BranchPrefix + minorVersion // Creates "release-x.y" format.

		sv, err := utils.GetCurrentVersionFromReleaseBranch(repo, baseBranchName) // Returns "x.y.z" format.
		if err != nil {
			return fmt.Errorf("unable to read current version from release branch %q: %w", baseBranchName, err)
		}

		currentReleaseVersion := utils.VersionPrefix + sv.String()
		exists, err := hasCurrentReleaseVersionTag(repo, baseBranchName, currentReleaseVersion)
		if err != nil {
			return fmt.Errorf("unable to retrieve version for release branch %q: %w", minorVersion, err)
		}
		if !exists {
			if err := pushTagToRemote(repo, currentReleaseVersion, remote); err != nil {
				return fmt.Errorf("unable to push tag %q: %w", currentReleaseVersion, err)
			}
		} else {
			logrus.Info("Version already exists on remote, skipping")
		}
	}

	return nil
}

func pushTagToRemote(repo *git.Repo, tag, remote string) error {
	logrus.Infof("Adding tag to repository: %s", tag)
	if err := repo.Tag(tag, tag); err != nil {
		return fmt.Errorf("unable to tag repository: %w", err)
	}

	logrus.Infof("Pushing tag to origin: %s", tag)
	if err := command.NewWithWorkDir(repo.Dir(), "git", "push", remote, "tag", tag).RunSilentSuccess(); err != nil {
		return fmt.Errorf("unable to run git push: %w", err)
	}

	logrus.Infof("Running GitHub `test` workflow")
	if err := command.NewWithWorkDir(repo.Dir(), "gh", "workflow", "run", "test", "--ref", tag).RunSilentSuccess(); err != nil {
		return fmt.Errorf("unable to run GitHub workflow: %w", err)
	}

	return nil
}

func hasCurrentReleaseVersionTag(repo *git.Repo, baseBranchName, tag string) (exists bool, err error) {
	existingTags, err := repo.TagsForBranch(baseBranchName)
	if err != nil {
		return false, fmt.Errorf("unable to read remote tags for branch %q: %w", baseBranchName, err)
	}

	return slices.Contains(existingTags, tag), nil
}
