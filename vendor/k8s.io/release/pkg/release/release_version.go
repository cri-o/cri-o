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
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/release/regex"
)

const (
	ReleaseTypeOfficial string = "official"
	ReleaseTypeRC       string = "rc"
	ReleaseTypeBeta     string = "beta"
	ReleaseTypeAlpha    string = "alpha"
)

// Versions specifies the collection of found release versions
type Versions struct {
	prime    string
	official string
	rc       string
	beta     string
	alpha    string
}

// Prime can be used to get the most prominent release version
func (r *Versions) Prime() string {
	return r.prime
}

// Official can be used to get the ReleaseTypeOfficial from the versions
func (r *Versions) Official() string {
	return r.official
}

// Rc can be used to get the ReleaseTypeRC from the versions
func (r *Versions) RC() string {
	return r.rc
}

// Beta can be used to get the ReleaseTypeBeta from the versions
func (r *Versions) Beta() string {
	return r.beta
}

// Alpha can be used to get the ReleaseTypeAlpha from the versions
func (r *Versions) Alpha() string {
	return r.alpha
}

// String returns a string representation for the release versions
func (r *Versions) String() string {
	sb := &strings.Builder{}
	fmt.Fprintf(sb, "prime: %s, ", r.prime)
	if r.official != "" {
		fmt.Fprintf(sb, "%s: %s, ", ReleaseTypeOfficial, r.official)
	}
	if r.rc != "" {
		fmt.Fprintf(sb, "%s: %s, ", ReleaseTypeRC, r.rc)
	}
	if r.beta != "" {
		fmt.Fprintf(sb, "%s: %s, ", ReleaseTypeBeta, r.beta)
	}
	if r.alpha != "" {
		fmt.Fprintf(sb, "%s: %s", ReleaseTypeAlpha, r.alpha)
	}
	return sb.String()
}

// Slice can be used when iterating over the release versions in the order
// official, rc, beta, alpha
func (r *Versions) Slice() []string {
	res := []string{}
	if r.official != "" {
		res = append(res, r.official)
	}
	if r.rc != "" {
		res = append(res, r.rc)
	}
	if r.beta != "" {
		res = append(res, r.beta)
	}
	if r.alpha != "" {
		res = append(res, r.alpha)
	}
	return res
}

