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

// SHA1 is the currently accepted hash algorithm for SPDX documents, used for
// file integrity checks, NOT security.
// Instances of G401 and G505 can be safely ignored in this file.
//
// ref: https://github.com/spdx/spdx-spec/issues/11
//
//nolint:gosec
package spdx

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"text/template"

	purl "github.com/package-url/packageurl-go"
	"github.com/sirupsen/logrus"
)

const OptionVersionPattern PurlSearchOption = "VERSION_PATTERN"

var packageTemplate = `##### Package: {{ .Name }}

{{ if .Name }}PackageName: {{ .Name }}
{{ end -}}
{{ if .ID }}SPDXID: {{ .ID }}
{{ end -}}
{{- if .Checksum -}}
{{- range $key, $value := .Checksum -}}
{{ if . }}PackageChecksum: {{ $key }}: {{ $value }}
{{ end -}}
{{- end -}}
{{- end -}}
PackageDownloadLocation: {{ if .DownloadLocation }}{{ .DownloadLocation }}{{ else }}NONE{{ end }}
FilesAnalyzed: {{ .FilesAnalyzed }}
{{ if .Supplier -}}
{{- if .Supplier.Person }}PackageSupplier: Person: {{ .Supplier.Person }}
{{ end -}}
{{- if .Supplier.Organization }}PackageSupplier: Organization: {{ .Supplier.Organization }}
{{ end -}}
{{ end -}}
{{ if .VerificationCode }}PackageVerificationCode: {{ .VerificationCode }}
{{ end -}}
PackageLicenseConcluded: {{ if .LicenseConcluded }}{{ .LicenseConcluded }}{{ else }}NOASSERTION{{ end }}
{{ if .FileName }}PackageFileName: {{ .FileName }}
{{ end -}}
{{ if .LicenseInfoFromFiles }}{{- range $key, $value := .LicenseInfoFromFiles -}}PackageLicenseInfoFromFiles: {{ $value }}
{{ end -}}
{{ end -}}
{{ if .Version }}PackageVersion: {{ .Version }}
{{ end -}}
{{ if .HomePage }}PackageHomePage: {{ .HomePage }}
{{ end -}}
{{ if .PrimaryPurpose }}PrimaryPackagePurpose: {{ .PrimaryPurpose }}
{{ end -}}
{{ if .ExternalRefs }}{{- range $key, $value := .ExternalRefs -}}ExternalRef: {{ $value.Category }} {{ $value.Type }} {{ $value.Locator }}
{{ end -}}
{{ end -}}
{{ if .LicenseComments }}PackageLicenseComments: <text>{{ .LicenseComments }}
</text>
{{ end -}}
PackageLicenseDeclared: {{ if .LicenseDeclared }}{{ .LicenseDeclared }}{{ else }}NOASSERTION{{ end }}
PackageCopyrightText: {{ if .CopyrightText }}<text>{{ .CopyrightText }}
</text>{{ else }}NOASSERTION{{ end }}

`

// Package groups a set of files
type Package struct {
	Entity
	sync.RWMutex
	FilesAnalyzed        bool     // true
	VerificationCode     string   // 6486e016b01e9ec8a76998cefd0705144d869234
	LicenseInfoFromFiles []string // GPL-3.0-or-later
	LicenseDeclared      string   // GPL-3.0-or-later
	Version              string   // Package version
	Comment              string   // a place for the SPDX document creator to record any general comments
	HomePage             string   // A web site that serves as the package home page
	PrimaryPurpose       string   // Estimate of the most likely package usage

	// Supplier: the actual distribution source for the package/directory
	Supplier struct {
		Person       string // person name and optional (<email>)
		Organization string // organization name and optional (<email>)
	}
	// Originator: For example, the SPDX file identifies the package glibc and Red Hat as the Package Supplier,
	// but the Free Software Foundation is the Package Originator.
	Originator struct {
		Person       string // person name and optional (<email>)
		Organization string // organization name and optional (<email>)
	}

	ExternalRefs []ExternalRef // List of external references
}

// PackagePurposes lists the valid package purposes
// https://spdx.github.io/spdx-spec/v2.3/package-information/#724-primary-package-purpose-field
var PackagePurposes = []string{
	"APPLICATION", "FRAMEWORK", "LIBRARY", "CONTAINER", "OPERATING-SYSTEM",
	"DEVICE", "FIRMWARE", "SOURCE", "ARCHIVE", "FILE", "INSTALL", "OTHER",
}

