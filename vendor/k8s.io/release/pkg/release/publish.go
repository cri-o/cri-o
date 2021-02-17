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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/release/pkg/gcp"
	"k8s.io/release/pkg/http"
	"k8s.io/release/pkg/object"
	"k8s.io/release/pkg/util"
)

// Publisher is the structure for publishing anything release related
type Publisher struct {
	client   publisherClient
	objStore object.Store
}

// NewPublisher creates a new Publisher instance
func NewPublisher() *Publisher {
	return &Publisher{
		client:   &defaultPublisher{},
		objStore: object.NewGCS(),
	}
}

// SetClient can be used to set the internal publisher client
func (p *Publisher) SetClient(client publisherClient) {
	p.client = client
}

// publisherClient is a client for working with GCS
//counterfeiter:generate . publisherClient
type publisherClient interface {
	GSUtil(args ...string) error
	GSUtilOutput(args ...string) (string, error)
	GetURLResponse(url string) (string, error)
}

type defaultPublisher struct{}

func (*defaultPublisher) GSUtil(args ...string) error {
	return gcp.GSUtil(args...)
}

func (*defaultPublisher) GSUtilOutput(args ...string) (string, error) {
	return gcp.GSUtilOutput(args...)
}

func (*defaultPublisher) GetURLResponse(url string) (string, error) {
	return http.GetURLResponse(url, true)
}

// Publish a new version, (latest or stable) but only if the files actually
// exist on GCS and the artifacts we're dealing with are newer than the
// contents in GCS.
// buildType - One of 'release' or 'ci'
// version - The version
// buildDir - build output directory
// bucket - GCS bucket
// gcsRoot - The top-level GCS directory builds will be released to
//
// Expected destination format:
//   gs://<bucket>/<gcsRoot>[/fast]/<version>
//
func (p *Publisher) PublishVersion(
	buildType, version, buildDir, bucket, gcsRoot string,
	extraVersionMarkers []string,
	privateBucket, fast bool,
) error {
	logrus.Info("Publishing version")
	releaseType := "latest"

	if buildType == "release" {
		// For release/ targets, type should be 'stable'
		if !(strings.Contains(version, ReleaseTypeAlpha) ||
			strings.Contains(version, ReleaseTypeBeta) ||
			strings.Contains(version, ReleaseTypeRC)) {
			releaseType = "stable"
		}
	}

	sv, err := util.TagStringToSemver(version)
	if err != nil {
		return errors.Errorf("invalid version %s", version)
	}

	markerPath, markerPathErr := p.objStore.GetMarkerPath(
		bucket,
		gcsRoot,
	)
	if markerPathErr != nil {
		return errors.Wrap(markerPathErr, "get version marker path")
	}

	releasePath, releasePathErr := p.objStore.GetReleasePath(
		bucket,
		gcsRoot,
		version,
		fast,
	)
	if releasePathErr != nil {
		return errors.Wrap(releasePathErr, "get release path")
	}

	// TODO: This should probably be a more thorough check of explicit files
	// TODO: This should explicitly do a `gsutil ls` via gcs.PathExists
	if err := p.client.GSUtil("ls", releasePath); err != nil {
		return errors.Wrapf(err, "release files don't exist at %s", releasePath)
	}

	var versionMarkers []string
	if fast {
		versionMarkers = append(
			versionMarkers,
			releaseType+"-fast",
		)
	} else {
		versionMarkers = append(
			versionMarkers,
			releaseType,
			fmt.Sprintf("%s-%d", releaseType, sv.Major),
			fmt.Sprintf("%s-%d.%d", releaseType, sv.Major, sv.Minor),
		)
	}

	if len(extraVersionMarkers) > 0 {
		versionMarkers = append(versionMarkers, extraVersionMarkers...)
	}

	logrus.Infof("Publish version markers: %v", versionMarkers)
	logrus.Infof("Publish official pointer text files to %s", markerPath)

	for _, file := range versionMarkers {
		versionMarker := file + ".txt"
		needsUpdate, err := p.VerifyLatestUpdate(
			versionMarker, markerPath, version,
		)
		if err != nil {
			return errors.Wrapf(err, "verify latest update for %s", versionMarker)
		}

		// If there's a version that's above the one we're trying to release,
		// don't do anything, and just try the next one.
		if !needsUpdate {
			logrus.Infof(
				"Skipping %s for %s because it does not need to be updated",
				versionMarker, version,
			)
			continue
		}

		if err := p.PublishToGcs(
			versionMarker, buildDir, markerPath, version, privateBucket,
		); err != nil {
			return errors.Wrap(err, "publish release to GCS")
		}
	}

	return nil
}

