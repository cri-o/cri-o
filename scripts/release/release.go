package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/cri-o/cri-o/internal/version"
	"github.com/cri-o/cri-o/scripts/utils"
	goGit "github.com/go-git/go-git/v5"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-sdk/git"
	"sigs.k8s.io/release-sdk/github"
	"sigs.k8s.io/release-utils/env"
)

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	if err := run(); err != nil {
		logrus.Fatalf("Unable to %v", err)
	}
}

func run() error {
	if !env.IsSet(utils.GithubTokenEnvKey) {
		return fmt.Errorf(
			"run: $%s environment variable is not set", utils.GithubTokenEnvKey,
		)
	}

	remote := env.Default(utils.GitRemoteEnvKey, "origin")
	logrus.Infof("Using repository fork remote: %s", remote)

	org := env.Default(utils.OrgEnvKey, utils.CrioOrgRepo)
	logrus.Infof("Using repository fork organization: %s", org)

	if err := git.ConfigureGlobalDefaultUserAndEmail(); err != nil {
		return fmt.Errorf("unable to configure global default user and email: %w", err)
	}

	repo, err := git.OpenRepo(".")
	if err != nil {
		return fmt.Errorf("unable to open local repo: %w", err)
	}

	for _, minorVersion := range version.ReleaseMinorVersions {
		baseBranchName := utils.BranchPrefix + minorVersion // returns "release-x.y"

		sv, err := utils.GetCurrentVersionFromReleaseBranch(repo, baseBranchName) // returns "x.y.z"
		if err != nil {
			return fmt.Errorf("unable to read current version from release branch %q: %w", baseBranchName, err)
		}

		// Bump up the patch version
		oldVersion := sv.String()
		sv.Patch++
		newVersion := sv.String()

		if err := updateVersionAndCreatePR(
			repo, newVersion, oldVersion, baseBranchName, org, remote,
		); err != nil {
			return fmt.Errorf("unable to update version in local repository: %w", err)
		}
	}

	return nil
}

func updateVersionAndCreatePR(
	repo *git.Repo, newVersion, oldVersion string, baseBranchName, org, remote string,
) error {
	logrus.Infof("Updating repository from %s to %s", oldVersion, newVersion)

	newBranch := utils.BranchPrefix + newVersion
	doesTheBranchExistRemotely, err := repo.HasRemoteBranch(newBranch)
	if err != nil {
		return fmt.Errorf("unable to assert remote has branch %q: %w", newBranch, err)
	}

	if doesTheBranchExistRemotely {
		// Only Rebase and force push
		if err := repo.Rebase(newBranch); err != nil {
			return fmt.Errorf("unable to rebase branch %q: %w", newBranch, err)
		}
		opts := &goGit.PushOptions{
			Force: true,
		}
		if err := repo.PushToRemoteWithOptions(opts); err != nil {
			return fmt.Errorf("unable to force push to remote: %q: %w", newBranch, err)
		}
		return nil
	}
	logrus.Infof("Switching to branch: %s", newBranch)
	if err := repo.Checkout("-B", newBranch); err != nil {
		return fmt.Errorf("unable to checkout branch %q: %w", newBranch, err)
	}

	logrus.Infof("Updating new tag in file %s", utils.VersionFile)
	if err := modifyVersionFile(utils.VersionFile, oldVersion, newVersion); err != nil {
		return fmt.Errorf("unable to update version file: %w", err)
	}

	logrus.Info("Committing changes")
	if err := repo.Add(utils.VersionFile); err != nil {
		return fmt.Errorf("unable to add file %q to repo: %w", utils.VersionFile, err)
	}

	if err := repo.UserCommit(
		"version: bump to " + newVersion,
	); err != nil {
		return fmt.Errorf("unable to commit changes: %w", err)
	}

	logrus.Info("Pushing changes")
	if err := repo.PushToRemote(remote, newBranch); err != nil {
		return fmt.Errorf("unable to push to remote: %q: %w", remote, err)
	}

	logrus.Info("Creating PR")
	gh := github.New()

	headBranchName := fmt.Sprintf("%s:%s", org, newBranch)
	title := "Bump version to " + newVersion
	body := fmt.Sprintf(
		"Automated version bump to version `%s`\n\n```release-note\nNone\n```",
		newVersion,
	)

	pr, err := gh.CreatePullRequest(org, utils.CrioOrgRepo, baseBranchName,
		headBranchName,
		title,
		body,
	)
	if err != nil {
		return fmt.Errorf("unable to create pull request: %w", err)
	}
	logrus.Infof("Created PR #%d", pr.GetNumber())

	return nil
}

func modifyVersionFile(filePath, oldVersion, newVersion string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	modifiedContent := bytes.ReplaceAll(content, []byte(oldVersion), []byte(newVersion))

	err = os.WriteFile(filePath, modifiedContent, 0o644)
	if err != nil {
		return err
	}

	return nil
}
