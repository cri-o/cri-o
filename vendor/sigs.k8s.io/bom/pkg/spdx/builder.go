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

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

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

// DocBuilder is a tool to write spdx manifests
type DocBuilder struct {
	options *DocBuilderOptions
	impl    DocBuilderImplementation
}

// Generate creates anew SPDX document describing the artifacts specified in the options
func (db *DocBuilder) Generate(genopts *DocGenerateOptions) (*Document, error) {
	if genopts.ConfigFile != "" {
		if err := db.impl.ReadYamlConfiguration(genopts.ConfigFile, genopts); err != nil {
			return nil, fmt.Errorf("parsing configuration file: %w", err)
		}
	}
	// Create the SPDX document
	doc, err := db.impl.GenerateDoc(db.options, genopts)
	if err != nil {
		return nil, fmt.Errorf("creating SPDX document: %w", err)
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

type DocBuilderImplementation interface {
	GenerateDoc(*DocBuilderOptions, *DocGenerateOptions) (*Document, error)
	WriteDoc(*Document, string) error
	ReadYamlConfiguration(string, *DocGenerateOptions) error
}

// defaultDocBuilderImpl is the default implementation for the
// SPDX document builder
type defaultDocBuilderImpl struct {
	format Format
}

// Generate generates a document
func (builder *defaultDocBuilderImpl) GenerateDoc(
	opts *DocBuilderOptions, genopts *DocGenerateOptions,
) (doc *Document, err error) {
	if err := genopts.Validate(); err != nil {
		return nil, fmt.Errorf("checking build options: %w", err)
	}

	spdx := NewSPDX()
	if len(genopts.IgnorePatterns) > 0 {
		spdx.Options().IgnorePatterns = genopts.IgnorePatterns
	}
	spdx.Options().AnalyzeLayers = genopts.AnalyseLayers
	spdx.Options().ProcessGoModules = genopts.ProcessGoModules
	spdx.Options().ScanImages = genopts.ScanImages

	if !util.Exists(opts.WorkDir) {
		if err := os.MkdirAll(opts.WorkDir, os.FileMode(0o755)); err != nil {
			return nil, fmt.Errorf("creating builder worskpace dir: %w", err)
		}
	}

	// Create the new document
	doc = NewDocument()
	doc.Name = genopts.Name

	// If we do not have a namespace, we generate one
	// under the public SPDX URL defined in the spec.
	// (ref https://spdx.github.io/spdx-spec/document-creation-information/#65-spdx-document-namespace-field)
	if genopts.Namespace == "" {
		doc.Namespace = "https://spdx.org/spdxdocs/k8s-releng-bom-" + uuid.NewString()
	} else {
		doc.Namespace = genopts.Namespace
	}
	doc.Creator.Person = genopts.CreatorPerson
	doc.ExternalDocRefs = genopts.ExternalDocumentRef

	for _, dirPattern := range genopts.Directories {
		matches, err := filepath.Glob(dirPattern)
		if err != nil {
			return nil, err
		}
		for _, dirMatch := range matches {
			isFile, err := pathIsOfFile(dirMatch)
			if err != nil {
				return nil, fmt.Errorf("stat dir: %w", err)
			}
			if isFile {
				logrus.Debugf("Skipping %s because it's a file", dirMatch)
				continue
			}
			logrus.Infof("Processing directory %s", dirMatch)
			pkg, err := spdx.PackageFromDirectory(dirMatch)
			if err != nil {
				return nil, fmt.Errorf("generating package from directory: %w", err)
			}
			doc.ensureUniqueElementID(pkg)
			if err := doc.AddPackage(pkg); err != nil {
				return nil, fmt.Errorf("adding directory package to document: %w", err)
			}
		}
	}

	// Process all image references from registries
	for _, i := range genopts.Images {
		logrus.Infof("Processing image reference: %s", i)
		p, err := spdx.ImageRefToPackage(i)
		if err != nil {
			return nil, fmt.Errorf("generating SPDX package from image ref %s: %w", i, err)
		}
		doc.ensureUniqueElementID(p)
		doc.ensureUniquePeerIDs(p.GetRelationships())
		if err := doc.AddPackage(p); err != nil {
			return nil, fmt.Errorf("adding package to document: %w", err)
		}
	}

	// Process OCI image archives
	for _, tb := range genopts.Tarballs {
		logrus.Infof("Processing image archive %s", tb)
		p, err := spdx.PackageFromImageTarball(tb)
		if err != nil {
			return nil, fmt.Errorf("generating tarball package: %w", err)
		}
		doc.ensureUniqueElementID(p)
		doc.ensureUniquePeerIDs(p.GetRelationships())
		if err := doc.AddPackage(p); err != nil {
			return nil, fmt.Errorf("adding package to document: %w", err)
		}
	}

	// Add archive files as packages
	for _, tf := range genopts.Archives {
		logrus.Infof("Adding archive file as package: %s", tf)
		p, err := spdx.PackageFromArchive(tf)
		if err != nil {
			return nil, fmt.Errorf("creating spdx package from archive: %w", err)
		}
		doc.ensureUniqueElementID(p)
		doc.ensureUniquePeerIDs(p.GetRelationships())
		if err := doc.AddPackage(p); err != nil {
			return nil, fmt.Errorf("adding package to document: %w", err)
		}
	}

	// Process single files, not part of a package
	for _, filePattern := range genopts.Files {
		matches, err := filepath.Glob(filePattern)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			logrus.Warnf("%s pattern didn't match any file", filePattern)
		}
		for _, filePath := range matches {
			isFile, err := pathIsOfFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("stat file: %w", err)
			}
			if !isFile {
				continue
			}
			f, err := spdx.FileFromPath(filePath)
			if err != nil {
				return nil, fmt.Errorf("adding file: %w", err)
			}
			doc.ensureUniqueElementID(f)
			if err := doc.AddFile(f); err != nil {
				return nil, fmt.Errorf("adding file to document: %w", err)
			}
		}
	}
	return doc, nil
}

