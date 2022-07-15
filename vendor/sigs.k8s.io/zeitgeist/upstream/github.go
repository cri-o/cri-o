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

package upstream

import (
	"errors"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/release-sdk/github"
)

// Github upstream representation
type Github struct {
	Base `mapstructure:",squash"`

	// Github URL, e.g. hashicorp/terraform or helm/helm
	URL string

	// Optional: semver constraints, e.g. < 2.0.0
	// Will have no effect if the dependency does not follow Semver
	Constraints string

	// If branch is specified, the version should be a commit SHA
	// Will look for new commits on the branch
	Branch string
}

// LatestVersion returns the latest non-draft, non-prerelease Github Release
// for the given repository (depending on the Constraints if set).
//
// Authentication
//
// The Github API allows unauthenticated requests, but the API limits are very
// strict: https://developer.github.com/v3/#rate-limiting
//
// To authenticate your requests, use the GITHUB_ACCESS_TOKEN environment variable.
func (upstream Github) LatestVersion() (string, error) {
	log.Debug("Using GitHub flavour")
	return latestVersion(upstream)
}

func latestVersion(upstream Github) (string, error) {
	if upstream.Branch == "" {
		return latestRelease(upstream)
	}
	return latestCommit(upstream)
}

func latestRelease(upstream Github) (string, error) {
	client := github.New()

	if !strings.Contains(upstream.URL, "/") {
		return "", fmt.Errorf(
			"invalid github repo: %s\nGithub repo should be in the form owner/repo e.g., kubernetes/kubernetes",
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
	repo := splitURL[1]

	log.Debugf("Retrieving repository information for %s/%s...", owner, repo)
	repoInfo, err := client.GetRepository(owner, repo)
	if err != nil {
		return "", fmt.Errorf("retrieving GitHub repository: %w", err)
	}

	if repoInfo.GetArchived() {
		log.Warnf("GitHub repository %s/%s is archived", owner, repo)
	}

	var tags []string
	// We'll need to fetch all releases, as Github doesn't provide sorting options.
	// If we don't do that, we risk running into the case where for example:
	// - Version 1.0.0 and 2.0.0 exist
	// - A bugfix 1.0.1 gets released
	//
	// Now the "latest" (date-wise) release is not the highest semver, and not necessarily the one we want
	log.Debugf("Retrieving releases for %s/%s...", owner, repo)
	releases, err := client.Releases(owner, repo, false)
	if err != nil {
		return "", fmt.Errorf("retrieving GitHub releases: %w", err)
	}

	// if there is no releases we will try to get the tags, the project might just use tags to release.
	if len(releases) == 0 {
		gitHubTags, err := client.ListTags(owner, repo)
		if err != nil {
			return "", fmt.Errorf("retrieving GitHub tags: %w", err)
		}

		for _, tag := range gitHubTags {
			tags = append(tags, tag.GetName())
		}
	} else {
		for _, release := range releases {
			if release.TagName == nil {
				log.Debug("Skipping release without TagName")
			}

			if release.Draft != nil && *release.Draft {
				log.Debugf("Skipping draft release: %s\n", release.GetTagName())
				continue
			}

			tags = append(tags, release.GetTagName())
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

		log.Debugf("Found latest matching release: %s\n", version.String())
		return version.String(), nil
	}

	// No latest version found – no versions? Only prereleases?
	return "", errors.New("no potential version found")
}

func latestCommit(upstream Github) (string, error) {
	client := github.New()

	if !strings.Contains(upstream.URL, "/") {
		return "", fmt.Errorf(
			"invalid github repo: %s\nGithub repo should be in the form owner/repo e.g., kubernetes/kubernetes",
			upstream.URL,
		)
	}

	splitURL := strings.Split(upstream.URL, "/")
	owner := splitURL[0]
	repo := splitURL[1]

	branches, err := client.ListBranches(owner, repo)
	if err != nil {
		return "", fmt.Errorf("retrieving GitHub branches: %w", err)
	}
	for _, branch := range branches {
		if branch.GetName() == upstream.Branch {
			return *branch.GetCommit().SHA, nil
		}
	}
	return "", fmt.Errorf("branch '%s' not found", upstream.Branch)
}
