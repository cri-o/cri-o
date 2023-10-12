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
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	intoto "github.com/in-toto/in-toto-golang/in_toto"
	purl "github.com/package-url/packageurl-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/term"

	"sigs.k8s.io/bom/pkg/provenance"
	"sigs.k8s.io/release-utils/hash"
	"sigs.k8s.io/release-utils/util"
	"sigs.k8s.io/release-utils/version"
)

var docTemplate = `{{ if .Version }}SPDXVersion: {{.Version}}
{{ end -}}
DataLicense: CC0-1.0
{{ if .ID }}SPDXID: {{ .ID }}
{{ end -}}
{{ if .Name }}DocumentName: {{ .Name }}
{{ end -}}
{{ if .Namespace }}DocumentNamespace: {{ .Namespace }}
{{ end -}}
{{- if .ExternalDocRefs -}}
{{- range $key, $value := .ExternalDocRefs -}}
ExternalDocumentRef:{{ extDocFormat $value }}
{{ end -}}
{{- end -}}
{{ if .Creator -}}
{{- if .Creator.Person }}Creator: Person: {{ .Creator.Person }}
{{ end -}}
{{- if .Creator.Organization }}Creator: Organization: {{ .Creator.Organization }}
{{ end -}}
{{- if .Creator.Tool -}}
{{- range $key, $value := .Creator.Tool }}Creator: Tool: {{ $value }}
{{ end -}}
{{- end -}}
{{ end -}}
{{ if .LicenseListVersion }}LicenseListVersion: {{ .LicenseListVersion }}
{{ end -}}
{{ if .Created }}Created: {{ dateFormat .Created }}
{{ end }}

`

const (
	connectorL          = "└"
	connectorT          = "├"
	MessageHashMismatch = "Hash mismatch"
)

// Document abstracts the SPDX document
type Document struct {
	Version     string // SPDX-2.2
	DataLicense string // CC0-1.0
	ID          string // SPDXRef-DOCUMENT
	Name        string // hello-go-src
	Namespace   string // https://swinslow.net/spdx-examples/example6/hello-go-src-v1
	Creator     struct {
		Person       string // Steve Winslow (steve@swinslow.net)
		Organization string
		Tool         []string // github.com/spdx/tools-golang/builder
	}
	Created            time.Time // 2020-11-24T01:12:27Z
	LicenseListVersion string
	Packages           map[string]*Package
	Files              map[string]*File      // List of files
	ExternalDocRefs    []ExternalDocumentRef // List of related external documents
}

// ExternalDocumentRef is a pointer to an external, related document
type ExternalDocumentRef struct {
	ID        string            `yaml:"id"`        // Identifier for the external doc (eg "external-source-bom")
	URI       string            `yaml:"uri"`       // URI where the doc can be retrieved
	Checksums map[string]string `yaml:"checksums"` // Document checksums
}

// Example: cpe23Type cpe:2.3:a:base-files:base-files:10.3+deb10u9:*:*:*:*:*:*:*
type ExternalRef struct {
	Category string // SECURITY | PACKAGE-MANAGER | PERSISTENT-ID | OTHER
	Type     string // cpe22Type | cpe23Type | maven-central | npm | nuget | bower | purl | swh | other
	Locator  string // unique string with no spaces
}

type DrawingOptions struct {
	Width       int
	Height      int
	Recursion   int
	DisableTerm bool
	LastItem    bool
	SkipName    bool
	OnlyIDs     bool
	ASCIIOnly   bool
	Purls       bool
	Version     bool
}

// String returns the SPDX string of the external document ref
func (ed *ExternalDocumentRef) String() string {
	if len(ed.Checksums) == 0 || ed.ID == "" || ed.URI == "" {
		return ""
	}
	var csAlgo, csHash string
	for csAlgo, csHash = range ed.Checksums {
		break
	}

	return fmt.Sprintf("DocumentRef-%s %s %s: %s", ed.ID, ed.URI, csAlgo, csHash)
}

// ReadSourceFile populates the external reference data (the sha256 checksum)
// from a given path
func (ed *ExternalDocumentRef) ReadSourceFile(path string) error {
	if ed.Checksums == nil {
		ed.Checksums = map[string]string{}
	}
	// The SPDX validator tools are broken and cannot validate non SHA1 checksums
	// ref https://github.com/spdx/tools-java/issues/21
	val, err := hash.SHA1ForFile(path)
	if err != nil {
		return fmt.Errorf("while calculating the sha256 checksum of the external reference: %w", err)
	}
	ed.Checksums["SHA1"] = val
	return nil
}