// SetReleaseVersion returns the release versions for the provided parameters
func SetReleaseVersion(
	releaseType, version, branch, parentBranch string,
) (*Versions, error) {
	logrus.Infof(
		"Setting release version for %s (branch: %s, parent branch: %s)",
		version, branch, parentBranch,
	)

	branchMatch := regex.BranchRegex.FindStringSubmatch(branch)
	if len(branchMatch) < 3 {
		return nil, errors.Errorf("invalid formatted branch %s", branch)
	}
	branchMajor, err := strconv.Atoi(branchMatch[1])
	if err != nil {
		return nil, errors.Wrap(err, "parsing branch major version to int")
	}
	branchMinor, err := strconv.Atoi(branchMatch[2])
	if err != nil {
		return nil, errors.Wrap(err, "parsing branch minor version to int")
	}
	releaseBranch := struct{ major, minor int }{
		major: branchMajor, minor: branchMinor,
	}

	// if branch == master, version is an alpha or beta
	// if branch == release, version is a rc
	// if branch == release+1, version is an alpha
	versionMatch := regex.ReleaseRegex.FindStringSubmatch(version)
	if len(versionMatch) < 5 {
		return nil, errors.Errorf("invalid formatted version %s", version)
	}
	buildMajor, err := strconv.Atoi(versionMatch[1])
	if err != nil {
		return nil, errors.Wrap(err, "parsing build major version to int")
	}
	buildMinor, err := strconv.Atoi(versionMatch[2])
	if err != nil {
		return nil, errors.Wrap(err, "parsing build minor version to int")
	}
	buildPatch, err := strconv.Atoi(versionMatch[3])
	if err != nil {
		return nil, errors.Wrap(err, "parsing build patch version to int")
	}
	var labelID *int
	if versionMatch[5] != "" {
		parsedLabelID, err := strconv.Atoi(versionMatch[5])
		if err != nil {
			return nil, errors.Wrap(err, "parsing build label ID to int")
		}
		labelID = &parsedLabelID
	}
	buildVersion := struct {
		major, minor, patch int
		labelID             *int
		label               string
	}{
		major:   buildMajor,
		minor:   buildMinor,
		patch:   buildPatch,
		label:   versionMatch[4], // -alpha, -beta, -rc
		labelID: labelID,
	}

	// releaseVersions.prime is the default release version for this
	// session/type Other labels such as alpha, beta, and rc are set as needed
	// Index ordering is important here as it's how they are processed
	releaseVersions := &Versions{}
	if parentBranch == git.Master {
		// This is a new branch, set new alpha and RC versions
		releaseVersions.alpha = fmt.Sprintf("v%d.%d.0-alpha.0",
			releaseBranch.major, releaseBranch.minor+1)
		releaseVersions.rc = fmt.Sprintf("v%d.%d.0-rc.0",
			releaseBranch.major, releaseBranch.minor)
		releaseVersions.prime = releaseVersions.rc
	} else if strings.HasPrefix(branch, "release-") {
		// Build directly from releaseVersions
		releaseVersions.prime = fmt.Sprintf("v%d.%d",
			buildVersion.major, buildVersion.minor)

		// If the incoming version is anything bigger than vX.Y.Z, then it's a
		// Jenkin's build version and it stands as is, otherwise increment the
		// patch
		patch := buildVersion.patch
		if buildVersion.labelID == nil {
			patch++
		}
		releaseVersions.prime += fmt.Sprintf(".%d", patch)

		labelID := 1
		if buildVersion.labelID != nil {
			labelID = *buildVersion.labelID + 1
		}

		if releaseType == ReleaseTypeOfficial {
			releaseVersions.official = releaseVersions.prime
			// Only primary branches get rc releases
			if regexp.MustCompile(`^release-([0-9]{1,})\.([0-9]{1,})$`).MatchString(branch) {
				releaseVersions.rc = fmt.Sprintf(
					"v%d.%d.%d-rc.0",
					buildVersion.major,
					buildVersion.minor,
					buildVersion.patch+1,
				)
			}
		} else if releaseType == ReleaseTypeRC {
			releaseVersions.rc = fmt.Sprintf(
				"%s-rc.%d", releaseVersions.prime, labelID,
			)
			releaseVersions.prime = releaseVersions.rc
		} else if releaseType == ReleaseTypeBeta {
			releaseVersions.beta = fmt.Sprintf(
				"v%d.%d.%d",
				buildVersion.major,
				buildVersion.minor,
				buildVersion.patch,
			)

			// Enable building beta releases on the master branch.
			// If the last build version was an alpha (x.y.z-alpha.N), set the
			// build
			// label to 'beta' and release version to x.y.z-beta.0.
			//
			// Otherwise, we'll assume this is the next x.y beta, so just
			// increment the
			// beta version e.g., x.y.z-beta.1 --> x.y.z-beta.2
			if buildVersion.label == "-alpha" {
				buildVersion.label = "-beta"
				releaseVersions.beta += fmt.Sprintf("%s.0", buildVersion.label)
			} else {
				releaseVersions.beta += fmt.Sprintf(
					"%s.%d", buildVersion.label, labelID,
				)
			}

			releaseVersions.prime = releaseVersions.beta
		} else {
			// In this code branch, we're implicitly supposed to be at an alpha
			// release. Here, we verify that we're not attempting to cut an
			// alpha release after a beta in the x.y release series.
			//
			// Concretely:
			// We should not be able to cut x.y.z-alpha.N after x.y.z-beta.M
			if buildVersion.label != "-alpha" {
				return nil, errors.Errorf(
					"cannot cut an alpha tag after a non-alpha release %s. %s",
					version,
					"please specify an allowed release type ('beta')",
				)
			}

			releaseVersions.alpha = fmt.Sprintf(
				"v%d.%d.%d%s.%d",
				buildVersion.major,
				buildVersion.minor,
				buildVersion.patch,
				buildVersion.label,
				labelID,
			)
			releaseVersions.prime = releaseVersions.alpha
		}
	}

	logrus.Infof("Found release versions: %+v", releaseVersions.String())
	return releaseVersions, nil
}
