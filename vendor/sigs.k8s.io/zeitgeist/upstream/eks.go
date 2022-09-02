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
	"io"
	"net/http"
	"regexp"

	"github.com/blang/semver/v4"
	log "github.com/sirupsen/logrus"
)

// EKS is the Elastic Kubernetes Service upstream
//
// See: https://docs.aws.amazon.com/eks/index.html
type EKS struct {
	Base `mapstructure:",squash"`

	// Optional: semver constraints, e.g. < 1.16.0
	Constraints string
}

// LatestVersion returns the latest available EKS version.
//
// Retrieves all available EKS versions from the parsing HTML from AWS's documentation page
// This feels brittle and wrong, but AFAIK there is no better way to do this
func (upstream EKS) LatestVersion() (string, error) {
	log.Debug("Using EKS upstream")

	semverConstraints := upstream.Constraints
	if semverConstraints == "" {
		// If no range is passed, just use the broadest possible range
		semverConstraints = ">= 0.0.0"
	}

	expectedRange, err := semver.ParseRange(semverConstraints)
	if err != nil {
		return "", fmt.Errorf("invalid semver constraints range: %v: %w", upstream.Constraints, err)
	}

	const docsURL = "https://docs.aws.amazon.com/eks/latest/userguide/kubernetes-versions.html"
	log.Debugf("Retrieving EKS releases from  %s...", docsURL)

	resp, err := http.Get(docsURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// fmt.Println(string(body))
	// Versions are listed as semver within a `<p><code class="code">` tag
	r := regexp.MustCompile(`<p><code class="code">(\d+.\d+.\d+)</code></p>`)
	matches := r.FindAllSubmatch(body, -1)
	for _, match := range matches {
		versionString := string(match[1])
		version, err := semver.Parse(versionString)
		if err != nil {
			log.Debugf("Error parsing version %v (%v) as semver, cannot validate semver constraints", versionString, err)
		} else if !expectedRange(version) {
			log.Debugf("Skipping version not matching range constraints (%v): %v", upstream.Constraints, versionString)
			continue
		}

		log.Debugf("Found latest matching release: %v", version)

		return version.String(), nil
	}

	return "", errors.New("no matching EKS version found")
}
