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
	"sort"

	"github.com/blang/semver/v4"
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/zeitgeist/pkg/container"
)

// Container upstream representation
type Container struct {
	Base `mapstructure:",squash"`
	// Registry URL, e.g. gcr.io/k8s-staging-kubernetes/conformance
	Registry string
	// Optional: semver constraints, e.g. < 2.0.0
	// Will have no effect if the dependency does not follow Semver
	Constraints string
}

// LatestVersion returns the latest tag for the given repository
// (depending on the Constraints if set).
func (upstream Container) LatestVersion() (string, error) { // nolint:gocritic
	log.Debugf("Using Container flavour")
	return highestSemanticImageTag(&upstream)
}

func highestSemanticImageTag(upstream *Container) (string, error) {
	client := container.New()

	semverConstraints := upstream.Constraints
	if semverConstraints == "" {
		// If no range is passed, just use the broadest possible range
		semverConstraints = DefaultSemVerConstraints
	}
	expectedRange, err := semver.ParseRange(semverConstraints)
	if err != nil {
		return "", fmt.Errorf("invalid semver constraints range: %v: %w", upstream.Constraints, err)
	}

	log.Debugf("Retrieving tags for %s...", upstream.Registry)
	tags, err := client.ListTags(upstream.Registry)
	if err != nil {
		return "", fmt.Errorf("retrieving Container tags: %w", err)
	}
	log.Debugf("Found %d tags for %s...", len(tags), upstream.Registry)

	// parse semvers first so we can safely sort
	type semverWithOrig struct {
		orig   string         // original tag string
		parsed semver.Version // parsed semver
	}
	versions := make([]semverWithOrig, 0, len(tags))
	for _, tag := range tags {
		parsed, err := semver.ParseTolerant(tag)
		if err != nil {
			log.Debugf("Error parsing version %s (%v) as semver", tag, err)
			continue
		}
		versions = append(versions, semverWithOrig{
			orig:   tag,
			parsed: parsed,
		})
	}
	// reverse sort, highest first
	sort.Slice(versions, func(i, j int) bool {
		return versions[j].parsed.LT(versions[i].parsed)
	})

	// find first version matching constraints
	for _, version := range versions {
		if !expectedRange(version.parsed) {
			log.Debugf("Skipping release not matching range constraints (%s): %s", upstream.Constraints, version.parsed.String())
			continue
		}
		log.Debugf("Found latest matching tag: %s", version.orig)
		return version.orig, nil
	}

	return "", errors.New("no potential tag found")
}