var ExternalRefCategories = map[string][]string{
	"SECURITY":        {"cpe22Type", "cpe23Type", "advisory", "fix", "url", "swid"},
	"PACKAGE_MANAGER": {"maven-central", "npm", "nuget", "bower", "purl"},
	"PACKAGE-MANAGER": {"maven-central", "npm", "nuget", "bower", "purl"},
	"PERSISTENT-ID":   {"swh", "gitoid"},
	"PERSISTENT_ID":   {"swh", "gitoid"},
	"OTHER":           {},
}

func NewPackage() (p *Package) {
	p = &Package{}
	p.Entity.Opts = &ObjectOptions{}
	return p
}

// AddFile adds a file contained in the package
func (p *Package) AddFile(file *File) error {
	p.Lock()
	defer p.Unlock()

	// If file does not have an ID, we try to build one
	// by hashing the file name
	if file.ID == "" {
		if file.Name == "" {
			return errors.New("unable to generate file ID, filename not set")
		}
		if p.Name == "" {
			return errors.New("unable to generate file ID, package not set")
		}
		h := sha1.New()
		if _, err := h.Write([]byte(p.Name + ":" + file.Name)); err != nil {
			return fmt.Errorf("getting sha1 of filename: %w", err)
		}
		file.BuildID(fmt.Sprintf("%x", h.Sum(nil)))
	}

	// Add the file to the package's relationships
	p.AddRelationship(&Relationship{
		FullRender: true,
		Type:       CONTAINS,
		Peer:       file,
	})

	return nil
}

// AddPackage adds a new subpackage to a package
func (p *Package) AddPackage(pkg *Package) error {
	p.AddRelationship(&Relationship{
		Peer:       pkg,
		Type:       CONTAINS,
		FullRender: true,
	})
	return nil
}

// AddDependency adds a new subpackage as a dependency
func (p *Package) AddDependency(pkg *Package) error {
	p.AddRelationship(&Relationship{
		Peer:       pkg,
		Type:       DEPENDS_ON,
		FullRender: true,
	})
	return nil
}

// Files returns all contained files in the package
func (p *Package) Files() []*File {
	ret := []*File{}
	for _, rel := range p.Relationships {
		if rel.Peer != nil {
			if p, ok := rel.Peer.(*File); ok {
				ret = append(ret, p)
			}
		}
	}
	return ret
}

// ComputeVerificationCode calculates the package verification
// code according to the SPDX spec
func (p *Package) ComputeVerificationCode() error {
	files := p.Files()
	p.VerificationCode = ""

	// If files where not analyzed, the code is not required
	if !p.FilesAnalyzed {
		return nil
	}

	// If there are no files, the code is not required
	if len(files) == 0 {
		return nil
	}
	shaList := []string{}
	for _, f := range files {
		if f.Checksum == nil {
			return fmt.Errorf("unable to render package, file has no checksums")
		}
		if _, ok := f.Checksum["SHA1"]; !ok {
			return fmt.Errorf(
				"unable to render package, files were analyzed but some do not have sha1 checksums",
			)
		}
		shaList = append(shaList, f.Checksum["SHA1"])
	}

	// Sort the strings:
	sort.Strings(shaList)
	h := sha1.New()
	if _, err := h.Write([]byte(strings.Join(shaList, ""))); err != nil {
		return fmt.Errorf("getting SHA1 verification of files: %w", err)
	}
	p.VerificationCode = fmt.Sprintf("%x", h.Sum(nil))
	return nil
}

// ComputeLicenseListComputes the license list from the
// files contained in the package
func (p *Package) ComputeLicenseList() error {
	p.LicenseInfoFromFiles = []string{}
	if !p.FilesAnalyzed {
		return nil
	}

	files := p.Files()
	if len(files) == 0 {
		return fmt.Errorf("unable to compute license list, package has no files")
	}

	filesTagList := []string{}
	for _, f := range files {
		// Collect the license tags
		if f.LicenseInfoInFile != "" {
			collected := false
			for _, tag := range filesTagList {
				if tag == f.LicenseInfoInFile {
					collected = true
					break
				}
			}
			if !collected {
				filesTagList = append(filesTagList, f.LicenseInfoInFile)
			}
		}
	}

	for _, tag := range filesTagList {
		if tag != NONE && tag != NOASSERTION {
			p.LicenseInfoFromFiles = append(p.LicenseInfoFromFiles, tag)
		}
	}

	// If no license tags where collected from files, then the SBOM has
	// to express "NONE" in the LicenseInfoFromFiles section to be compliant:
	if len(filesTagList) == 0 {
		p.LicenseInfoFromFiles = append(p.LicenseInfoFromFiles, NONE)
	}

	return nil
}

