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
	"bytes"
	"crypto/sha1"
	"fmt"
	"html/template"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-utils/hash"
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
{{- if .Creator.Tool -}}
{{- range $key, $value := .Creator.Tool }}Creator: Tool: {{ $value }}
{{ end -}}
{{- end -}}
{{ end -}}
{{ if .Created }}Created: {{ dateFormat .Created }}
{{ end }}

`

// Document abstracts the SPDX document
type Document struct {
	Version     string // SPDX-2.2
	DataLicense string // CC0-1.0
	ID          string // SPDXRef-DOCUMENT
	Name        string // hello-go-src
	Namespace   string // https://swinslow.net/spdx-examples/example6/hello-go-src-v1
	Creator     struct {
		Person string   // Steve Winslow (steve@swinslow.net)
		Tool   []string // github.com/spdx/tools-golang/builder
	}
	Created         time.Time // 2020-11-24T01:12:27Z
	Packages        map[string]*Package
	Files           map[string]*File      // List of files
	ExternalDocRefs []ExternalDocumentRef // List of related external documents
}

// ExternalDocumentRef is a pointer to an external, related document
type ExternalDocumentRef struct {
	ID        string            `yaml:"id"`        // Identifier for the external doc (eg "external-source-bom")
	URI       string            `yaml:"uri"`       // URI where the doc can be retrieved
	Checksums map[string]string `yaml:"checksums"` // Document checksums
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
		return errors.Wrap(err, "while calculating the sha256 checksum of the external reference")
	}
	ed.Checksums["SHA1"] = val
	return nil
}

// NewDocument returns a new SPDX document with some defaults preloaded
func NewDocument() *Document {
	return &Document{
		ID:          "SPDXRef-DOCUMENT",
		Version:     "SPDX-2.2",
		DataLicense: "CC0-1.0",
		Created:     time.Now().UTC(),
		Creator: struct {
			Person string
			Tool   []string
		}{
			Person: defaultDocumentAuthor,
			Tool:   []string{"k8s.io/release/pkg/spdx"},
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
	}
	if pkg.SPDXID() == "" {
		return errors.New("package id is needed to add a new package")
	}
	if _, ok := d.Packages[pkg.SPDXID()]; ok {
		return errors.New("a package named " + pkg.SPDXID() + " already exists in the document")
	}

	d.Packages[pkg.SPDXID()] = pkg
	return nil
}

// Write outputs the SPDX document into a file
func (d *Document) Write(path string) error {
	content, err := d.Render()
	if err != nil {
		return errors.Wrap(err, "rendering SPDX code")
	}
	if err := os.WriteFile(path, []byte(content), os.FileMode(0o644)); err != nil {
		return errors.Wrap(err, "writing SPDX code to file")
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
		return "", errors.Wrap(err, "executing spdx document template")
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
			return "", errors.Wrap(err, "rendering file "+file.Name)
		}
		doc += fileDoc
		filesDescribed += fmt.Sprintf("Relationship: %s DESCRIBES %s\n\n", d.ID, file.ID)
	}
	doc += filesDescribed

	// Cycle all packages and get their data
	for _, pkg := range d.Packages {
		pkgDoc, err := pkg.Render()
		if err != nil {
			return "", errors.Wrap(err, "rendering pkg "+pkg.Name)
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
			return errors.Wrap(err, "getting sha1 of filename")
		}
		file.ID = "SPDXRef-File-" + fmt.Sprintf("%x", h.Sum(nil))
	}
	d.Files[file.ID] = file
	return nil
}
