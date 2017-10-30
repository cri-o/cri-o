package main

const releaseNotes = `Welcome to the release of CRI-O {{.Version}}!
{{if .PreRelease}}
*This is a pre-release of CRI-O*
{{- end}}

{{.Preface}}

Please try out the release binaries and report any issues at
https://github.com/kubernetes-incubator/cri-o/issues.

{{range  $note := .Notes}}
### {{$note.Title}}

{{$note.Description}}
{{- end}}

### Contributors
{{range $contributor := .Contributors}}
* {{$contributor}}
{{- end}}

### Changes
{{range $change := .Changes}}
* {{$change.Commit}} {{$change.Description}}
{{- end}}

### Dependency Changes

Previous release can be found at [{{.Previous}}](https://github.com/kubernetes-incubator/cri-o/releases/tag/{{.Previous}})
{{range $dep := .Dependencies}}
* {{$dep.Previous}} -> {{$dep.Commit}} **{{$dep.Name}}**
{{- end}}
`
