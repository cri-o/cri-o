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

package spdx

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"sigs.k8s.io/release-utils/util"
)

type YamlBuildArtifact struct {
	Type      string `yaml:"type"` //  directory
	Source    string `yaml:"source"`
	License   string `yaml:"license"`   // SPDX license ID Apache-2.0
	GoModules *bool  `yaml:"gomodules"` // Shoud we scan go modules
}

type YamlBOMConfiguration struct {
	Namespace string `yaml:"namespace"`
	License   string `yaml:"license"` // Document wide license
	Name      string `yaml:"name"`
	Creator   struct {
		Person string `yaml:"person"`
		Tool   string `yaml:"tool"`
	} `yaml:"creator"`
	ExternalDocRefs []ExternalDocumentRef `yaml:"external-docs"`
	Artifacts       []*YamlBuildArtifact  `yaml:"artifacts"`
}

// NewDocBuilderOption is a function with operates on a newDocBuilderSettings object.
type NewDocBuilderOption func(*newDocBuilderSettings)

type newDocBuilderSettings struct {
	format Format
}

// WithFormat returns an NewDocBuilderOption setting the format.
func WithFormat(format Format) NewDocBuilderOption {
	return func(settings *newDocBuilderSettings) {
		settings.format = format
	}
}

func NewDocBuilder(options ...NewDocBuilderOption) *DocBuilder {
	settings := &newDocBuilderSettings{
		format: FormatTagValue,
	}
	for _, option := range options {
		option(settings)
	}
	db := &DocBuilder{
		options: &defaultDocBuilderOpts,
		impl: &defaultDocBuilderImpl{
			format: settings.format,
		},
	}
	return db
}

// DocBuilder is a tool to write SPDX SBOMs. It is configurable by
// defining values in its DocBuilderOptions. Options to customize the
// generated document are passed to the Generate() method in DocGenerateOptions
// struct.
type DocBuilder struct {
	options *DocBuilderOptions
	impl    DocBuilderImplementation
}

// Generate creates a new SPDX SBOM. The resulting document will describe the all
// artifacts specified in the DocGenerateOptions struct passed.
func (db *DocBuilder) Generate(genopts *DocGenerateOptions) (*Document, error) {
	if err := db.impl.ReadYamlConfiguration(genopts.ConfigFile, genopts); err != nil {
		return nil, fmt.Errorf("parsing configuration file: %w", err)
	}

	if err := db.impl.ValidateOptions(genopts); err != nil {
		return nil, fmt.Errorf("checking build options: %w", err)
	}

	spdx, err := db.impl.CreateSPDXClient(genopts, db.options)
	if err != nil {
		return nil, fmt.Errorf("generating spdx client")
	}

	doc, err := db.impl.CreateDocument(genopts, spdx)
	if err != nil {
		return nil, fmt.Errorf("creating spdx document: %w", err)
	}

	if err := db.impl.ScanDirectories(genopts, spdx, doc); err != nil {
		return nil, fmt.Errorf("scanning directories: %w", err)
	}

	if err := db.impl.ScanImages(genopts, spdx, doc); err != nil {
		return nil, fmt.Errorf("scanning images: %w", err)
	}

	if err := db.impl.ScanImageArchives(genopts, spdx, doc); err != nil {
		return nil, fmt.Errorf("scanning image archives: %w", err)
	}

	if err := db.impl.ScanArchives(genopts, spdx, doc); err != nil {
		return nil, fmt.Errorf("scanning archives: %w", err)
	}

	if err := db.impl.ScanFiles(genopts, spdx, doc); err != nil {
		return nil, fmt.Errorf("scanning files: %w", err)
	}

	return doc, nil
}

type DocGenerateOptions struct {
	AnalyseLayers       bool                  // A flag that controls if deep layer analysis should be performed
	NoGitignore         bool                  // Do not read exclusions from gitignore file
	ProcessGoModules    bool                  // Analyze go.mod to include data about packages
	OnlyDirectDeps      bool                  // Only include direct dependencies from go.mod
	ScanLicenses        bool                  // Try to look into files to determine their license
	ScanImages          bool                  // When true, scan images for OS information
	ConfigFile          string                // Path to SBOM configuration file
	Format              string                // Output format
	OutputFile          string                // Output location
	Name                string                // Name to use in the resulting document
	Namespace           string                // Namespace for the document (a unique URI)
	CreatorPerson       string                // Document creator information
	License             string                // Main license of the document
	LicenseListVersion  string                // Version of the SPDX list to use
	Tarballs            []string              // A slice of docker archives (tar)
	Archives            []string              // A list of archive files to add as packages
	Files               []string              // A slice of naked files to include in the bom
	Images              []string              // A slice of docker images
	Directories         []string              // A slice of directories to convert into packages
	IgnorePatterns      []string              // A slice of regexp patterns to ignore when scanning dirs
	ExternalDocumentRef []ExternalDocumentRef // List of external documents related to the bom
}

func (o *DocGenerateOptions) Validate() error {
	if len(o.Tarballs) == 0 &&
		len(o.Files) == 0 &&
		len(o.Images) == 0 &&
		len(o.Directories) == 0 &&
		len(o.Archives) == 0 {
		return errors.New(
			"to build a document at least an image, tarball, directory or a file has to be specified",
		)
	}

	if o.ConfigFile != "" && !util.Exists(o.ConfigFile) {
		return errors.New("the specified configuration file was not found")
	}

	// Check namespace is a valid URL
	if _, err := url.Parse(o.Namespace); err != nil {
		return fmt.Errorf("parsing the namespace URL: %w", err)
	}
	return nil
}

type DocBuilderOptions struct {
	WorkDir string // Working directory (defaults to a tmp dir)
}

var defaultDocBuilderOpts = DocBuilderOptions{
	WorkDir: filepath.Join(os.TempDir(), "spdx-docbuilder"),
}

// TODO: Move this to https://github.com/kubernetes-sigs/release-utils/blob/main/util/common.go#L485
func pathIsOfFile(path string) (bool, error) {
	fInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return !fInfo.IsDir(), nil
}