// VerifyLatestUpdate checks if the new version is greater than the version
// currently published on GCS. It returns `true` for `needsUpdate` if the remote
// version does not exist or needs to be updated.
// publishFile - the version marker to look for
// markerPath - the GCS path to search for the version marker in
// version - release version
func (p *Publisher) VerifyLatestUpdate(
	publishFile, markerPath, version string,
) (needsUpdate bool, err error) {
	logrus.Infof("Testing %s > %s (published)", version, publishFile)

	publishFileDst, publishFileDstErr := p.objStore.NormalizePath(markerPath, publishFile)
	if publishFileDstErr != nil {
		return false, errors.Wrap(publishFileDstErr, "get marker file destination")
	}

	// TODO: Should we add a object.`GCS` method for `gsutil cat`?
	gcsVersion, err := p.client.GSUtilOutput("cat", publishFileDst)
	if err != nil {
		logrus.Infof("%s does not exist but will be created", publishFileDst)
		return true, nil
	}

	sv, err := util.TagStringToSemver(version)
	if err != nil {
		return false, errors.Errorf("invalid version format %s", version)
	}

	gcsSemverVersion, err := util.TagStringToSemver(gcsVersion)
	if err != nil {
		return false, errors.Errorf("invalid GCS version format %s", gcsVersion)
	}

	if sv.LTE(gcsSemverVersion) {
		logrus.Infof(
			"Not updating version, because %s <= %s", version, gcsVersion,
		)
		return false, nil
	}

	logrus.Infof("Updating version, because %s > %s", version, gcsVersion)
	return true, nil
}

// PublishToGcs publishes a release to GCS
// publishFile - the GCS location to look in
// buildDir - build output directory
// markerPath - the GCS path to publish a version marker to
// version - release version
func (p *Publisher) PublishToGcs(
	publishFile, buildDir, markerPath, version string,
	privateBucket bool,
) error {
	releaseStage := filepath.Join(buildDir, ReleaseStagePath)
	publishFileDst, publishFileDstErr := p.objStore.NormalizePath(markerPath, publishFile)
	if publishFileDstErr != nil {
		return errors.Wrap(publishFileDstErr, "get marker file destination")
	}

	publicLink := fmt.Sprintf("%s/%s", URLPrefixForBucket(markerPath), publishFile)
	if strings.HasPrefix(markerPath, ProductionBucket) {
		publicLink = fmt.Sprintf("%s/%s", ProductionBucketURL, publishFile)
	}

	uploadDir := filepath.Join(releaseStage, "upload")
	if err := os.MkdirAll(uploadDir, os.FileMode(0o755)); err != nil {
		return errors.Wrapf(err, "create upload dir %s", uploadDir)
	}

	latestFile := filepath.Join(uploadDir, "latest")
	if err := ioutil.WriteFile(
		latestFile, []byte(version), os.FileMode(0o644),
	); err != nil {
		return errors.Wrap(err, "write latest version file")
	}

	if err := p.client.GSUtil(
		"-m",
		"-h", "Content-Type:text/plain",
		"-h", "Cache-Control:private, max-age=0, no-transform",
		"cp",
		latestFile,
		publishFileDst,
	); err != nil {
		return errors.Wrapf(err, "copy %s to %s", latestFile, publishFileDst)
	}

	var content string
	if !privateBucket {
		// New Kubernetes infra buckets, like k8s-staging-kubernetes, have a
		// bucket-only ACL policy set, which means attempting to set the ACL on
		// an object will fail. We should skip this ACL change in those
		// instances, as new buckets already default to being publicly
		// readable.
		//
		// Ref:
		// - https://cloud.google.com/storage/docs/bucket-policy-only
		// - https://github.com/kubernetes/release/issues/904
		if !strings.HasPrefix(markerPath, object.GcsPrefix+"k8s-") {
			aclOutput, err := p.client.GSUtilOutput(
				"acl", "ch", "-R", "-g", "all:R", publishFileDst,
			)
			if err != nil {
				return errors.Wrapf(err, "change %s permissions", publishFileDst)
			}
			logrus.Infof("Making uploaded version file public: %s", aclOutput)
		}

		// If public, validate public link
		response, err := p.client.GetURLResponse(publicLink)
		if err != nil {
			return errors.Wrapf(err, "get content of %s", publicLink)
		}
		content = response
	} else {
		response, err := p.client.GSUtilOutput("cat", publicLink)
		if err != nil {
			return errors.Wrapf(err, "get content of %s", publicLink)
		}
		content = response
	}

	logrus.Infof("Validating uploaded version file at %s", publicLink)
	if version != content {
		return errors.Errorf(
			"version %s it not equal response %s",
			version, content,
		)
	}

	logrus.Info("Version equals response")
	return nil
}
