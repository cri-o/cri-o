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
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/containers/image/v5/docker/archive"
	"github.com/containers/image/v5/docker/tarfile"
	"github.com/containers/image/v5/manifest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gopkg.in/yaml.v2"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/release/pkg/command"
	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/http"
	"k8s.io/release/pkg/object"
	"k8s.io/release/pkg/tar"
	"k8s.io/release/pkg/util"
)

const (
	DefaultToolRepo   = "release"
	DefaultToolBranch = git.DefaultBranch
	DefaultToolOrg    = git.DefaultGithubOrg
	// TODO(vdf): Need to reference K8s Infra project here
	DefaultKubernetesStagingProject = "kubernetes-release-test"
	DefaultRelengStagingProject     = "k8s-staging-releng"
	DefaultDiskSize                 = "500"
	BucketPrefix                    = "kubernetes-release-"
	BucketPrefixK8sInfra            = "k8s-release-"

	versionReleaseRE   = `v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[a-zA-Z0-9]+)*\.*(0|[1-9][0-9]*)?`
	versionBuildRE     = `([0-9]{1,})\+([0-9a-f]{5,40})`
	versionWorkspaceRE = `gitVersion ([^\n]+)`
	versionDirtyRE     = `(-dirty)`

	KubernetesTar = "kubernetes.tar.gz"

	// Staged source code tarball of Kubernetes
	SourcesTar = "src.tar.gz"

	// Root path on the bucket for staged artifacts
	StagePath = "stage"

	// Path where the release container images are stored
	ImagesPath = "release-images"

	// GCSStagePath is the directory where release artifacts are staged before
	// push to GCS.
	GCSStagePath = "gcs-stage"

	// ReleaseStagePath is the directory where releases are staged.
	ReleaseStagePath = "release-stage"

	// GCEPath is the directory where GCE scripts are created.
	GCEPath = ReleaseStagePath + "/full/kubernetes/cluster/gce"

	// GCIPath is the path for the container optimized OS for GCP.
	GCIPath = ReleaseStagePath + "/full/kubernetes/cluster/gce/gci"

	// ReleaseTarsPath is the directory where release artifacts are created.
	ReleaseTarsPath = "release-tars"

	// WindowsLocalPath is the directory where Windows GCE scripts are created.
	WindowsLocalPath = ReleaseStagePath + "/full/kubernetes/cluster/gce/windows"

	// CIBucketLegacy is the default bucket for Kubernetes CI releases
	CIBucketLegacy = "kubernetes-release-dev"

	// CIBucketK8sInfra is the community infra bucket for Kubernetes CI releases
	CIBucketK8sInfra = "k8s-release-dev"

	// TestBucket is the default bucket for mocked Kubernetes releases
	TestBucket = "kubernetes-release-gcb"

	// ProductionBucket is the default bucket for Kubernetes releases
	ProductionBucket = "kubernetes-release"

	// ProductionBucketURL is the url for the ProductionBucket
	ProductionBucketURL = "https://dl.k8s.io"

	// Production registry root URL
	GCRIOPathProd = "k8s.gcr.io"

	// Staging registry root URL
	GCRIOPathStaging = "gcr.io/k8s-staging-kubernetes"

	// Mock staging registry root URL
	GCRIOPathMock = GCRIOPathStaging + "/mock"

	// BuildDir is the default build output directory.
	BuildDir = "_output"

	// The default bazel build directory.
	BazelBuildDir = "bazel-bin/build"

	// Archive path is the root path in the bucket where releases are archived
	ArchivePath = "archive"
)

var (
	ManifestImages = []string{
		"conformance",
		"kube-apiserver",
		"kube-controller-manager",
		"kube-proxy",
		"kube-scheduler",
	}

	SupportedArchitectures = []string{
		"amd64",
		"arm",
		"arm64",
		"ppc64le",
		"s390x",
	}

	FastArchitectures = []string{
		"amd64",
	}
)

// ImagePromoterImages abtracts the manifest used by the image promoter
type ImagePromoterImages []struct {
	Name string              `json:"name"`
	DMap map[string][]string `json:"dmap"` // eg "sha256:ef9493aff21f7e368fb3968b46ff2542b0f6863a5de2b9bc58d8d151d8b0232c": ["v1.17.12-rc.0"]
}

// GetDefaultToolRepoURL returns the default HTTPS repo URL for Release Engineering tools.
// Expected: https://github.com/kubernetes/release
func GetDefaultToolRepoURL() string {
	return GetToolRepoURL(DefaultToolOrg, DefaultToolRepo, false)
}

// GetToolRepoURL takes a GitHub org and repo, and useSSH as a boolean and
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
	bazelBuild := filepath.Join(workDir, BazelBuildDir, ReleaseTarsPath, KubernetesTar)
	dockerBuild := filepath.Join(workDir, BuildDir, ReleaseTarsPath, KubernetesTar)
	return util.MoreRecent(bazelBuild, dockerBuild)
}

