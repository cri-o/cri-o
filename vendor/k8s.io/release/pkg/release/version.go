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

package release

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/http"
)

// VersionType is a simple wrapper around a Kubernetes release version
type VersionType string

const (
	// VersionStable references the latest stable Kubernetes version,
	// for example `v1.17.3`
	VersionTypeStable VersionType = "release/stable"

	// VersionStablePreRelease references the latest stable pre release
	// Kubernetes version, for example `v1.19.0-alpha.0`
	VersionTypeStablePreRelease VersionType = "release/latest"

	// VersionStable references the latest CI Kubernetes version,
	// for example `v1.19.0-alpha.0.721+f8ff8f44206ff4`
	VersionTypeCILatest VersionType = "ci/latest"

	// baseURL is the base URL for every release version retrieval
	baseURL = "https://dl.k8s.io/"
)

// url retrieves the full URL of the Kubernetes release version
func (t VersionType) url(version string) string {
	url := baseURL + string(t)

	if version != "" {
		url += "-" + version
	}
	url += ".txt"

	return url
}

// GetKubeVersion retrieves the version of the provided Kubernetes version type
func GetKubeVersion(versionType VersionType) (string, error) {
	logrus.Infof("Retrieving Kubernetes release version for %s", versionType)
	return kubeVersionFromURL(versionType.url(""))
}

// GetKubeVersionForBranch returns the remote Kubernetes release version for
// the provided branch
func GetKubeVersionForBranch(versionType VersionType, branch string) (string, error) {
	logrus.Infof(
		"Retrieving Kubernetes release version for %s on branch %s",
		versionType, branch,
	)

	version := ""
	if branch != git.Master {
		if !git.IsReleaseBranch(branch) {
			return "", errors.Errorf("%s is not a valid release branch", branch)
		}
		version = strings.TrimPrefix(branch, "release-")
	}
	url := versionType.url(version)

	return kubeVersionFromURL(url)
}

// kubeVersionFromURL retrieves the Kubernetes version from the provided URL
// ans strips the tag prefix if `stripTagPrefix` is `true`
func kubeVersionFromURL(url string) (string, error) {
	logrus.Infof("Retrieving Kubernetes build version from %s...", url)
	version, httpErr := http.GetURLResponse(url, true)
	if httpErr != nil {
		return "", errors.Wrap(httpErr, "retrieving kube version")
	}

	logrus.Infof("Retrieved Kubernetes version: %s", version)
	return version, nil
}
