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
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/github"
	"k8s.io/release/pkg/object"
	"k8s.io/release/pkg/tar"
)

// PrepareWorkspaceStage sets up the workspace by cloning a new copy of k/k.
func PrepareWorkspaceStage(directory string) error {
	logrus.Infof("Preparing workspace for staging in %s", directory)
	logrus.Infof("Cloning repository to %s", directory)
	repo, err := git.CloneOrOpenGitHubRepo(
		directory, git.DefaultGithubOrg, git.DefaultGithubRepo, false,
	)
	if err != nil {
		return errors.Wrap(err, "clone k/k repository")
	}

	token, ok := os.LookupEnv(github.TokenEnvKey)
	if !ok {
		return errors.Errorf("%s env variable is not set", github.TokenEnvKey)
	}

	if err := repo.SetURL(git.DefaultRemote, (&url.URL{
		Scheme: "https",
		User:   url.UserPassword("git", token),
		Host:   "github.com",
		Path:   filepath.Join(git.DefaultGithubOrg, git.DefaultGithubRepo),
	}).String()); err != nil {
		return errors.Wrap(err, "changing git remote of repository")
	}

	return nil
}

// PrepareWorkspaceRelease sets up the workspace by downloading and extracting
// the staged sources on the provided bucket.
func PrepareWorkspaceRelease(directory, buildVersion, bucket string) error {
	logrus.Infof("Preparing workspace for release in %s", directory)
	logrus.Infof("Searching for staged %s on %s", SourcesTar, bucket)
	tempDir, err := ioutil.TempDir("", "staged-")
	if err != nil {
		return errors.Wrap(err, "create staged sources temp dir")
	}
	defer os.RemoveAll(tempDir)

	// On `release`, we lookup the staged sources and use them directly
	src := filepath.Join(bucket, StagePath, buildVersion, SourcesTar)
	dst := filepath.Join(tempDir, SourcesTar)

	gcs := object.NewGCS()
	gcs.WithAllowMissing(false)
	if err := gcs.CopyToLocal(src, dst); err != nil {
		return errors.Wrap(err, "copying staged sources from GCS")
	}

	logrus.Info("Got staged sources, extracting archive")
	if err := tar.Extract(
		dst, strings.TrimSuffix(directory, "/src/k8s.io/kubernetes"),
	); err != nil {
		return errors.Wrapf(err, "extracting %s", dst)
	}

	return nil
}
