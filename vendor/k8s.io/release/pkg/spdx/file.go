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
	"text/template"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var fileTemplate = `{{ if .Name }}FileName: {{ .Name }}
{{ end -}}
{{ if .ID }}SPDXID: {{ .ID }}
{{ end -}}
{{- if .Checksum -}}
{{- range $key, $value := .Checksum -}}
{{ if . }}FileChecksum: {{ $key }}: {{ $value }}
{{ end -}}
{{- end -}}
{{- end -}}
LicenseConcluded: {{ if .LicenseConcluded }}{{ .LicenseConcluded }}{{ else }}NOASSERTION{{ end }}
LicenseInfoInFile: {{ if .LicenseInfoInFile }}{{ .LicenseInfoInFile }}{{ else }}NOASSERTION{{ end }}
FileCopyrightText: {{ if .CopyrightText }}<text>{{ .CopyrightText }}
</text>{{ else }}NOASSERTION{{ end }}

`

// File abstracts a file contained in a package
type File struct {
	Entity
	LicenseInfoInFile string // GPL-3.0-or-later
}

func NewFile() (f *File) {
	f = &File{}
	f.Entity.Opts = &ObjectOptions{}
	return f
}

// Render renders the document fragment of a file
func (f *File) Render() (docFragment string, err error) {
	// If we have not yet checksummed the file, do it now:
	if f.Checksum == nil || len(f.Checksum) == 0 {
		if f.SourceFile != "" {
			if err := f.ReadSourceFile(f.SourceFile); err != nil {
				return "", errors.Wrap(err, "checksumming file")
			}
		} else {
			logrus.Warnf(
				"File %s does not have checksums, SBOM will not be SPDX compliant", f.ID,
			)
		}
	}
	var buf bytes.Buffer
	tmpl, err := template.New("file").Parse(fileTemplate)
	if err != nil {
		return "", errors.Wrap(err, "parsing file template")
	}

	// Run the template to verify the output.
	if err := tmpl.Execute(&buf, f); err != nil {
		return "", errors.Wrap(err, "executing spdx file template")
	}

	docFragment = buf.String()
	return docFragment, nil
}

// BuildID sets the file ID, optionally from a series of strings
func (f *File) BuildID(seeds ...string) {
	f.Entity.BuildID(append([]string{"SPDXRef-File"}, seeds...)...)
}