// ReadBazelVersion reads the version from a Bazel build.
func ReadBazelVersion(workDir string) (string, error) {
	version, err := ioutil.ReadFile(filepath.Join(workDir, "bazel-bin", "version"))
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
	dockerTarball := filepath.Join(workDir, BuildDir, ReleaseTarsPath, KubernetesTar)
	reader, err := tar.ReadFileFromGzippedTar(
		dockerTarball, filepath.Join("kubernetes", "version"),
	)
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

func GetWorkspaceVersion() (string, error) {
	workspaceStatusScript := "hack/print-workspace-status.sh"
	_, workspaceStatusScriptStatErr := os.Stat(workspaceStatusScript)
	if os.IsNotExist(workspaceStatusScriptStatErr) {
		return "", errors.Wrapf(workspaceStatusScriptStatErr,
			"checking for workspace status script",
		)
	}

	/*
		version = ''
		try:
				match = re.search(
						r'gitVersion ([^\n]+)',
						check_output('hack/print-workspace-status.sh')
				)
				if match:
						version = match.group(1)
		except subprocess.CalledProcessError as exc:
				# fallback with doing a real build
				print >>sys.stderr, 'Failed to get k8s version, continue: %s' % exc
				return False
	*/

	logrus.Info("Getting workspace status")
	workspaceStatusStream, getWorkspaceStatusErr := command.New(workspaceStatusScript).RunSuccessOutput()
	if getWorkspaceStatusErr != nil {
		return "", errors.Wrapf(getWorkspaceStatusErr, "getting workspace status")
	}

	workspaceStatus := workspaceStatusStream.Output()

	re := regexp.MustCompile(versionWorkspaceRE)
	submatch := re.FindStringSubmatch(workspaceStatus)

	version := submatch[1]

	logrus.Infof("Found workspace version: %s", version)
	return version, nil
}

// GetKubecrossVersion returns the current kube-cross container version.
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
	bucket = strings.TrimPrefix(bucket, object.GcsPrefix)
	urlPrefix := fmt.Sprintf("https://storage.googleapis.com/%s", bucket)
	if bucket == ProductionBucket {
		urlPrefix = ProductionBucketURL
	}
	return urlPrefix
}

// GetImageTags Takes a workdir and returns the release images from the manifests
func GetImageTags(workDir string) (imagesList map[string][]string, err error) {
	// Our image list will be lists of tags indexed by arch
	imagesList = make(map[string][]string)

	// Images are held inside a subdir of the workdir
	imagesDir := filepath.Join(workDir, ImagesPath)
	if !util.Exists(imagesDir) {
		return nil, errors.Errorf("images directory %s does not exist", imagesDir)
	}

	archDirs, err := ioutil.ReadDir(imagesDir)
	if err != nil {
		return nil, errors.Wrap(err, "reading images dir")
	}

	for _, archDir := range archDirs {
		imagesList[archDir.Name()] = make([]string, 0)
		tarFiles, err := ioutil.ReadDir(filepath.Join(imagesDir, archDir.Name()))
		if err != nil {
			return nil, errors.Wrapf(err, "listing tar files for %s", archDir.Name())
		}
		for _, tarFile := range tarFiles {
			tarmanifest, err := GetTarManifest(filepath.Join(imagesDir, archDir.Name(), tarFile.Name()))
			if err != nil {
				return nil, errors.Wrapf(
					err, "while getting the manifest from %s/%s",
					archDir.Name(), tarFile.Name(),
				)
			}
			imagesList[archDir.Name()] = append(imagesList[archDir.Name()], tarmanifest.RepoTags...)
		}
	}
	return imagesList, nil
}

// GetTarManifest return the image tar manifest
func GetTarManifest(tarPath string) (*tarfile.ManifestItem, error) {
	imageSource, err := tarfile.NewSourceFromFile(tarPath)
	if err != nil {
		return nil, errors.Wrap(err, "creating image source from tar file")
	}

	defer func() {
		if err := imageSource.Close(); err != nil {
			logrus.Error(err)
		}
	}()

	tarManifest, err := imageSource.LoadTarManifest()
	if err != nil {
		return nil, errors.Wrap(err, "reading the tar manifest")
	}
	if len(tarManifest) == 0 {
		return nil, errors.New("could not find a tar manifest in the specified tar file")
	}
	return &tarManifest[0], nil
}

