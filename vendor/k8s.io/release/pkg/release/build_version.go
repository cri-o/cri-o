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
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/testgrid"
)

func SetBuildVersion(
	branch string,
) error {
	logrus.Infof("Setting build version for branch %q", branch)

	if branch == git.Master {
		branch = "release-master"
		logrus.Infof("Changing %s branch to %q", git.Master, branch)
	}

	allJobs, err := testgrid.New().BlockingTests(branch)
	if err != nil {
		return errors.Wrap(err, "getting all test jobs")
	}
	logrus.Infof("Got testgrid jobs for branch %q: %v", branch, allJobs)

	// TODO: continue port from releaselib.sh::set_build_version

	return errors.New("unimplemented")
}
