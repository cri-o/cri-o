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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/release-sdk/sign"
	"sigs.k8s.io/release-utils/command"
)

// Images is a wrapper around container image related functionality.
type Images struct {
	imageImpl
	signer *sign.Signer
}

// NewImages creates a new Images instance
func NewImages() *Images {
	return &Images{
		imageImpl: &defaultImageImpl{},
		signer:    sign.New(sign.Default()),
	}
}

// SetImpl can be used to set the internal image implementation.
func (i *Images) SetImpl(impl imageImpl) {
	i.imageImpl = impl
}

// imageImpl is a client for working with container images.
//
//counterfeiter:generate . imageImpl
type imageImpl interface {
	Execute(cmd string, args ...string) error
	ExecuteOutput(cmd string, args ...string) (string, error)
	RepoTagFromTarball(path string) (string, error)
	SignImage(*sign.Signer, string) error
	VerifyImage(*sign.Signer, string) error
}

type defaultImageImpl struct{}

func (*defaultImageImpl) Execute(cmd string, args ...string) error {
	return command.New(cmd, args...).RunSilentSuccess()
}

func (*defaultImageImpl) ExecuteOutput(cmd string, args ...string) (string, error) {
	res, err := command.New(cmd, args...).RunSilentSuccessOutput()
	if err != nil {
		return "", err
	}
	return res.OutputTrimNL(), nil
}

func (*defaultImageImpl) RepoTagFromTarball(path string) (string, error) {
	tagOutput, err := command.
		New("tar", "xf", path, "manifest.json", "-O").
		Pipe("jq", "-r", ".[0].RepoTags[0]").
		RunSilentSuccessOutput()
	if err != nil {
		return "", err
	}
	return tagOutput.OutputTrimNL(), nil
}

func (*defaultImageImpl) SignImage(signer *sign.Signer, reference string) error {
	_, err := signer.SignImage(reference)
	return err
}

func (*defaultImageImpl) VerifyImage(signer *sign.Signer, reference string) error {
	_, err := signer.VerifyImage(reference)
	return err
}

var tagRegex = regexp.MustCompile(`^.+/(.+):.+$`)