// GetOCIManifest Reads a tar file and returns a v1.Manifest structure with the image data
func GetOCIManifest(tarPath string) (*ocispec.Manifest, error) {
	ctx := context.Background()

	// Since we know we're working with tar files,
	// get the image reference directly from the tar transport
	ref, err := archive.ParseReference(tarPath)
	if err != nil {
		return nil, errors.Wrap(err, "parsing reference")
	}
	logrus.Info(ref.StringWithinTransport())
	// Get a docker image using the tar reference
	// sys := &types.SystemContext{}

	dockerImage, err := ref.NewImage(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting image")
	}

	// Get the manifest data from the dockerImage
	dockerManifest, _, err := dockerImage.Manifest(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "while getting image manifest")
	}

	// Convert the manifest data to an OCI manifest
	ociman, err := manifest.OCI1FromManifest(dockerManifest)
	if err != nil {
		return nil, errors.Wrap(err, "converting the docker manifest to OCI v1")
	}

	// Return the embedded v1 manifest wrapped in the container/image struct
	return &ociman.Manifest, err
}

// CopyBinaries takes the provided `rootPath` and copies the binaries sorted by
// their platform into the `targetPath`.
func CopyBinaries(rootPath, targetPath string) error {
	platformsPath := filepath.Join(rootPath, "client")
	platformsAndArches, err := ioutil.ReadDir(platformsPath)
	if err != nil {
		return errors.Wrapf(err, "retrieve platforms from %s", platformsPath)
	}

	for _, platformArch := range platformsAndArches {
		if !platformArch.IsDir() {
			logrus.Warnf(
				"Skipping platform and arch %q because it's not a directory",
				platformArch.Name(),
			)
			continue
		}

		split := strings.Split(platformArch.Name(), "-")
		if len(split) != 2 {
			return errors.Errorf(
				"expected `platform-arch` format for %s", platformArch.Name(),
			)
		}

		platform := split[0]
		arch := split[1]
		logrus.Infof(
			"Copying binaries for %s platform on %s arch", platform, arch,
		)

		src := filepath.Join(
			rootPath, "client", platformArch.Name(), "kubernetes", "client", "bin",
		)

		// We assume here the "server package" is a superset of the "client
		// package"
		serverSrc := filepath.Join(rootPath, "server", platformArch.Name())
		if util.Exists(serverSrc) {
			logrus.Infof("Server source found in %s, copying them", serverSrc)
			src = filepath.Join(serverSrc, "kubernetes", "server", "bin")
		}

		dst := filepath.Join(targetPath, "bin", platform, arch)
		logrus.Infof("Copying server binaries from %s to %s", src, dst)
		if err := util.CopyDirContentsLocal(src, dst); err != nil {
			return errors.Wrapf(err,
				"copy server binaries from %s to %s", src, dst,
			)
		}

		// Copy node binaries if they exist and this isn't a 'server' platform
		nodeSrc := filepath.Join(rootPath, "node", platformArch.Name())
		if !util.Exists(serverSrc) && util.Exists(nodeSrc) {
			src = filepath.Join(nodeSrc, "kubernetes", "node", "bin")

			logrus.Infof("Copying node binaries from %s to %s", src, dst)
			if err := util.CopyDirContentsLocal(src, dst); err != nil {
				return errors.Wrapf(err,
					"copy node binaries from %s to %s", src, dst,
				)
			}
		}
	}
	return nil
}

