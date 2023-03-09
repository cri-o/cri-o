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

package image

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v2"

	"sigs.k8s.io/release-utils/util"
)

const (
	// Production registry root URL
	ProdRegistry = "k8s.gcr.io"

	// Staging repository root URL prefix
	StagingRepoPrefix = "gcr.io/k8s-staging-"

	// The suffix of the default image repository to promote images from
	// i.e., gcr.io/<staging-prefix>-<staging-suffix>
	// e.g., gcr.io/k8s-staging-foo
	StagingRepoSuffix = "kubernetes"
)

// ManifestList abstracts the manifest used by the image promoter
type ManifestList []struct {
	Name string `json:"name"`

	// A digest to tag(s) map used in the promoter manifest e.g.,
	// "sha256:ef9493aff21f7e368fb3968b46ff2542b0f6863a5de2b9bc58d8d151d8b0232c": ["v1.17.12-rc.0", "foo", "bar"]
	DMap map[string][]string `json:"dmap"`
}

// NewManifestListFromFile parses an image promoter manifest file
func NewManifestListFromFile(manifestPath string) (imagesList *ManifestList, err error) {
	if !util.Exists(manifestPath) {
		return nil, errors.New("could not find image promoter manifest")
	}
	yamlCode, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("reading yaml code from file: %w", err)
	}

	imagesList = &ManifestList{}
	if err := imagesList.Parse(yamlCode); err != nil {
		return nil, fmt.Errorf("parsing manifest yaml: %w", err)
	}

	return imagesList, nil
}

// Parse reads yaml code into an ImagePromoterManifest object
func (imagesList *ManifestList) Parse(yamlCode []byte) error {
	if err := yaml.Unmarshal(yamlCode, imagesList); err != nil {
		return err
	}
	return nil
}

// Write writes the promoter image list into an YAML file.
func (imagesList *ManifestList) Write(filePath string) error {
	yamlCode, err := imagesList.ToYAML()
	if err != nil {
		return fmt.Errorf("while marshalling image list: %w", err)
	}
	// Write the yaml into the specified file
	if err := os.WriteFile(filePath, yamlCode, os.FileMode(0o644)); err != nil {
		return fmt.Errorf("writing yaml code into file: %w", err)
	}

	return nil
}

// ToYAML serializes an image list into an YAML file.
// We serialize the data by hand to emulate the way it's done by the image promoter
func (imagesList *ManifestList) ToYAML() ([]byte, error) {
	// Images are sorted by:
	//	  1. Name 2. Tag
	// If there are multiple tags for a single image digest, the smallest tag is used in the sort.''
	// Image tags are sorted lexicographically as they are not guaranteed to be Semver compliant. See https://github.com/opencontainers/distribution-spec/issues/154 fpr more details.

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

		sortedDigests := sortImageDigestMapByTag(imgData.DMap)

		for _, digestSHA := range sortedDigests {
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

func sortImageDigestMapByTag(imageDMap map[string][]string) []string {
	var sortedDigests []string
	// we need to sort the map by value (tags)
	tags := make([]string, 0, len(imageDMap))
	valuesToKeys := make(map[string][]string)
	for k, v := range imageDMap {
		// find the smallest tag for each image digest
		sort.Strings(v)
		first := v[0]
		// and add it to the list of tags
		if _, ok := valuesToKeys[first]; !ok {
			tags = append(tags, first)
		}
		valuesToKeys[first] = append(valuesToKeys[first], k)
	}
	// Then, sort the tags lexicographically
	sort.Strings(tags)

	for _, v := range tags {
		// for each version, add the corresponding digest SHA to the list of sorted digests.
		digests := valuesToKeys[v]
		sort.Strings(digests) // in the unlikely case that there are multiple tags for the same digest, we sort them to make sure this function is deterministic
		sortedDigests = append(sortedDigests, digests...)
	}

	return sortedDigests
}