// TODO: Move this to https://github.com/kubernetes-sigs/release-utils/blob/main/util/common.go#L485
func pathIsOfFile(path string) (bool, error) {
	fInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return !fInfo.IsDir(), nil
}

// WriteDoc renders the document to a file
func (builder *defaultDocBuilderImpl) WriteDoc(doc *Document, path string) error {
	markup, err := doc.Render()
	if err != nil {
		return fmt.Errorf("generating document markup: %w", err)
	}
	logrus.Infof("writing document to %s", path)

	if err := os.WriteFile(path, []byte(markup), os.FileMode(0o644)); err != nil {
		return fmt.Errorf(
			"writing document markup to file: %w",
			err,
		)
	}
	return nil
}

// ReadYamlConfiguration reads a yaml configuration and
// set the values in an options struct
func (builder *defaultDocBuilderImpl) ReadYamlConfiguration(
	path string, opts *DocGenerateOptions,
) (err error) {
	yamldata, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading yaml SBOM configuration: %w", err)
	}

	conf := &YamlBOMConfiguration{}
	if err := yaml.Unmarshal(yamldata, conf); err != nil {
		return fmt.Errorf("unmarshalling SBOM configuration YAML: %w", err)
	}

	if conf.Name != "" {
		opts.Name = conf.Name
	}

	if conf.Namespace != "" {
		opts.Namespace = conf.Namespace
	}

	if conf.Creator.Person != "" {
		opts.CreatorPerson = conf.Creator.Person
	}

	if conf.License != "" {
		opts.License = conf.License
	}

	opts.ExternalDocumentRef = conf.ExternalDocRefs

	// Add all the artifacts
	for _, artifact := range conf.Artifacts {
		logrus.Infof("Configuration has artifact of type %s: %s", artifact.Type, artifact.Source)
		switch artifact.Type {
		case "directory":
			opts.Directories = append(opts.Directories, artifact.Source)
		case "image":
			opts.Images = append(opts.Images, artifact.Source)
		case "docker-archive":
			opts.Tarballs = append(opts.Tarballs, artifact.Source)
		case "file":
			opts.Files = append(opts.Files, artifact.Source)
		case "archive":
			opts.Archives = append(opts.Archives, artifact.Source)
		}
	}

	return nil
}