// WriteChecksums writes the SHA256SUMS/SHA512SUMS files (contains all
// checksums) as well as a sepearete *.sha[256|512] file containing only the
// SHA for the corresponding file name.
func WriteChecksums(rootPath string) error {
	logrus.Info("Writing artifact hashes to SHA256SUMS/SHA512SUMS files")

	createSHASums := func(hasher hash.Hash) (string, error) {
		fileName := fmt.Sprintf("SHA%dSUMS", hasher.Size()*8)
		files := []string{}

		if err := filepath.Walk(rootPath,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}

				sha, err := fileToHash(path, hasher)
				if err != nil {
					return errors.Wrap(err, "get hash from file")
				}

				files = append(files, fmt.Sprintf("%s  %s", sha, path))
				return nil
			},
		); err != nil {
			return "", errors.Wrapf(err, "traversing root path %s", rootPath)
		}

		file, err := os.Create(fileName)
		if err != nil {
			return "", errors.Wrapf(err, "create file %s", fileName)
		}
		if _, err := file.WriteString(strings.Join(files, "\n")); err != nil {
			return "", errors.Wrapf(err, "write to file %s", fileName)
		}

		return file.Name(), nil
	}

	// Write the release checksum files.
	// We checksum everything except our checksum files, which we do next.
	sha256SumsFile, err := createSHASums(sha256.New())
	if err != nil {
		return errors.Wrap(err, "create SHA256 sums")
	}
	sha512SumsFile, err := createSHASums(sha512.New())
	if err != nil {
		return errors.Wrap(err, "create SHA512 sums")
	}

	// After all the checksum files are generated, move them into the bucket
	// staging area
	moveFile := func(file string) error {
		if err := util.CopyFileLocal(
			file, filepath.Join(rootPath, file), true,
		); err != nil {
			return errors.Wrapf(err, "move %s sums file to %s", file, rootPath)
		}
		if err := os.RemoveAll(file); err != nil {
			return errors.Wrapf(err, "remove file %s", file)
		}
		return nil
	}
	if err := moveFile(sha256SumsFile); err != nil {
		return errors.Wrap(err, "move SHA256 sums")
	}
	if err := moveFile(sha512SumsFile); err != nil {
		return errors.Wrap(err, "move SHA512 sums")
	}

	logrus.Infof("Hashing files in %s", rootPath)

	writeSHAFile := func(fileName string, hasher hash.Hash) error {
		sha, err := fileToHash(fileName, hasher)
		if err != nil {
			return errors.Wrap(err, "get hash from file")
		}
		shaFileName := fmt.Sprintf("%s.sha%d", fileName, hasher.Size()*8)

		return errors.Wrapf(
			ioutil.WriteFile(shaFileName, []byte(sha), os.FileMode(0o644)),
			"write SHA to file %s", shaFileName,
		)
	}

	if err := filepath.Walk(rootPath,
		func(path string, file os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if file.IsDir() {
				return nil
			}

			if err := writeSHAFile(path, sha256.New()); err != nil {
				return errors.Wrapf(err, "write %s.sha256", file.Name())
			}

			if err := writeSHAFile(path, sha512.New()); err != nil {
				return errors.Wrapf(err, "write %s.sha512", file.Name())
			}
			return nil
		},
	); err != nil {
		return errors.Wrapf(err, "traversing root path %s", rootPath)
	}

	return nil
}

func fileToHash(fileName string, hasher hash.Hash) (string, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return "", errors.Wrapf(err, "opening file %s", fileName)
	}
	defer file.Close()

	hasher.Reset()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", errors.Wrapf(err, "copy file %s into hasher", fileName)
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// NewPromoterImageListFromFile parses an image promoter manifest file
func NewPromoterImageListFromFile(manifestPath string) (imagesList *ImagePromoterImages, err error) {
	if !util.Exists(manifestPath) {
		return nil, errors.New("could not find image promoter manifest")
	}
	yamlCode, err := ioutil.ReadFile(manifestPath)
	if err != nil {
		return nil, errors.Wrap(err, "reading yaml code from file")
	}

	imagesList = &ImagePromoterImages{}
	if err := imagesList.Parse(yamlCode); err != nil {
		return nil, errors.Wrap(err, "parsing manifest yaml")
	}

	return imagesList, nil
}

// Parse reads yaml code into an ImagePromoterManifest object
func (imagesList *ImagePromoterImages) Parse(yamlCode []byte) error {
	if err := yaml.Unmarshal(yamlCode, imagesList); err != nil {
		return err
	}
	return nil
}

// ToYAML serializes an image list into an YAML file.
// We serialize the data by hand to emulate the way it's done by the image promoter
func (imagesList *ImagePromoterImages) ToYAML() ([]byte, error) {
	// The image promoter code sorts images by:
	//	  1. Name 2. Digest SHA (asc)  3. Tag

	// First, sort by name (sort #1)
	sort.Slice(*imagesList, func(i, j int) bool {
		return (*imagesList)[i].Name < (*imagesList)[j].Name
	})

	// Let's build the YAML code
	yamlCode := ""
	for _, imgData := range *imagesList {
		// Add the new name key (it is not sorted in the promoter code)
		yamlCode += fmt.Sprintf("- name: %s\n", imgData.Name)
		yamlCode += "  dmap:\n"

		// Now, lets sort by the digest sha (sort #2)
		keys := make([]string, 0, len(imgData.DMap))
		for k := range imgData.DMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, digestSHA := range keys {
			// Finally, sort bt tag (sort #3)
			tags := imgData.DMap[digestSHA]
			sort.Strings(tags)
			yamlCode += fmt.Sprintf("    %q: [", digestSHA)
			for i, tag := range tags {
				if i > 0 {
					yamlCode += ","
				}
				yamlCode += fmt.Sprintf("%q", tag)
			}
			yamlCode += "]\n"
		}
	}

	return []byte(yamlCode), nil
}

// Write writes the promoter image list into an YAML file.
func (imagesList *ImagePromoterImages) Write(filePath string) error {
	yamlCode, err := imagesList.ToYAML()
	if err != nil {
		return errors.Wrap(err, "while marshalling image list")
	}
	// Write the yaml into the specified file
	if err := ioutil.WriteFile(filePath, yamlCode, os.FileMode(0o644)); err != nil {
		return errors.Wrap(err, "writing yaml code into file")
	}

	return nil
}
