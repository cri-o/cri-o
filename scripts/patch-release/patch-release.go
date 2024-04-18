package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/cri-o/cri-o/internal/version"
	goGit "github.com/go-git/go-git/v5"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-sdk/git"
	"sigs.k8s.io/release-sdk/github"
	"sigs.k8s.io/release-utils/env"
	"sigs.k8s.io/release-utils/util"
)

const (
	githubTokenEnvKey = "GITHUB_TOKEN"
	gitRemoteEnvKey   = "REMOTE"
	orgEnvKey         = "ORG"
	versionFile       = "internal/version/version.go"
	branchPrefix      = "release-"
	crioOrgRepo       = "cri-o"
)

var releaseMinorVersions = []string{"1.29", "1.28", "1.27"}

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	if err := run(); err != nil {
		logrus.Fatalf("Unable to %v", err)
	}
}

func run() error {
	if !env.IsSet(githubTokenEnvKey) {
		return fmt.Errorf(
			"run: $%s environment variable is not set", githubTokenEnvKey,
		)
	}

	remote := env.Default(gitRemoteEnvKey, "origin")
	logrus.Infof("Using repository fork remote: %s", remote)

	org := env.Default(orgEnvKey, crioOrgRepo)
	logrus.Infof("Using repository fork organization: %s", org)

	repo, err := git.OpenRepo(".")
	if err != nil {
		return fmt.Errorf("open local repo: %w", err)
	}

	for _, minorVersion := range releaseMinorVersions {
		baseBranchName := branchPrefix + minorVersion // returns "release-x.y"

		sv, err := getCurrentVersionFromReleaseBranch(repo, baseBranchName) // returns "x.y.z"
		if err != nil {
			return fmt.Errorf("current version from release branch %s to semver: %w", minorVersion, err)
		}

		// Bump up the patch version
		sv.Patch++

		if err := updateVersionAndCreatePR(
			repo, sv, baseBranchName, org, remote,
		); err != nil {
			return fmt.Errorf("update version in local repository: %w", err)
		}
	}

	return nil
}

func getCurrentVersionFromReleaseBranch(repo *git.Repo, baseBranchName string) (res semver.Version, err error) {
	logrus.Infof("Switching to branch: %s", baseBranchName)
	if err := repo.Checkout(baseBranchName); err != nil {
		return res, fmt.Errorf("checkout branch %s: %w", baseBranchName, err)
	}

	versionFromVersionFile, err := getCurrentVersionFromVersionFile(versionFile) // returns "x.xx.x"
	if err != nil {
		return res, fmt.Errorf("reading latest version: %w", err)
	}

	return convertStringToSemver(versionFromVersionFile)
}

func getCurrentVersionFromVersionFile(versionFile string) (string, error) {
	versionPattern := `const\s+Version\s+=\s+"(.+)"`

	content, err := os.ReadFile(versionFile)
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(versionPattern)
	matches := re.FindStringSubmatch(string(content))
	if len(matches) < 2 {
		return "", fmt.Errorf("unable to find version in %q", versionFile)
	}

	return matches[1], nil
}

func convertStringToSemver(tag string) (res semver.Version, err error) {
	sv, err := util.TagStringToSemver(strings.TrimSpace(tag))
	if err != nil {
		return res, fmt.Errorf("convert tag string %s to semver: %w", tag, err)
	}

	// clear any suffix like `-dev`
	sv.Pre = nil
	return sv, nil
}

func updateVersionAndCreatePR(
	repo *git.Repo, newVersion semver.Version, baseBranchName, org, remote string,
) error {
	logrus.Info("Updating repository")

	newBranch := branchPrefix + newVersion.String()
	doesTheBranchExistRemotely, err := repo.HasRemoteBranch(newBranch)
	if err != nil {
		return fmt.Errorf("remote has branch %s: %w", newBranch, err)
	}

	if doesTheBranchExistRemotely {
		// Only Rebase and force push
		if err := repo.Rebase(newBranch); err != nil {
			return fmt.Errorf("rebase branch %s: %w", newBranch, err)
		}
		opts := &goGit.PushOptions{
			Force: true,
		}
		if err := repo.PushToRemoteWithOptions(opts); err != nil {
			return fmt.Errorf("force pushing to remote: %s: %w", newBranch, err)
		}
		return nil
	}
	logrus.Infof("Switching to branch: %s", newBranch)
	if err := repo.Checkout("-B", newBranch); err != nil {
		return fmt.Errorf("checkout branch %s: %w", newBranch, err)
	}

	logrus.Infof("Updating new tag in file %s", versionFile)
	if err := modifyVersionFile(versionFile, version.Version, newVersion.String()); err != nil {
		return fmt.Errorf("update version file: %w", err)
	}

	logrus.Info("Committing changes")
	if err := repo.Add(versionFile); err != nil {
		return fmt.Errorf("add file %s to repo: %w", versionFile, err)
	}

	if err := repo.UserCommit(
		"version: bump to " + newVersion.String(),
	); err != nil {
		return fmt.Errorf("commit changes: %w", err)
	}

	logrus.Info("Pushing changes")
	if err := repo.PushToRemote(remote, newBranch); err != nil {
		return fmt.Errorf("pushing to remote: %s: %w", remote, err)
	}

	logrus.Info("Creating PR")
	gh := github.New()

	headBranchName := fmt.Sprintf("%s:%s", org, newBranch)
	title := fmt.Sprintf("Bump version for %s", newVersion)
	body := fmt.Sprintf(
		"Automated version bump to version `%s`\n\n%s",
		newVersion, "/release-note-none",
	)

	pr, err := gh.CreatePullRequest(crioOrgRepo, crioOrgRepo, baseBranchName,
		headBranchName,
		title,
		body,
	)
	if err != nil {
		return fmt.Errorf("create pull request: %w", err)
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
