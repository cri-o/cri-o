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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	"sigs.k8s.io/release-sdk/gcli"
	"sigs.k8s.io/release-sdk/object"
	"sigs.k8s.io/release-utils/http"
	"sigs.k8s.io/release-utils/util"
)

// Publisher is the structure for publishing anything release related
type Publisher struct {
	client publisherClient
}

// NewPublisher creates a new Publisher instance
func NewPublisher() *Publisher {
	objStore := *object.NewGCS()
	objStore.SetOptions(
		objStore.WithNoClobber(false),
	)
	return &Publisher{
		client: &defaultPublisher{&objStore},
	}
}

// SetClient can be used to set the internal publisher client
func (p *Publisher) SetClient(client publisherClient) {
	p.client = client
}

// publisherClient is a client for working with GCS
//
//counterfeiter:generate . publisherClient
type publisherClient interface {
	GSUtil(args ...string) error
	GSUtilOutput(args ...string) (string, error)
	GSUtilStatus(args ...string) (bool, error)
	GetURLResponse(url string) (string, error)
	GetReleasePath(bucket, gcsRoot, version string, fast bool) (string, error)
	GetMarkerPath(bucket, gcsRoot string) (string, error)
	NormalizePath(pathParts ...string) (string, error)
	TempDir(dir, pattern string) (name string, err error)
	CopyToLocal(remote, local string) error
	ReadFile(filename string) ([]byte, error)
	Unmarshal(data []byte, v interface{}) error
	Marshal(v interface{}) ([]byte, error)
	TempFile(dir, pattern string) (f *os.File, err error)
	CopyToRemote(local, remote string) error
}

type defaultPublisher struct {
	objStore object.Store
}

func (*defaultPublisher) GSUtil(args ...string) error {
	return gcli.GSUtil(args...)
}

func (*defaultPublisher) GSUtilOutput(args ...string) (string, error) {
	return gcli.GSUtilOutput(args...)
}

func (*defaultPublisher) GSUtilStatus(args ...string) (bool, error) {
	status, err := gcli.GSUtilStatus(args...)
	if err != nil {
		return false, err
	}
	return status.Success(), nil
}

func (*defaultPublisher) GetURLResponse(url string) (string, error) {
	return http.GetURLResponse(url, true)
}

func (d *defaultPublisher) GetReleasePath(
	bucket, gcsRoot, version string, fast bool,
) (string, error) {
	return d.objStore.GetReleasePath(bucket, gcsRoot, version, fast)
}

func (d *defaultPublisher) GetMarkerPath(
	bucket, gcsRoot string,
) (string, error) {
	return d.objStore.GetMarkerPath(bucket, gcsRoot)
}

func (d *defaultPublisher) NormalizePath(pathParts ...string) (string, error) {
	return d.objStore.NormalizePath(pathParts...)
}

func (*defaultPublisher) TempDir(dir, pattern string) (name string, err error) {
	return os.MkdirTemp(dir, pattern)
}

func (d *defaultPublisher) CopyToLocal(remote, local string) error {
	return d.objStore.CopyToLocal(remote, local)
}

func (*defaultPublisher) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func (*defaultPublisher) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (*defaultPublisher) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (*defaultPublisher) TempFile(dir, pattern string) (f *os.File, err error) {
	return os.CreateTemp(dir, pattern)
}

