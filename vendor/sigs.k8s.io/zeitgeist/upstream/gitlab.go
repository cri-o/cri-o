/*
Copyright 2021 The Kubernetes Authors.

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

package upstream

import (
	"errors"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/zeitgeist/pkg/gitlab"
)

// GitLab upstream representation
type GitLab struct {
	Base `mapstructure:",squash"`

	// GitLab Server if is a self-hosted GitLab instead, default to gitlab.com
	Server string

	// GitLab URL, e.g. hashicorp/terraform or helm/helm
	URL string

	// Optional: semver constraints, e.g. < 2.0.0
	// Will have no effect if the dependency does not follow Semver
	Constraints string

	// If branch is specified, the version should be a commit SHA
	// Will look for new commits on the branch
	Branch string
}

// LatestVersion returns the latest non-draft, non-prerelease GitLab Release
// for the given repository (depending on the Constraints if set).
//
// To authenticate your requests, use the GITLAB_TOKEN environment variable.
func (upstream GitLab) LatestVersion() (string, error) { // nolint:gocritic
	log.Debug("Using GitLab flavour")
	return latestGitLabVersion(&upstream)
}

func latestGitLabVersion(upstream *GitLab) (string, error) {
	if upstream.Branch == "" {
		return latestGitLabRelease(upstream)
	}
	return latestGitlabCommit(upstream)
}

func latestGitLabRelease(upstream *GitLab) (string, error) {
	var client *gitlab.GitLab
	if upstream.Server == "" {
		client = gitlab.New()
	} else {
		client = gitlab.NewPrivate(upstream.Server)
	}
	if client == nil {
		return "", errors.New(
			"cannot configure a GitLab client, make sure you have exported the GITLAB_TOKEN",
		)
	}

	if !strings.Contains(upstream.URL, "/") {
		return "", fmt.Errorf(
			"invalid gitlab repo: %s\nGitLab repo should be in the form owner/repo e.g., kubernetes/kubernetes",
			upstream.URL,
		)
	}

	semverConstraints := upstream.Constraints
	if semverConstraints == "" {
		// If no range is passed, just use the broadest possible range
		semverConstraints = DefaultSemVerConstraints
	}

	expectedRange, err := semver.ParseRange(semverConstraints)
	if err != nil {
		return "", fmt.Errorf("invalid semver constraints range: %#v: %w", upstream.Constraints, err)
	}

	splitURL := strings.Split(upstream.URL, "/")
	owner := splitURL[0]
	repo := strings.Join(splitURL[1:], "/")

	var tags []string
	// We'll need to fetch all releases, as GitLab doesn't provide sorting options.
	// If we don't do that, we risk running into the case where for example:
	// - Version 1.0.0 and 2.0.0 exist
	// - A bugfix 1.0.1 gets released
	//
	// Now the "latest" (date-wise) release is not the highest semver, and not necessarily the one we want
	log.Debugf("Retrieving releases for %s/%s...", owner, repo)
	releases, err := client.Releases(owner, repo)
	if err != nil {
		return "", fmt.Errorf("retrieving GitLab releases: %w", err)
	}

	if len(releases) == 0 {
		gitLabTags, err := client.ListTags(owner, repo)
		if err != nil {
			return "", fmt.Errorf("retrieving GitLab tags: %w", err)
		}

		for _, tag := range gitLabTags {
			tags = append(tags, tag.Name)
		}
	} else {
		for _, release := range releases {
			if release.TagName == "" {
				log.Debug("Skipping release without TagName")
			}

			tags = append(tags, release.TagName)
		}
	}

	for _, tag := range tags {
		// Try to match semver and range
		version, err := semver.Parse(strings.Trim(tag, "v"))
		if err != nil {
			log.Debugf("Error parsing version %s (%#v) as semver, cannot validate semver constraints", tag, err)
		} else if !expectedRange(version) {
			log.Debugf("Skipping release not matching range constraints (%s): %s\n", upstream.Constraints, tag)
			continue
		}

		log.Debugf("Found latest matching release: %s\n", version)

		return version.String(), nil
	}

	// No latest version found – no versions? Only prereleases?
	return "", errors.New("no potential version found")
}

func latestGitlabCommit(upstream *GitLab) (string, error) {
	var client *gitlab.GitLab
	if upstream.Server == "" {
		client = gitlab.New()
	} else {
		client = gitlab.NewPrivate(upstream.Server)
	}
	if client == nil {
		return "", errors.New(
			"cannot configure a GitLab client, make sure you have exported the GITLAB_TOKEN",
		)
	}

	splitURL := strings.Split(upstream.URL, "/")
	owner := splitURL[0]
	repo := strings.Join(splitURL[1:], "/")

	log.Debugf("Retrieving repository information for %s/%s...", owner, repo)
	repoInfo, err := client.GetRepository(owner, repo)
	if err != nil {
		return "", fmt.Errorf("retrieving GitLab repository: %w", err)
	}

	if repoInfo.Archived {
		log.Warnf("GitLab repository %s/%s is archived", owner, repo)
	}

	branches, err := client.Branches(owner, repo)
	if err != nil {
		return "", fmt.Errorf("retrieving GitLab branches: %w", err)
	}
	for _, branch := range branches {
		if branch.Name == upstream.Branch {
			return branch.Commit.ID, nil
		}
	}
	return "", fmt.Errorf("branch '%s' not found", upstream.Branch)
}
