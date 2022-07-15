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

// Package dependencies checks dependencies, locally or remotely
package dependency

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"sigs.k8s.io/zeitgeist/upstream"
)

// Client holds any client that is needed
type Client struct {
	AWSEC2Client ec2iface.EC2API
}

// Dependencies is used to deserialise the configuration file
type Dependencies struct {
	Dependencies []*Dependency `yaml:"dependencies"`
}

// Dependency is the internal representation of a dependency
type Dependency struct {
	Name string `yaml:"name"`
	// Version of the dependency that should be present throughout your code
	Version string `yaml:"version"`
	// Scheme for versioning this dependency
	Scheme VersionScheme `yaml:"scheme"`
	// Optional: sensitivity, to alert e.g. on new major versions
	Sensitivity VersionSensitivity `yaml:"sensitivity"`
	// Optional: upstream
	Upstream map[string]string `yaml:"upstream"`
	// List of references to this dependency in local files
	RefPaths []*RefPath `yaml:"refPaths"`
}

// RefPath represents a file to check for a reference to the version
type RefPath struct {
	// Path of the file to test
	Path string `yaml:"path"`
	// Match expression for the line that should contain the dependency's version. Regexp is supported.
	Match string `yaml:"match"`
}

// NewClient returns all clients that can be used to the validation
func NewClient() *Client {
	return &Client{
		AWSEC2Client: upstream.NewAWSClient(),
	}
}

// UnmarshalYAML implements custom unmarshalling of Dependency with validation
func (decoded *Dependency) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Use a different type to prevent infinite loop in unmarshalling
	type DependencyYAML Dependency

	d := (*DependencyYAML)(decoded)

	if err := unmarshal(&d); err != nil {
		return err
	}

	// Custom validation for the Dependency type
	if d.Name == "" {
		return fmt.Errorf("Dependency has no `name`: %#v", d)
	}

	if d.Version == "" {
		return fmt.Errorf("Dependency has no `version`: %#v", d)
	}

	// Default scheme to Semver if unset
	if d.Scheme == "" {
		d.Scheme = Semver
	}

	// Validate Scheme and return
	switch d.Scheme {
	case Semver, Alpha, Random:
		// All good!
	default:
		return fmt.Errorf("unknown version scheme: %s", d.Scheme)
	}

	log.Debugf("Deserialised Dependency %s: %#v", d.Name, d)

	return nil
}

func fromFile(dependencyFilePath string) (*Dependencies, error) {
	depFile, err := ioutil.ReadFile(dependencyFilePath)
	if err != nil {
		return nil, err
	}

	dependencies := &Dependencies{}

	err = yaml.Unmarshal(depFile, dependencies)
	if err != nil {
		return nil, err
	}

	return dependencies, nil
}

// LocalCheck checks whether dependencies are in-sync locally
//
// Will return an error if the dependency cannot be found in the files it has defined, or if the version does not match
func (c *Client) LocalCheck(dependencyFilePath, basePath string) error {
	log.Debugf("Base path: %s", basePath)
	externalDeps, err := fromFile(dependencyFilePath)
	if err != nil {
		return err
	}

	var nonMatchingPaths []string
	for _, dep := range externalDeps.Dependencies {
		log.Debugf("Examining dependency: %s", dep.Name)

		for _, refPath := range dep.RefPaths {
			filePath := filepath.Join(basePath, refPath.Path)

			log.Debugf("Examining file: %s", filePath)

			file, err := os.Open(filePath)
			if err != nil {
				return err
			}

			match := refPath.Match
			matcher, err := regexp.Compile(match)
			if err != nil {
				return fmt.Errorf("compiling regex: %w", err)
			}
			scanner := bufio.NewScanner(file)

			var found bool

			var lineNumber int
			for scanner.Scan() {
				lineNumber++

				line := scanner.Text()
				if matcher.MatchString(line) {
					if strings.Contains(line, dep.Version) {
						log.Debugf(
							"Line %d matches expected regexp %q and version %q: %s",
							lineNumber,
							match,
							dep.Version,
							line,
						)

						found = true
						break
					}
				}
			}

			if !found {
				log.Debugf("Finished reading file %s, no match found.", filePath)

				nonMatchingPaths = append(nonMatchingPaths, refPath.Path)
			}
		}

		if len(nonMatchingPaths) > 0 {
			log.Errorf(
				"%s indicates that %s should be at version %s, but the following files didn't match: %s",
				dependencyFilePath,
				dep.Name,
				dep.Version,
				strings.Join(nonMatchingPaths, ", "),
			)

			return errors.New("Dependencies are not in sync")
		}
	}

	return nil
}

