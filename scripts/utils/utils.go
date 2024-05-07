package utils

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-sdk/git"
	"sigs.k8s.io/release-utils/util"
)

const (
	GithubTokenEnvKey = "GITHUB_TOKEN"
	GitRemoteEnvKey   = "REMOTE"
	OrgEnvKey         = "ORG"
	VersionFile       = "internal/version/version.go"
	BranchPrefix      = "release-"
	VersionPrefix     = "v"
	CrioOrgRepo       = "cri-o"
)

func GetCurrentVersionFromReleaseBranch(repo *git.Repo, baseBranchName string) (res semver.Version, err error) {
	logrus.Infof("Switching to branch: %s", baseBranchName)
	if err := repo.Checkout(baseBranchName); err != nil {
		return res, fmt.Errorf("unable to checkout branch %s: %w", baseBranchName, err)
	}

	versionFromVersionFile, err := GetCurrentVersionFromVersionFile(VersionFile) // returns "x.xx.x"
	if err != nil {
		return res, fmt.Errorf("unable to read latest version: %w", err)
	}

	logrus.Infof("Using version: %s", versionFromVersionFile)
	return ConvertStringToSemver(versionFromVersionFile)
}

func ConvertStringToSemver(tag string) (res semver.Version, err error) {
	sv, err := util.TagStringToSemver(strings.TrimSpace(tag))
	if err != nil {
		return res, fmt.Errorf("unable to convert tag %q to semver: %w", tag, err)
	}

	// Clear any pre-release and development suffixes.
	sv.Pre = nil
	return sv, nil
}

func GetCurrentVersionFromVersionFile(versionFile string) (string, error) {
	const versionPattern = `const\s+Version\s+=\s+"(.+)"`

	content, err := os.ReadFile(versionFile)
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(versionPattern)
	matches := re.FindStringSubmatch(string(content))
	if len(matches) < 2 {
		return "", fmt.Errorf("unable to find current release version using file: %s", versionFile)
	}

	return matches[1], nil
}
