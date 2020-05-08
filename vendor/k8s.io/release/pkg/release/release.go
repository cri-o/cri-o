/*
Copyright 2019 The Kubernetes Authors.

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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/http"
	"k8s.io/release/pkg/util"
)

const (
	// gcbmgr/anago defaults
	DefaultToolRepo   = "release"
	DefaultToolBranch = git.Master
	DefaultToolOrg    = git.DefaultGithubOrg
	// TODO(vdf): Need to reference K8s Infra project here
	DefaultKubernetesStagingProject = "kubernetes-release-test"
	DefaultRelengStagingProject     = "k8s-staging-releng"
	DefaultDiskSize                 = "300"
	BucketPrefix                    = "kubernetes-release-"

	versionReleaseRE  = `v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[a-zA-Z0-9]+)*\.*(0|[1-9][0-9]*)?`
	versionBuildRE    = `([0-9]{1,})\+([0-9a-f]{5,40})`
	versionDirtyRE    = `(-dirty)`
	dockerBuildPath   = "_output/release-tars"
	bazelBuildPath    = "bazel-bin/build/release-tars"
	bazelVersionPath  = "bazel-bin/version"
	dockerVersionPath = "kubernetes/version"
	kubernetesTar     = "kubernetes.tar.gz"

	// GCSStagePath is the directory where release artifacts are staged before
	// push to GCS.
	GCSStagePath = "gcs-stage"

	// ReleaseStagePath is the directory where releases are staged.
	ReleaseStagePath = "release-stage"

	// GCEPath is the directory where GCE scripts are created.
	GCEPath = "release-stage/full/kubernetes/cluster/gce"

	// GCIPath is the path for the container optimized OS for GCP.
	GCIPath = "release-stage/full/kubernetes/cluster/gce/gci"

	// ReleaseTarsPath is the directory where release artifacts are created.
	ReleaseTarsPath = "release-tars"

	// WindowsLocalPath is the directory where Windows GCE scripts are created.
	WindowsLocalPath = "release-stage/full/kubernetes/cluster/gce/windows"

	// WindowsGCSPath is the directory where Windoes GCE scripts are staged
	// before push to GCS.
	WindowsGCSPath = "gcs-stage/extra/gce/windows"

	// ProductionBucket is the default bucket for Kubernetes releases
	ProductionBucket = "kubernetes-release"

	// ProductionBucketURL is the url for the ProductionBucket
	ProductionBucketURL = "https://dl.k8s.io"
)

// GetDefaultKubernetesRepoURL returns the default HTTPS repo URL for Release Engineering tools.
// Expected: https://github.com/kubernetes/release
func GetDefaultToolRepoURL() string {
	return GetToolRepoURL(DefaultToolOrg, DefaultToolRepo, false)
}

// GetKubernetesRepoURL takes a GitHub org and repo, and useSSH as a boolean and
// returns a repo URL for Release Engineering tools.
// Expected result is one of the following:
// - https://github.com/<org>/release
// - git@github.com:<org>/release
func GetToolRepoURL(org, repo string, useSSH bool) string {
	if org == "" {
		org = GetToolOrg()
	}
	if repo == "" {
		repo = GetToolRepo()
	}

	return git.GetRepoURL(org, repo, useSSH)
}

// GetToolOrg checks if the 'TOOL_ORG' environment variable is set.
// If 'TOOL_ORG' is non-empty, it returns the value. Otherwise, it returns DefaultToolOrg.
func GetToolOrg() string {
	return util.EnvDefault("TOOL_ORG", DefaultToolOrg)
}

// GetToolRepo checks if the 'TOOL_REPO' environment variable is set.
// If 'TOOL_REPO' is non-empty, it returns the value. Otherwise, it returns DefaultToolRepo.
func GetToolRepo() string {
	return util.EnvDefault("TOOL_REPO", DefaultToolRepo)
}

// GetToolBranch checks if the 'TOOL_BRANCH' environment variable is set.
// If 'TOOL_BRANCH' is non-empty, it returns the value. Otherwise, it returns DefaultToolBranch.
func GetToolBranch() string {
	return util.EnvDefault("TOOL_BRANCH", DefaultToolBranch)
}

// BuiltWithBazel determines whether the most recent Kubernetes release was built with Bazel.
func BuiltWithBazel(workDir string) (bool, error) {
	bazelBuild := filepath.Join(workDir, bazelBuildPath, kubernetesTar)
	dockerBuild := filepath.Join(workDir, dockerBuildPath, kubernetesTar)
	return util.MoreRecent(bazelBuild, dockerBuild)
}

// ReadBazelVersion reads the version from a Bazel build.
func ReadBazelVersion(workDir string) (string, error) {
	version, err := ioutil.ReadFile(filepath.Join(workDir, bazelVersionPath))
	if os.IsNotExist(err) {
		// The check for version in bazel-genfiles can be removed once everyone is
		// off of versions before 0.25.0.
		// https://github.com/bazelbuild/bazel/issues/8651
		version, err = ioutil.ReadFile(filepath.Join(workDir, "bazel-genfiles/version"))
	}
	return string(version), err
}

// ReadDockerizedVersion reads the version from a Dockerized Kubernetes build.
func ReadDockerizedVersion(workDir string) (string, error) {
	dockerTarball := filepath.Join(workDir, dockerBuildPath, kubernetesTar)
	reader, err := util.ReadFileFromGzippedTar(dockerTarball, dockerVersionPath)
	if err != nil {
		return "", err
	}
	file, err := ioutil.ReadAll(reader)
	return strings.TrimSpace(string(file)), err
}

// IsValidReleaseBuild checks if build version is valid for release.
func IsValidReleaseBuild(build string) (bool, error) {
	return regexp.MatchString("("+versionReleaseRE+`(\.`+versionBuildRE+")?"+versionDirtyRE+"?)", build)
}

// IsDirtyBuild checks if build version is dirty.
func IsDirtyBuild(build string) bool {
	return strings.Contains(build, "dirty")
}

// GetKubecrossVersion returns the current kube-cross container version.
// Replaces release::kubecross_version
func GetKubecrossVersion(branches ...string) (string, error) {
	for i, branch := range branches {
		logrus.Infof("Trying to get the kube-cross version for %s...", branch)

		versionURL := fmt.Sprintf("https://raw.githubusercontent.com/kubernetes/kubernetes/%s/build/build-image/cross/VERSION", branch)

		version, httpErr := http.GetURLResponse(versionURL, true)
		if httpErr != nil {
			if i < len(branches)-1 {
				logrus.Infof("Error retrieving the kube-cross version for the '%s': %v", branch, httpErr)
			} else {
				return "", httpErr
			}
		}

		if version != "" {
			logrus.Infof("Found the following kube-cross version: %s", version)
			return version, nil
		}
	}

	return "", errors.New("kube-cross version should not be empty; cannot continue")
}

// URLPrefixForBucket returns the URL prefix for the provided bucket string
func URLPrefixForBucket(bucket string) string {
	urlPrefix := fmt.Sprintf("https://storage.googleapis.com/%s/release", bucket)
	if bucket == ProductionBucket {
		urlPrefix = ProductionBucketURL
	}
	return urlPrefix
}