func (d *defaultPublisher) CopyToRemote(local, remote string) error {
	return d.objStore.CopyToRemote(local, remote)
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
//
//	gs://<bucket>/<gcsRoot>[/fast]/<version>
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
		return fmt.Errorf("invalid version %s", version)
	}

	markerPath, markerPathErr := p.client.GetMarkerPath(
		bucket,
		gcsRoot,
	)
	if markerPathErr != nil {
		return fmt.Errorf("get version marker path: %w", markerPathErr)
	}

	releasePath, releasePathErr := p.client.GetReleasePath(
		bucket,
		gcsRoot,
		version,
		fast,
	)
	if releasePathErr != nil {
		return fmt.Errorf("get release path: %w", releasePathErr)
	}

	// TODO: This should probably be a more thorough check of explicit files
	// TODO: This should explicitly do a `gsutil ls` via gcs.PathExists
	if err := p.client.GSUtil("ls", releasePath); err != nil {
		return fmt.Errorf("release files don't exist at %s: %w", releasePath, err)
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
			return fmt.Errorf("verify latest update for %s: %w", versionMarker, err)
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
			return fmt.Errorf("publish release to GCS: %w", err)
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

	publishFileDst, publishFileDstErr := p.client.NormalizePath(markerPath, publishFile)
	if publishFileDstErr != nil {
		return false, fmt.Errorf("get marker file destination: %w", publishFileDstErr)
	}

	// TODO: Should we add a object.`GCS` method for `gsutil cat`?
	gcsVersion, err := p.client.GSUtilOutput("cat", publishFileDst)
	if err != nil {
		logrus.Infof("%s does not exist but will be created", publishFileDst)
		return true, nil
	}

	sv, err := util.TagStringToSemver(version)
	if err != nil {
		return false, fmt.Errorf("invalid version format %s", version)
	}

	gcsSemverVersion, err := util.TagStringToSemver(gcsVersion)
	if err != nil {
		return false, fmt.Errorf("invalid GCS version format %s", gcsVersion)
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
	publishFileDst, publishFileDstErr := p.client.NormalizePath(markerPath, publishFile)
	if publishFileDstErr != nil {
		return fmt.Errorf("get marker file destination: %w", publishFileDstErr)
	}

	publicLink := fmt.Sprintf("%s/%s", URLPrefixForBucket(markerPath), publishFile)
	if strings.HasPrefix(markerPath, ProductionBucket) {
		publicLink = fmt.Sprintf("%s/%s", ProductionBucketURL, publishFile)
	}

	uploadDir := filepath.Join(releaseStage, "upload")
	if err := os.MkdirAll(uploadDir, os.FileMode(0o755)); err != nil {
		return fmt.Errorf("create upload dir %s: %w", uploadDir, err)
	}

	latestFile := filepath.Join(uploadDir, "latest")
	if err := os.WriteFile(
		latestFile, []byte(version), os.FileMode(0o644),
	); err != nil {
		return fmt.Errorf("write latest version file: %w", err)
	}

	if err := p.client.GSUtil(
		"-m",
		"-h", "Content-Type:text/plain",
		"-h", "Cache-Control:private, max-age=0, no-transform",
		"cp",
		latestFile,
		publishFileDst,
	); err != nil {
		return fmt.Errorf("copy %s to %s: %w", latestFile, publishFileDst, err)
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
				return fmt.Errorf("change %s permissions: %w", publishFileDst, err)
			}
			logrus.Infof("Making uploaded version file public: %s", aclOutput)
		}

		// If public, validate public link
		response, err := p.client.GetURLResponse(publicLink)
		if err != nil {
			return fmt.Errorf("get content of %s: %w", publicLink, err)
		}
		content = response
	} else {
		response, err := p.client.GSUtilOutput("cat", publicLink)
		if err != nil {
			return fmt.Errorf("get content of %s: %w", publicLink, err)
		}
		content = response
	}

	logrus.Infof("Validating uploaded version file at %s", publicLink)
	if version != content {
		return fmt.Errorf(
			"version %s it not equal response %s",
			version, content,
		)
	}

	logrus.Info("Version equals response")
	return nil
}

// PublishReleaseNotesIndex updates or creates the release notes index JSON at
// the target `gcsIndexRootPath`.
func (p *Publisher) PublishReleaseNotesIndex(
	gcsIndexRootPath, gcsReleaseNotesPath, version string,
) error {
	const releaseNotesIndex = "/release-notes-index.json"

	indexFilePath, err := p.client.NormalizePath(
		gcsIndexRootPath, releaseNotesIndex,
	)
	if err != nil {
		return fmt.Errorf("normalize index file: %w", err)
	}
	logrus.Infof("Publishing release notes index %s", indexFilePath)

	releaseNotesFilePath, err := p.client.NormalizePath(gcsReleaseNotesPath)
	if err != nil {
		return fmt.Errorf("normalize release notes file: %w", err)
	}

	success, err := p.client.GSUtilStatus("-q", "stat", indexFilePath)
	if err != nil {
		return fmt.Errorf("run gcsutil stat: %w", err)
	}

	logrus.Info("Building release notes index")
	versions := make(map[string]string)
	if success {
		logrus.Info("Modifying existing release notes index file")

		tempDir, err := p.client.TempDir("", "release-notes-index-")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tempDir)
		tempIndexFile := filepath.Join(tempDir, releaseNotesIndex)

		if err := p.client.CopyToLocal(
			indexFilePath, tempIndexFile,
		); err != nil {
			return fmt.Errorf("copy index file to local: %w", err)
		}

		indexBytes, err := p.client.ReadFile(tempIndexFile)
		if err != nil {
			return fmt.Errorf("read local index file: %w", err)
		}

		if err := p.client.Unmarshal(indexBytes, &versions); err != nil {
			return fmt.Errorf("unmarshal versions: %w", err)
		}
	} else {
		logrus.Info("Creating non existing release notes index file")
	}
	versions[version] = releaseNotesFilePath

	versionJSON, err := p.client.Marshal(versions)
	if err != nil {
		return fmt.Errorf("marshal version JSON: %w", err)
	}

	logrus.Infof("Writing new release notes index: %s", string(versionJSON))
	tempFile, err := p.client.TempFile("", "release-notes-index-")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	if _, err := tempFile.Write(versionJSON); err != nil {
		return fmt.Errorf("write temp index: %w", err)
	}

	logrus.Info("Uploading release notes index")
	if err := p.client.CopyToRemote(
		tempFile.Name(), indexFilePath,
	); err != nil {
		return fmt.Errorf("upload index file: %w", err)
	}

	return nil
}