// NewDocument returns a new SPDX document with some defaults preloaded
func NewDocument() *Document {
	return &Document{
		ID:          "SPDXRef-DOCUMENT",
		Version:     "SPDX-2.3",
		DataLicense: "CC0-1.0",
		Created:     time.Now().UTC(),
		Creator: struct {
			Person       string
			Organization string
			Tool         []string
		}{
			Person:       defaultDocumentAuthor,
			Organization: "Kubernetes Release Engineering",
			Tool: []string{
				fmt.Sprintf("%s-%s", "bom", version.GetVersionInfo().GitVersion),
			},
		},
	}
}

// AddPackage adds a new empty package to the document
func (d *Document) AddPackage(pkg *Package) error {
	if d.Packages == nil {
		d.Packages = map[string]*Package{}
	}

	if pkg.SPDXID() == "" {
		pkg.BuildID(pkg.Name)
		d.ensureUniqueElementID(pkg)
	}
	if pkg.SPDXID() == "" {
		return errors.New("package ID is needed to add a new package")
	}
	if _, ok := d.Packages[pkg.SPDXID()]; ok {
		return fmt.Errorf("a package with ID %s already exists in the document", pkg.SPDXID())
	}

	d.Packages[pkg.SPDXID()] = pkg
	return nil
}

