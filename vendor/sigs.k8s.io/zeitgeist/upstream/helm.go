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
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/blang/semver/v4"
	log "github.com/sirupsen/logrus"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

// Helm upstream representation
type Helm struct {
	Base `mapstructure:",squash"`

	// Helm repository URL, e.g. https://grafana.github.io/helm-charts
	Repo string

	// Helm chart name in this repository
	Chart string

	// Optional: semver constraints, e.g. < 2.0.0
	// Will have no effect if the dependency does not follow Semver
	Constraints string
}

// LatestVersion returns the latest non-draft, non-prerelease Helm Release
// for the given repository (depending on the Constraints if set).
func (upstream Helm) LatestVersion() (string, error) {
	log.Debug("Using Helm flavour")
	return latestChartVersion(upstream)
}

func latestChartVersion(upstream Helm) (string, error) {
	// Sanity checking
	if upstream.Repo == "" {
		return "", errors.New("invalid helm upstream: missing repo argument")
	}

	if upstream.Chart == "" {
		return "", errors.New("invalid helm upstream: missing chart argument")
	}
	parsedRepo, err := url.Parse(upstream.Repo)
	if err != nil {
		return "", fmt.Errorf("invalid helm repo url: %s: %w", upstream.Repo, err)
	}
	s := parsedRepo.Scheme
	if s != "http" && s != "https" && s != "oci" {
		// We currently only support http-based and oci repos (Helm defaults)
		// Helm allows custom handlers via plugins, but I've never seen it in practice - could be added later if needed
		return "", fmt.Errorf("invalid helm repo: %s, only http, https and oci are supported", upstream.Repo)
	}

	var useSemverConstraints bool
	var expectedRange semver.Range
	semverConstraints := upstream.Constraints
	if semverConstraints == "" {
		useSemverConstraints = false
	} else {
		useSemverConstraints = true
		validatedExpectedRange, err := semver.ParseRange(semverConstraints)
		if err != nil {
			return "", fmt.Errorf("invalid semver constraints range: %#v: %w", upstream.Constraints, err)
		}
		expectedRange = validatedExpectedRange
	}

	// First, get the repo index
	// Helm expects a cache directory, so we create a temporary one
	cacheDir, err := os.MkdirTemp("", "zeitgeist-helm-cache")
	if err != nil {
		log.Errorf("failed to create temporary directory for Helm cache")
		return "", err
	}
	defer os.RemoveAll(cacheDir)

	cfg := repo.Entry{
		Name: "zeitgeist",
		URL:  upstream.Repo,
	}
	settings := cli.EnvSettings{
		PluginsDirectory: "",
		RepositoryCache:  cacheDir,
	}
	re, err := repo.NewChartRepository(&cfg, getter.All(&settings))
	if err != nil {
		log.Errorf("failed to instantiate the Helm Chart Repository")
		return "", err
	}

	log.Debugf("Downloading repo index for %s...", upstream.Repo)
	indexFile, err := re.DownloadIndexFile()
	if err != nil {
		log.Errorf("failed to download index file for repo %s", upstream.Repo)
		return "", err
	}

	log.Debugf("Loading repo index for %s...", upstream.Repo)
	index, err := repo.LoadIndexFile(indexFile)
	if err != nil {
		log.Errorf("failed to load index file for repo %s", upstream.Repo)
		return "", err
	}

	chartVersions := index.Entries[upstream.Chart]
	if chartVersions == nil {
		return "", fmt.Errorf("no chart for %s found in repository %s", upstream.Chart, upstream.Repo)
	}

	// Iterate over versions and get the first newer version
	// (Or the first version that matches our semver constraints, if defined)
	// Versions are already ordered, cf https://github.com/helm/helm/blob/6a3daaa7aa5b89a150042cadcbe869b477bb62a1/pkg/repo/index.go#L344
	for _, chartVersion := range chartVersions {
		chartVersionStr := strings.TrimPrefix(chartVersion.Version, "v")

		prerelease, err := strconv.ParseBool(chartVersion.Annotations["artifacthub.io/prerelease"])
		if err == nil && prerelease {
			log.Debugf("Skipping annotated prerelease: %s\n", chartVersionStr)
			continue
		}

		version, err := semver.Parse(chartVersionStr)
		if err != nil { //nolint:gocritic
			log.Debugf("Error parsing version %s (%#v) as semver, cannot validate semver constraints", chartVersionStr, err)
		} else if len(version.Pre) > 0 {
			log.Debugf("Skipping semver prerelease: %s\n", chartVersionStr)
			continue
		} else if useSemverConstraints && !expectedRange(version) {
			log.Debugf("Skipping release not matching range constraints (%s): %s\n", upstream.Constraints, chartVersionStr)
			continue
		}

		log.Debugf("Found latest matching release: %s\n", chartVersionStr)

		return chartVersionStr, nil
	}

	// No latest version found – no versions? Only prereleases?
	return "", errors.New("no potential version found")
}
