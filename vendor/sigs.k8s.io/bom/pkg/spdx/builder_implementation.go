/*
Copyright 2023 The Kubernetes Authors.

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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"sigs.k8s.io/bom/pkg/license"
	"sigs.k8s.io/release-utils/util"
)

type DocBuilderImplementation interface {
	WriteDoc(*Document, string) error
	ReadYamlConfiguration(string, *DocGenerateOptions) error
	CreateSPDXClient(*DocGenerateOptions, *DocBuilderOptions) (*SPDX, error)
	ValidateOptions(*DocGenerateOptions) error

	// Document generation functions
	CreateDocument(*DocGenerateOptions, *SPDX) (*Document, error)
	ScanDirectories(*DocGenerateOptions, *SPDX, *Document) error
	ScanImages(*DocGenerateOptions, *SPDX, *Document) error
	ScanImageArchives(*DocGenerateOptions, *SPDX, *Document) error
	ScanArchives(*DocGenerateOptions, *SPDX, *Document) error
	ScanFiles(*DocGenerateOptions, *SPDX, *Document) error
}

// defaultDocBuilderImpl is the default implementation for the
// SPDX document builder
type defaultDocBuilderImpl struct {
	format Format
}

func (builder *defaultDocBuilderImpl) CreateDocument(genopts *DocGenerateOptions, _ *SPDX) (*Document, error) {
	// Create the new document
	doc := NewDocument()
	doc.Name = genopts.Name
	doc.LicenseListVersion = strings.TrimPrefix(license.DefaultCatalogOpts.Version, "v")
	if genopts.LicenseListVersion != "" {
		doc.LicenseListVersion = strings.TrimPrefix(genopts.LicenseListVersion, "v")
	}

	// If we do not have a namespace, we generate one under the public SPDX
	// URL as defined in the spec.
	// (ref https://spdx.github.io/spdx-spec/document-creation-information/#65-spdx-document-namespace-field)
	doc.Namespace = genopts.Namespace
	if genopts.Namespace == "" {
		doc.Namespace = "https://spdx.org/spdxdocs/k8s-releng-bom-" + uuid.NewString()
	}

	doc.Creator.Person = genopts.CreatorPerson
	doc.ExternalDocRefs = genopts.ExternalDocumentRef
	return doc, nil
}

func (builder *defaultDocBuilderImpl) CreateSPDXClient(genopts *DocGenerateOptions, opts *DocBuilderOptions) (*SPDX, error) {
	spdx := NewSPDX()
	if len(genopts.IgnorePatterns) > 0 {
		spdx.Options().IgnorePatterns = genopts.IgnorePatterns
	}
	spdx.Options().AnalyzeLayers = genopts.AnalyseLayers
	spdx.Options().ProcessGoModules = genopts.ProcessGoModules
	spdx.Options().ScanImages = genopts.ScanImages
	spdx.Options().LicenseListVersion = genopts.LicenseListVersion

	if !util.Exists(opts.WorkDir) {
		if err := os.MkdirAll(opts.WorkDir, os.FileMode(0o755)); err != nil {
			return nil, fmt.Errorf("creating builder worskpace dir: %w", err)
		}
	}
	return spdx, nil
}

func (builder *defaultDocBuilderImpl) ScanDirectories(genopts *DocGenerateOptions, spdx *SPDX, doc *Document) error {
	for _, dirPattern := range genopts.Directories {
		matches, err := filepath.Glob(dirPattern)
		if err != nil {
			return fmt.Errorf("globbing directory pattern: %w", err)
		}
		for _, dirMatch := range matches {
			isFile, err := pathIsOfFile(dirMatch)
			if err != nil {
				return fmt.Errorf("stat dir: %w", err)
			}
			if isFile {
				logrus.Debugf("Skipping %s because it's a file", dirMatch)
				continue
			}
			logrus.Infof("Processing directory %s", dirMatch)
			pkg, err := spdx.PackageFromDirectory(dirMatch)
			if err != nil {
				return fmt.Errorf("generating package from directory: %w", err)
			}
			doc.ensureUniqueElementID(pkg)
			if err := doc.AddPackage(pkg); err != nil {
				return fmt.Errorf("adding directory package to document: %w", err)
			}
		}
	}
	return nil
}

func (builder *defaultDocBuilderImpl) ScanImages(genopts *DocGenerateOptions, spdx *SPDX, doc *Document) error {
	// Process all image references from registries
	for _, i := range genopts.Images {
		logrus.Infof("Processing image reference: %s", i)
		p, err := spdx.ImageRefToPackage(i)
		if err != nil {
			return fmt.Errorf("generating SPDX package from image ref %s: %w", i, err)
		}
		doc.ensureUniqueElementID(p)
		doc.ensureUniquePeerIDs(p.GetRelationships())
		if err := doc.AddPackage(p); err != nil {
			return fmt.Errorf("adding package to document: %w", err)
		}
	}
	return nil
}

func (builder *defaultDocBuilderImpl) ScanImageArchives(genopts *DocGenerateOptions, spdx *SPDX, doc *Document) error {
	// Process OCI image archives
	for _, tb := range genopts.Tarballs {
		logrus.Infof("Processing image archive %s", tb)
		p, err := spdx.PackageFromImageTarball(tb)
		if err != nil {
			return fmt.Errorf("generating tarball package: %w", err)
		}
		doc.ensureUniqueElementID(p)
		doc.ensureUniquePeerIDs(p.GetRelationships())
		if err := doc.AddPackage(p); err != nil {
			return fmt.Errorf("adding package to document: %w", err)
		}
	}
	return nil
}

func (builder *defaultDocBuilderImpl) ScanArchives(genopts *DocGenerateOptions, spdx *SPDX, doc *Document) error {
	// Add archive files as packages
	for _, tf := range genopts.Archives {
		logrus.Infof("Adding archive file as package: %s", tf)
		p, err := spdx.PackageFromArchive(tf)
		if err != nil {
			return fmt.Errorf("creating spdx package from archive: %w", err)
		}
		doc.ensureUniqueElementID(p)
		doc.ensureUniquePeerIDs(p.GetRelationships())
		if err := doc.AddPackage(p); err != nil {
			return fmt.Errorf("adding package to document: %w", err)
		}
	}
	return nil
}

func (builder *defaultDocBuilderImpl) ScanFiles(genopts *DocGenerateOptions, spdx *SPDX, doc *Document) error {
	// Process single files, not part of a package
	for _, filePattern := range genopts.Files {
		matches, err := filepath.Glob(filePattern)
		if err != nil {
			return fmt.Errorf("globing files from expression: %w", err)
		}
		if len(matches) == 0 {
			logrus.Warnf("%s pattern didn't match any file", filePattern)
		}
		for _, filePath := range matches {
			isFile, err := pathIsOfFile(filePath)
			if err != nil {
				return fmt.Errorf("stat file: %w", err)
			}
			if !isFile {
				continue
			}
			f, err := spdx.FileFromPath(filePath)
			if err != nil {
				return fmt.Errorf("creating SPDX file: %w", err)
			}
			doc.ensureUniqueElementID(f)
			if err := doc.AddFile(f); err != nil {
				return fmt.Errorf("adding file to document: %w", err)
			}
		}
	}
	return nil
}

// ReadYamlConfiguration reads a yaml configuration and
// set the values in an options struct
func (builder *defaultDocBuilderImpl) ReadYamlConfiguration(
	path string, genopts *DocGenerateOptions,
) (err error) {
	// NOOP if no YAML file is specified
	if path == "" {
		return nil
	}

	yamldata, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading yaml SBOM configuration: %w", err)
	}

	conf := &YamlBOMConfiguration{}
	if err := yaml.Unmarshal(yamldata, conf); err != nil {
		return fmt.Errorf("unmarshalling SBOM configuration YAML: %w", err)
	}

	if conf.Name != "" {
		genopts.Name = conf.Name
	}

	if conf.Namespace != "" {
		genopts.Namespace = conf.Namespace
	}

	if conf.Creator.Person != "" {
		genopts.CreatorPerson = conf.Creator.Person
	}

	if conf.License != "" {
		genopts.License = conf.License
	}

	genopts.ExternalDocumentRef = conf.ExternalDocRefs

	// Add all the artifacts
	for _, artifact := range conf.Artifacts {
		logrus.Infof("Configuration has artifact of type %s: %s", artifact.Type, artifact.Source)
		switch artifact.Type {
		case "directory":
			genopts.Directories = append(genopts.Directories, artifact.Source)
		case "image":
			genopts.Images = append(genopts.Images, artifact.Source)
		case "docker-archive":
			genopts.Tarballs = append(genopts.Tarballs, artifact.Source)
		case "file":
			genopts.Files = append(genopts.Files, artifact.Source)
		case "archive":
			genopts.Archives = append(genopts.Archives, artifact.Source)
		}
	}

	return nil
}

func (builder *defaultDocBuilderImpl) ValidateOptions(genopts *DocGenerateOptions) error {
	if err := genopts.Validate(); err != nil {
		return fmt.Errorf("checking build options: %w", err)
	}
	return nil
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