// RemoteCheck checks whether dependencies are up to date with upstream
//
// Will return an error if checking the versions upstream fails.
//
// Out-of-date dependencies will be printed out on stdout at the INFO level.
func (c *Client) RemoteCheck(dependencyFilePath string) ([]string, error) {
	externalDeps, err := fromFile(dependencyFilePath)
	if err != nil {
		return nil, err
	}

	updates := make([]string, 0)

	versionUpdateInfos, err := c.checkUpstreamVersions(externalDeps.Dependencies)
	if err != nil {
		return nil, err
	}

	for _, vu := range versionUpdateInfos {
		if vu.updateAvailable {
			updates = append(
				updates,
				fmt.Sprintf(
					"Update available for dependency %s: %s (current: %s)",
					vu.name,
					vu.latest.Version,
					vu.current.Version,
				),
			)
		} else {
			log.Debugf(
				"No update available for dependency %s: %s (latest: %s)\n",
				vu.name,
				vu.current.Version,
				vu.latest.Version,
			)
		}
	}

	return updates, nil
}

func (c *Client) RemoteExport(dependencyFilePath string) ([]VersionUpdate, error) {
	externalDeps, err := fromFile(dependencyFilePath)
	if err != nil {
		return nil, err
	}

	versionUpdates := []VersionUpdate{}

	versionUpdatesInfos, err := c.checkUpstreamVersions(externalDeps.Dependencies)
	if err != nil {
		return nil, err
	}

	for _, vui := range versionUpdatesInfos {
		if vui.updateAvailable {
			versionUpdates = append(versionUpdates, VersionUpdate{
				Name:       vui.name,
				Version:    vui.current.Version,
				NewVersion: vui.latest.Version,
			})
		} else {
			log.Debugf(
				"No update available for dependency %s: %s (latest: %s)\n",
				vui.name,
				vui.current.Version,
				vui.latest.Version,
			)
		}
	}
	return versionUpdates, nil
}

func (c *Client) checkUpstreamVersions(deps []*Dependency) ([]versionUpdateInfo, error) {
	versionUpdates := []versionUpdateInfo{}
	for _, dep := range deps {
		if dep.Upstream == nil {
			continue
		}

		up := dep.Upstream
		latestVersion := Version{dep.Version, dep.Scheme}
		currentVersion := Version{dep.Version, dep.Scheme}

		var err error

		// Cast the flavour from the currently unknown upstream type
		flavour := upstream.Flavour(up["flavour"])
		switch flavour {
		case upstream.DummyFlavour:
			var d upstream.Dummy

			decodeErr := mapstructure.Decode(up, &d)
			if decodeErr != nil {
				return nil, decodeErr
			}

			latestVersion.Version, err = d.LatestVersion()
		case upstream.GithubFlavour:
			var gh upstream.Github

			decodeErr := mapstructure.Decode(up, &gh)
			if decodeErr != nil {
				return nil, decodeErr
			}

			latestVersion.Version, err = gh.LatestVersion()
		case upstream.GitLabFlavour:
			var gl upstream.GitLab

			decodeErr := mapstructure.Decode(up, &gl)
			if decodeErr != nil {
				return nil, decodeErr
			}

			latestVersion.Version, err = gl.LatestVersion()
		case upstream.HelmFlavour:
			var h upstream.Helm

			decodeErr := mapstructure.Decode(up, &h)
			if decodeErr != nil {
				return nil, decodeErr
			}

			latestVersion.Version, err = h.LatestVersion()
		case upstream.AMIFlavour:
			var ami upstream.AMI

			decodeErr := mapstructure.Decode(up, &ami)
			if decodeErr != nil {
				return nil, decodeErr
			}

			ami.ServiceClient = c.AWSEC2Client

			latestVersion.Version, err = ami.LatestVersion()
		case upstream.ContainerFlavour:
			var ct upstream.Container

			decodeErr := mapstructure.Decode(up, &ct)
			if decodeErr != nil {
				log.Debug("errr decoding")
				return nil, decodeErr
			}

			latestVersion.Version, err = ct.LatestVersion()
		case upstream.EKSFlavour:
			var eks upstream.EKS

			decodeErr := mapstructure.Decode(up, &eks)
			if decodeErr != nil {
				return nil, decodeErr
			}

			latestVersion.Version, err = eks.LatestVersion()
		default:
			return nil, fmt.Errorf("unknown upstream flavour '%#v' for dependency %s", flavour, dep.Name)
		}

		if err != nil {
			return nil, err
		}

		updateAvailable, err := latestVersion.MoreSensitivelyRecentThan(currentVersion, dep.Sensitivity)
		if err != nil {
			return nil, err
		}

		versionUpdates = append(versionUpdates, versionUpdateInfo{
			name:            dep.Name,
			current:         currentVersion,
			latest:          latestVersion,
			updateAvailable: updateAvailable,
		})
	}

	return versionUpdates, nil
}