// Render renders the document fragment of the package
func (p *Package) Render() (docFragment string, err error) {
	// First thing, check all relationships
	if len(p.Relationships) > 0 {
		logrus.Infof("Package %s has %d relationships defined", p.SPDXID(), len(p.Relationships))
		if err := p.CheckRelationships(); err != nil {
			return "", fmt.Errorf("checking package relationships: %w", err)
		}
	}

	var buf bytes.Buffer
	tmpl, err := template.New("package").Parse(packageTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing package template: %w", err)
	}

	if p.FilesAnalyzed {
		// If files were analyzed, calculate the verification which
		// is a sha1sum from all sha1 checksum from contained files.
		if err := p.ComputeVerificationCode(); err != nil {
			return "", fmt.Errorf("computing verification code: %w", err)
		}

		// Extract the ltest license tags from the contained files
		// these MUST be listed in the LicenseInfoFromFiles tag
		if err := p.ComputeVerificationCode(); err != nil {
			return "", fmt.Errorf("computing verification code: %w", err)
		}
	}

	// Run the template to verify the output.
	if err := tmpl.Execute(&buf, p); err != nil {
		return "", fmt.Errorf("executing spdx package template: %w", err)
	}

	docFragment = buf.String()

	// Add the output from all related files
	for _, rel := range p.Relationships {
		fragment, err := rel.Render(p)
		if err != nil {
			return "", fmt.Errorf("rendering relationship: %w", err)
		}
		docFragment += fragment
	}
	docFragment += "\n"
	return docFragment, nil
}

// CheckRelationships ensures al linked relationships are complete
// before rendering.
func (p *Package) CheckRelationships() error {
	for _, related := range p.Relationships {
		if related.Peer != nil {
			if related.Peer.SPDXID() == "" {
				related.Peer.BuildID()
			}
		}
	}
	return nil
}

// BuildID sets the file ID, optionally from a series of strings
func (p *Package) BuildID(seeds ...string) {
	prefix := ""
	if p.Options() != nil {
		if p.Options().Prefix != "" {
			prefix = "-" + p.Options().Prefix
		}
	}
	p.Entity.BuildID(append([]string{"SPDXRef-Package" + prefix}, seeds...)...)
}

func (p *Package) SetEntity(e *Entity) {
	p.Entity = *e
}

func (p *Package) drawTitle(o *DrawingOptions) string {
	title := p.SPDXID()
	if o.Purls && p.Purl() != nil && p.Purl().Name != "" {
		title = p.Purl().String()
	} else if p.Name != "" {
		title = p.Name
		if o.Version && p.Version != "" {
			title += "@" + p.Version
		}
	}
	return title
}

// drawName returns the name string to be used in the outline
func (p *Package) drawName(o *DrawingOptions) string {
	name := p.SPDXID()
	if o.Purls && p.Purl() != nil && p.Purl().Name != "" {
		name = p.Purl().String()
	} else if p.Name != "" {
		name = p.Name
		if o.Version && p.Version != "" {
			name = name + "@" + p.Version
		}
	}
	return name
}

