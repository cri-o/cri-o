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

package document

// defaultReleaseNotesTemplate is the text template for the default release notes.
// k8s/release/cmd/release-notes uses text/template to render markdown
// templates.
const defaultReleaseNotesTemplate = `
{{- $CurrentRevision := .CurrentRevision -}}
{{- $PreviousRevision := .PreviousRevision -}}
# Release notes for {{$CurrentRevision}}

[Documentation](https://docs.k8s.io/docs/home)
{{if .Downloads}}
## Downloads for {{$CurrentRevision}}

{{- with .Downloads.Source}}

### Source Code

filename | sha512 hash
-------- | -----------
{{range .}}[{{.Name}}]({{.URL}}) | {{.Checksum}}{{println}}{{end}}
{{end}}

{{- with .Downloads.Client}}
### Client binaries

filename | sha512 hash
-------- | -----------
{{range .}}[{{.Name}}]({{.URL}}) | {{.Checksum}}{{println}}{{end}}
{{end}}

{{- with .Downloads.Server}}
### Server binaries

filename | sha512 hash
-------- | -----------
{{range .}}[{{.Name}}]({{.URL}}) | {{.Checksum}}{{println}}{{end}}
{{end}}

{{- with .Downloads.Node}}
### Node binaries

filename | sha512 hash
-------- | -----------
{{range .}}[{{.Name}}]({{.URL}}) | {{.Checksum}}{{println}}{{end}}
{{end -}}
{{end}}
# Changelog since {{$PreviousRevision}}

{{with .NotesWithActionRequired -}}
## Urgent Upgrade Notes 

### (No, really, you MUST read this before you upgrade)

{{range .}} {{println "-" .}} {{end}}
{{end}}

{{- with .Notes -}}
## Changes by Kind
{{ range .}}
### {{.Kind | prettyKind}}
{{range $note := .NoteEntries }} - {{$note}}{{end}}
{{- end}}
{{- end}}
{{- /* This removes any extra line at the end. */ -}}
`