// Write outputs the SPDX document into a file
func (d *Document) Write(path string) error {
	content, err := d.Render()
	if err != nil {
		return fmt.Errorf("rendering SPDX code: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), os.FileMode(0o644)); err != nil {
		return fmt.Errorf("writing SPDX code to file: %w", err)
	}
	logrus.Infof("SPDX SBOM written to %s", path)
	return nil
}

// Render reders the spdx manifest
func (d *Document) Render() (doc string, err error) {
	var buf bytes.Buffer
	funcMap := template.FuncMap{
		// The name "title" is what the function will be called in the template text.
		"dateFormat":   func(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") },
		"extDocFormat": func(ed ExternalDocumentRef) string { logrus.Infof("External doc: %s", ed.ID); return ed.String() },
	}

	if d.Name == "" {
		d.Name = "SBOM-SPDX-" + uuid.New().String()
		logrus.Warnf("Document has no name defined, automatically set to " + d.Name)
	}

	tmpl, err := template.New("document").Funcs(funcMap).Parse(docTemplate)
	if err != nil {
		log.Fatalf("parsing: %s", err)
	}

	// Run the template to verify the output.
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", fmt.Errorf("executing spdx document template: %w", err)
	}

	doc = buf.String()

	// List files in the document. Files listed directly on the
	// document do not contain relationships yet.
	filesDescribed := ""
	if len(d.Files) > 0 {
		doc += "\n##### Files independent of packages\n\n"
		filesDescribed = "\n"
	}

	for _, file := range d.Files {
		fileDoc, err := file.Render()
		if err != nil {
			return "", fmt.Errorf("rendering file "+file.Name+" :%w", err)
		}
		doc += fileDoc
		filesDescribed += fmt.Sprintf("Relationship: %s DESCRIBES %s\n\n", d.ID, file.ID)
	}
	doc += filesDescribed

	// Cycle all packages and get their data
	for _, pkg := range d.Packages {
		pkgDoc, err := pkg.Render()
		if err != nil {
			return "", fmt.Errorf("rendering pkg "+pkg.Name+" :%w", err)
		}

		doc += pkgDoc
		doc += fmt.Sprintf("Relationship: %s DESCRIBES %s\n\n", d.ID, pkg.ID)
	}

	return doc, err
}

// AddFile adds a file contained in the package
func (d *Document) AddFile(file *File) error {
	if d.Files == nil {
		d.Files = map[string]*File{}
	}
	// If file does not have an ID, we try to build one
	// by hashing the file name
	if file.ID == "" {
		if file.Name == "" {
			return errors.New("unable to generate file ID, filename not set")
		}
		if d.Name == "" {
			return errors.New("unable to generate file ID, filename not set")
		}
		h := sha1.New()
		if _, err := h.Write([]byte(d.Name + ":" + file.Name)); err != nil {
			return fmt.Errorf("getting sha1 of filename: %w", err)
		}
		file.ID = "SPDXRef-File-" + fmt.Sprintf("%x", h.Sum(nil))
	}
	d.ensureUniqueElementID(file)
	d.Files[file.ID] = file
	return nil
}

func treeLines(o *DrawingOptions, depth int, connector string) string {
	stick := "│"
	if o.ASCIIOnly {
		stick = "|"
	}
	if connector == "" {
		connector = stick
	}
	res := " " + strings.Repeat(fmt.Sprintf(" %s ", stick), depth)
	res += " " + connector + " "
	return res
}

// Outline draws an outline of the relationships inside the doc
func (d *Document) Outline(o *DrawingOptions) (outline string, err error) {
	seen := map[string]struct{}{}
	builder := &strings.Builder{}
	title := d.ID
	if d.Name != "" {
		title = d.Name
	}
	fmt.Fprintf(builder, " 📂 SPDX Document %s\n", title)
	fmt.Fprintln(builder, treeLines(o, 0, ""))
	var width, height int
	if term.IsTerminal(0) {
		width, height, err = term.GetSize(0)
		if err != nil {
			return "", fmt.Errorf("reading the terminal size: %w", err)
		}
		logrus.Debugf("Terminal size is %dx%d", width, height)
	}
	o.Width = width
	o.Height = height

	fmt.Fprintf(builder, treeLines(o, 0, "")+"📦 DESCRIBES %d Packages\n", len(d.Packages))
	fmt.Fprintln(builder, treeLines(o, 0, ""))
	i := 0
	for _, p := range d.Packages {
		i++
		o.LastItem = true
		if i < len(d.Packages) {
			o.LastItem = false
		}
		o.SkipName = false
		p.Draw(builder, o, 1, &seen)
	}
	if len(d.Files) > 0 {
		fmt.Fprintln(builder, treeLines(o, 0, ""))
	}
	connector := "│"
	if len(d.Files) == 0 {
		connector = connectorL
	}
	fmt.Fprintf(builder, treeLines(o, 0, connector)+"📄 DESCRIBES %d Files\n", len(d.Files))
	if len(d.Files) > 0 {
		fmt.Fprint(builder, treeLines(o, 0, ""))
	}
	fmt.Fprintln(builder, "")
	i = 0

	for _, f := range d.Files {
		i++
		o.LastItem = true
		if i < len(d.Files) {
			o.LastItem = false
		}
		f.Draw(builder, o, 0, &seen)
	}
	return builder.String(), nil
}

type ProvenanceOptions struct {
	Relationships map[string][]RelationshipType
}

// DefaultProvenanceOptions we consider examples and dependencies as not part of the doc
var DefaultProvenanceOptions = &ProvenanceOptions{
	Relationships: map[string][]RelationshipType{
		"include": {},
		"exclude": {
			EXAMPLE_OF,
			DEPENDS_ON,
		},
	},
}

func (d *Document) ToProvenanceStatement(opts *ProvenanceOptions) *provenance.Statement {
	statement := provenance.NewSLSAStatement()
	subs := []intoto.Subject{}
	seen := &map[string]struct{}{}

	for _, p := range d.Packages {
		subsubs := p.getProvenanceSubjects(opts, seen)
		subs = append(subs, subsubs...)
	}

	for _, f := range d.Files {
		subsubs := f.getProvenanceSubjects(opts, seen)
		subs = append(subs, subsubs...)
	}
	statement.Subject = subs
	return statement
}

// WriteProvenanceStatement writes the sbom as an in-toto provenance statement
func (d *Document) WriteProvenanceStatement(opts *ProvenanceOptions, path string) error {
	statement := d.ToProvenanceStatement(opts)
	data, err := json.Marshal(statement)
	if err != nil {
		return fmt.Errorf("serializing statement to json: %w", err)
	}

	if err := os.WriteFile(path, data, os.FileMode(0o644)); err != nil {
		return fmt.Errorf(
			"writing sbom as provenance statement: %w",
			err,
		)
	}
	return nil
}

// ensureUniquePackageID takes a string and checks if
// there is another string with the same name in the document.
// If there is one, it will append a digit until a unique name
// is found.
func (d *Document) ensureUniqueElementID(o Object) {
	newID := o.SPDXID()
	i := 0
	for {
		// Check if there us already an element with the same ID
		if el := d.GetElementByID(newID); el == nil {
			if o.SPDXID() != newID {
				logrus.Infof(
					"Element name changed from %s to %s to ensure it is unique",
					o.SPDXID(), newID,
				)
			}
			o.SetSPDXID(newID)
			break
		}
		i++
		newID = fmt.Sprintf("%s-%04d", o.SPDXID(), i)
	}
}

// ensureUniquePeerIDs gets a relationship collection and ensures all peers
// have unique IDs
func (d *Document) ensureUniquePeerIDs(rels *[]*Relationship) {
	// First, ensure peer names are unique among themselves
	seen := map[string]struct{}{}
	for _, rel := range *rels {
		if rel.Peer == nil || rel.Peer.SPDXID() == "" {
			continue
		}
		testName := rel.Peer.SPDXID()
		i := 0
		for {
			if _, ok := seen[testName]; !ok {
				rel.Peer.SetSPDXID(testName)
				seen[testName] = struct{}{}
				break
			}
			i++
			testName = fmt.Sprintf("%s-%04d", rel.Peer.SPDXID(), i)
		}
	}

	// And then check against the document
	for _, rel := range *rels {
		if rel.Peer == nil {
			continue
		}
		d.ensureUniqueElementID(rel.Peer)
	}
}

// GetPackageByID queries the packages to search for a specific entity by name
// note that this method returns a copy of the entity if found.
func (d *Document) GetElementByID(id string) Object {
	seen := map[string]struct{}{}
	for _, p := range d.Packages {
		if sub := recursiveIDSearch(id, p, &seen); sub != nil {
			return sub
		}
	}
	for _, f := range d.Files {
		if sub := recursiveIDSearch(id, f, &seen); sub != nil {
			return sub
		}
	}
	return nil
}

// GetPackagesByPurl queries the document packages and returns all that
// match the specified purl bits
func (d *Document) GetPackagesByPurl(purlSpec *purl.PackageURL, opts ...PurlSearchOption) []*Package {
	seen := map[string]struct{}{}
	foundPackages := []*Package{}

	if purlSpec.Type == "" {
		purlSpec.Type = "*"
	}

	if purlSpec.Name == "" {
		purlSpec.Name = "*"
	}

	if purlSpec.Version == "" {
		purlSpec.Version = "*"
	}

	if purlSpec.Namespace == "" {
		purlSpec.Namespace = "*"
	}

	for _, p := range d.Packages {
		foundPackages = append(foundPackages, recursivePurlSearch(purlSpec, p, &seen, opts...)...)
	}
	for _, f := range d.Files {
		foundPackages = append(foundPackages, recursivePurlSearch(purlSpec, f, &seen, opts...)...)
	}
	return foundPackages
}

type ValidationResults struct {
	Success          bool
	Message          string
	FileName         string
	FailedAlgorithms []string
}

// ValidateFiles gets a list of paths and checks the files in the document
// to make sure their integrity is known
func (d *Document) ValidateFiles(filePaths []string) ([]ValidationResults, error) {
	results := []ValidationResults{}
	if len(filePaths) == 0 {
		logrus.Warn("ValidateFiles called with 0 paths")
	}

	// Assume that the current working dir is within the package
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("unable to get current working dir: %w", err)
	}
	baseDir := filepath.Base(cwd)

	allFiles := make(map[string]*File)
	var pkg *Package
	// Search for the package describing the directory
	for _, p := range d.Packages {
		if p.Name == baseDir {
			pkg = p
			break
		}
	}
	if pkg == nil {
		if len(d.Packages) > 0 {
			return results, errors.New("directory not found in SBOM packages")
		}

		// No packages specified, use the root files
		for k, v := range d.Files {
			allFiles[k] = v
		}
	} else {
		for _, file := range pkg.Files() {
			allFiles[file.ID] = file
		}
	}

	if len(allFiles) == 0 {
		return results, errors.New("document has no files")
	}
	spdxObject := NewSPDX()
	var e error
	for _, path := range filePaths {
		res := ValidationResults{
			FailedAlgorithms: []string{},
		}
		if !util.Exists(path) {
			res.FileName = path
			res.Message = "File not found"
			results = append(results, res)
			e = errors.New("some files were not found")
			continue
		}

		// Create a new SPDX file from the path
		testFile, err := spdxObject.FileFromPath(path)
		if err != nil {
			e := fmt.Errorf("unable to create SPDX File from path: %w", err)
			res.Message = e.Error()
			continue
		}

		// Look for the file in the document
		valid := false
		message := "file path not found in document"
		res.FileName = path

		for _, docFile := range allFiles {
			if docFile.FileName != path {
				continue
			}

			if len(docFile.Checksum) == 0 {
				valid = false
				message = "no hashes found for file in SBOM"
				break
			}

			// File found, check it
			checks := 0
			for algo, documentHashValue := range docFile.Checksum {
				if artifactHashValue, ok := testFile.Checksum[algo]; ok {
					if artifactHashValue == documentHashValue {
						checks++
						valid = true
					} else {
						message = MessageHashMismatch
						res.FailedAlgorithms = append(res.FailedAlgorithms, algo)
					}
				} else {
					logrus.Warnf("document has hash in %s, which is not supported yet", algo)
				}
			}
			if checks == 0 {
				res.Message = "unable to find compatible algorithm in document"
				break
			}
			if len(res.FailedAlgorithms) > 0 {
				message = "some hash values don't match"
				valid = false
				break
			}

			res.Success = valid
			if valid {
				message = "File validated successfully"
			}
		}
		res.Message = message
		res.Success = valid
		results = append(results, res)
	}
	return results, e
}