// Draw renders the package data as a tree-like structure
//
//nolint:gocritic
func (p *Package) Draw(builder *strings.Builder, o *DrawingOptions, depth int, seen *map[string]struct{}) {
	(*seen)[p.SPDXID()] = struct{}{}

	title := p.drawTitle(o)
	if !o.SkipName {
		fmt.Fprintln(builder, treeLines(o, depth-1, connectorT)+title)
	}

	connector := ""
	if len(p.Relationships) == 0 || (o.Recursion > 0 && depth >= o.Recursion) {
		connector = connectorL
	}

	fmt.Fprintf(builder, treeLines(o, depth, connector)+"🔗 %d Relationships\n", len(p.Relationships))
	if depth >= o.Recursion && o.Recursion > 0 {
		fmt.Fprintln(builder, treeLines(o, depth-1, ""))
		return
	}

	i := 0
	for _, rel := range p.Relationships {
		i++
		o.LastItem = true
		if i < len(p.Relationships) {
			o.LastItem = false
		}

		connector = connectorT
		if o.LastItem {
			connector = connectorL
		}
		line := treeLines(o, depth, connector)
		if rel.Peer != nil {
			name := rel.Peer.SPDXID()
			etype := ""

			if !o.OnlyIDs {
				if _, ok := rel.Peer.(*Package); ok {
					name = rel.Peer.(*Package).drawName(o)
					etype = "PACKAGE"
				}

				if _, ok := rel.Peer.(*File); ok {
					name = rel.Peer.(*File).Name
					etype = "FILE"
				}
			}
			line += fmt.Sprintf("%s %s %s", rel.Type, etype, name)
		} else {
			line += fmt.Sprintf("%s %s", rel.Type, rel.PeerReference)
		}

		// If the peer is external, state it
		if rel.PeerExtReference != "" {
			line += " (external)"
		}

		// If it is a file, print the name
		if _, ok := rel.Peer.(*File); ok {
			if rel.Peer.(*File).Name != "" {
				line += fmt.Sprintf(" (%s)", rel.Peer.(*File).Name)
			}
		}
		if o.Width > 0 && len(line) > o.Width {
			line = line[:o.Width]
		}
		fmt.Fprintln(builder, line)

		// If the child has relationships, dig in
		if rel.Peer != nil {
			if _, ok := (*seen)[rel.Peer.SPDXID()]; !ok {
				// if the child is a package:
				if _, ok := rel.Peer.(*Package); ok {
					o.SkipName = true
					if len(rel.Peer.(*Package).Relationships) > 0 {
						rel.Peer.(*Package).Draw(builder, o, depth+1, seen)
					}
				}

				// If the child is a file:
				if _, ok := rel.Peer.(*File); ok {
					o.SkipName = false
					if len(rel.Peer.(*File).Relationships) > 0 {
						rel.Peer.(*File).Draw(builder, o, depth+1, seen)
					}
				}
			}
		}
		if i == len(p.Relationships) {
			fmt.Fprintln(builder, treeLines(o, depth-1, ""))
		}
	}
}

// ReadSourceFile reads a file from the filesystem and assigns its properties
// to the package metadata
func (p *Package) ReadSourceFile(path string) error {
	if err := p.Entity.ReadSourceFile(path); err != nil {
		return err
	}
	if p.SPDXID() == "" {
		p.BuildID()
	}
	return nil
}

// GetElementByID search the package and its peers looking for the specified SPDX
// id. If found, the function returns a copy of the object
func (p *Package) GetElementByID(id string) Object {
	if p.SPDXID() == id {
		return p
	}
	return recursiveIDSearch(id, p, &map[string]struct{}{})
}

// Purl searches the external refs in the package and returns
// a parsed purl if it finds a purl PACKAGE_MANAGER extref:
func (p *Package) Purl() *purl.PackageURL {
	if p.ExternalRefs == nil {
		return nil
	}
	purlString := ""
	for _, er := range p.ExternalRefs {
		if (er.Category == "PACKAGE-MANAGER" || er.Category == "PACKAGE_MANAGER") && er.Type == "purl" {
			purlString = er.Locator
		}
	}
	if purlString == "" {
		return nil
	}
	// Parse the purl
	purlObject, err := purl.FromString(purlString)
	if err != nil {
		logrus.Warnf("Invalid purl in package %s: %s", p.SPDXID(), purlString)
		return nil
	}
	return &purlObject
}

type PurlSearchOption string

// PurlMatches gets a spec url and returns true if its defined parts
// match the analog parts in the package's purl
func (p *Package) PurlMatches(spec *purl.PackageURL, _ ...PurlSearchOption) bool {
	pkgPurl := p.Purl()
	if pkgPurl == nil {
		return false
	}

	if spec.Type != "*" && spec.Type != pkgPurl.Type {
		return false
	}
	if spec.Namespace != "*" && spec.Namespace != pkgPurl.Namespace {
		return false
	}
	if spec.Name != "*" && spec.Name != pkgPurl.Name {
		return false
	}
	if spec.Version != "*" && spec.Version != pkgPurl.Version {
		return false
	}
	if spec.Subpath != "*" && spec.Subpath != pkgPurl.Subpath {
		return false
	}

	// Compare the qualifiers
	specQs := spec.Qualifiers.Map()
	pkgQs := pkgPurl.Qualifiers.Map()

	for k := range specQs {
		if _, ok := pkgQs[k]; !ok {
			return false
		}
		if specQs[k] != pkgQs[k] {
			return false
		}
	}
	return true
}