// PublishImages releases container images to the provided target registry
func (i *Images) Publish(registry, version, buildPath string) error {
	version = i.normalizeVersion(version)

	releaseImagesPath := filepath.Join(buildPath, ImagesPath)
	logrus.Infof(
		"Pushing container images from %s to registry %s",
		releaseImagesPath, registry,
	)

	manifestImages, err := i.GetManifestImages(
		registry, version, buildPath,
		func(path, origTag, newTagWithArch string) error {
			if err := i.Execute(
				"docker", "load", "-qi", path,
			); err != nil {
				return fmt.Errorf("load container image: %w", err)
			}

			if err := i.Execute(
				"docker", "tag", origTag, newTagWithArch,
			); err != nil {
				return fmt.Errorf("tag container image: %w", err)
			}

			logrus.Infof("Pushing %s", newTagWithArch)

			if err := i.Execute(
				"gcloud", "docker", "--", "push", newTagWithArch,
			); err != nil {
				return fmt.Errorf("push container image: %w", err)
			}

			if err := i.SignImage(i.signer, newTagWithArch); err != nil {
				return fmt.Errorf("sign container image: %w", err)
			}

			if err := i.Execute(
				"docker", "rmi", origTag, newTagWithArch,
			); err != nil {
				return fmt.Errorf("remove local container image: %w", err)
			}

			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("get manifest images: %w", err)
	}

	if err := os.Setenv("DOCKER_CLI_EXPERIMENTAL", "enabled"); err != nil {
		return fmt.Errorf("enable docker experimental CLI: %w", err)
	}

	for image, arches := range manifestImages {
		imageVersion := fmt.Sprintf("%s:%s", image, version)
		logrus.Infof("Creating manifest image %s", imageVersion)

		manifests := []string{}
		for _, arch := range arches {
			manifests = append(manifests,
				fmt.Sprintf("%s-%s:%s", image, arch, version),
			)
		}
		if err := i.Execute("docker", append(
			[]string{"manifest", "create", "--amend", imageVersion},
			manifests...,
		)...); err != nil {
			return fmt.Errorf("create manifest: %w", err)
		}

		for _, arch := range arches {
			logrus.Infof(
				"Annotating %s-%s:%s with --arch %s",
				image, arch, version, arch,
			)
			if err := i.Execute(
				"docker", "manifest", "annotate", "--arch", arch,
				imageVersion, fmt.Sprintf("%s-%s:%s", image, arch, version),
			); err != nil {
				return fmt.Errorf("annotate manifest with arch: %w", err)
			}
		}

		logrus.Infof("Pushing manifest image %s", imageVersion)
		if err := wait.ExponentialBackoff(wait.Backoff{
			Duration: time.Second,
			Factor:   1.5,
			Steps:    5,
		}, func() (bool, error) {
			if err := i.Execute("docker", "manifest", "push", imageVersion, "--purge"); err == nil {
				return true, nil
			} else if strings.Contains(err.Error(), "request canceled while waiting for connection") {
				// The error is unfortunately not exported:
				// https://github.com/golang/go/blob/dc04f3b/src/net/http/client.go#L720
				// https://github.com/golang/go/blob/dc04f3b/src/net/http/transport.go#L2518
				// ref: https://github.com/kubernetes/release/issues/2810
				logrus.Info("Retrying manifest push")
				return false, nil
			}

			return false, err
		}); err != nil {
			return fmt.Errorf("push manifest: %w", err)
		}

		if err := i.SignImage(i.signer, imageVersion); err != nil {
			return fmt.Errorf("sign manifest list: %w", err)
		}
	}

	return nil
}

// Validates that image manifests have been pushed to a specified remote
// registry.
func (i *Images) Validate(registry, version, buildPath string) error {
	logrus.Infof("Validating image manifests in %s", registry)
	version = i.normalizeVersion(version)

	manifestImages, err := i.GetManifestImages(
		registry, version, buildPath,
		func(_, _, image string) error {
			logrus.Infof("Verifying that image is signed: %s", image)
			if err := i.VerifyImage(i.signer, image); err != nil {
				return fmt.Errorf("verify signed image: %w", err)
			}
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("get manifest images: %w", err)
	}
	logrus.Infof("Got manifest images %+v", manifestImages)

	for image, arches := range manifestImages {
		imageVersion := fmt.Sprintf("%s:%s", image, version)

		manifestBytes, err := crane.Manifest(imageVersion)
		if err != nil {
			return fmt.Errorf("get remote manifest from %s: %w", imageVersion, err)
		}

		logrus.Info("Verifying that image manifest list is signed")
		if err := i.VerifyImage(i.signer, imageVersion); err != nil {
			return fmt.Errorf("verify signed manifest list: %w", err)
		}

		manifest := string(manifestBytes)
		manifestFile, err := os.CreateTemp("", "manifest-")
		if err != nil {
			return fmt.Errorf("create temp file for manifest: %w", err)
		}
		if _, err := manifestFile.WriteString(manifest); err != nil {
			return fmt.Errorf("write manifest to %s: %w", manifestFile.Name(), err)
		}

		for _, arch := range arches {
			logrus.Infof(
				"Checking image digest for %s on %s architecture", image, arch,
			)

			digest, err := i.ExecuteOutput(
				"jq", "--arg", "a", arch, "-r",
				".manifests[] | select(.platform.architecture == $a) | .digest",
				manifestFile.Name(),
			)
			if err != nil {
				return fmt.Errorf("get digest from manifest file %s for arch %s: %w", manifestFile.Name(), arch, err)
			}

			if digest == "" {
				return fmt.Errorf(
					"could not find the image digest for %s on %s",
					imageVersion, arch,
				)
			}

			logrus.Infof("Digest for %s on %s: %s", imageVersion, arch, digest)
		}

		if err := os.RemoveAll(manifestFile.Name()); err != nil {
			return fmt.Errorf("remove manifest file: %w", err)
		}
	}

	return nil
}

// Exists verifies that a set of image manifests exists on a specified remote
// registry. This is a simpler check than Validate, which doesn't presuppose the
// existence of a local build directory. Used in CI builds to quickly validate
// if a build is actually required.
func (i *Images) Exists(registry, version string, fast bool) (bool, error) {
	logrus.Infof("Validating image manifests in %s", registry)
	version = i.normalizeVersion(version)

	manifestImages := ManifestImages

	arches := SupportedArchitectures
	if fast {
		arches = FastArchitectures
	}

	for _, image := range manifestImages {
		imageVersion := fmt.Sprintf("%s/%s:%s", registry, image, version)

		manifestBytes, err := crane.Manifest(imageVersion)
		if err != nil {
			return false, fmt.Errorf("get remote manifest from %s: %w", imageVersion, err)
		}

		manifest := string(manifestBytes)
		manifestFile, err := os.CreateTemp("", "manifest-")
		if err != nil {
			return false, fmt.Errorf("create temp file for manifest: %w", err)
		}
		if _, err := manifestFile.WriteString(manifest); err != nil {
			return false, fmt.Errorf("write manifest to %s: %w", manifestFile.Name(), err)
		}

		for _, arch := range arches {
			logrus.Infof(
				"Checking image digest for %s on %s architecture", image, arch,
			)

			digest, err := i.ExecuteOutput(
				"jq", "--arg", "a", arch, "-r",
				".manifests[] | select(.platform.architecture == $a) | .digest",
				manifestFile.Name(),
			)
			if err != nil {
				return false, fmt.Errorf("get digest from manifest file %s for arch %s: %w", manifestFile.Name(), arch, err)
			}

			if digest == "" {
				return false, fmt.Errorf(
					"could not find the image digest for %s on %s",
					imageVersion, arch,
				)
			}

			logrus.Infof("Digest for %s on %s: %s", imageVersion, arch, digest)
		}

		if err := os.RemoveAll(manifestFile.Name()); err != nil {
			return false, fmt.Errorf("remove manifest file: %w", err)
		}
	}

	return true, nil
}

// GetManifestImages can be used to retrieve the map of built images and
// architectures.
func (i *Images) GetManifestImages(
	registry, version, buildPath string,
	forTarballFn func(path, origTag, newTagWithArch string) error,
) (map[string][]string, error) {
	manifestImages := make(map[string][]string)

	releaseImagesPath := filepath.Join(buildPath, ImagesPath)
	logrus.Infof("Getting manifest images in %s", releaseImagesPath)

	archPaths, err := os.ReadDir(releaseImagesPath)
	if err != nil {
		return nil, fmt.Errorf("read images path %s: %w", releaseImagesPath, err)
	}

	for _, archPath := range archPaths {
		arch := archPath.Name()
		if !archPath.IsDir() {
			logrus.Infof("Skipping %s because it's not a directory", arch)
			continue
		}

		if err := filepath.Walk(
			filepath.Join(releaseImagesPath, arch),
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}

				fileName := info.Name()
				if !strings.HasSuffix(fileName, ".tar") {
					logrus.Infof("Skipping non-tarball %s", fileName)
					return nil
				}

				origTag, err := i.RepoTagFromTarball(path)
				if err != nil {
					return fmt.Errorf("getting repo tags for tarball: %w", err)
				}

				tagMatches := tagRegex.FindStringSubmatch(origTag)
				if len(tagMatches) != 2 {
					return fmt.Errorf(
						"malformed tag %s in %s", origTag, path,
					)
				}

				binary := tagMatches[1]
				newTag := filepath.Join(
					registry,
					strings.TrimSuffix(binary, "-"+arch),
				)
				newTagWithArch := fmt.Sprintf("%s-%s:%s", newTag, arch, version)
				manifestImages[newTag] = append(manifestImages[newTag], arch)

				if forTarballFn != nil {
					if err := forTarballFn(
						path, origTag, newTagWithArch,
					); err != nil {
						return fmt.Errorf("executing tarball callback: %w", err)
					}
				}
				return nil
			},
		); err != nil {
			return nil, fmt.Errorf("traversing path: %w", err)
		}
	}
	return manifestImages, nil
}

// normalizeVersion normalizes an container image version by replacing all invalid characters.
func (i *Images) normalizeVersion(version string) string {
	return strings.ReplaceAll(version, "+", "_")
}
