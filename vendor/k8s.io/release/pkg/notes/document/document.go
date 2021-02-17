/*
Copyright 2019 The Kubernetes Authors.

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

import (
	"crypto/sha512"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/release/pkg/notes"
	"k8s.io/release/pkg/notes/options"
	"k8s.io/release/pkg/release"
)

// Document represents the underlying structure of a release notes document.
type Document struct {
	NotesWithActionRequired notes.Notes    `json:"action_required"`
	Notes                   NoteCollection `json:"notes"`
	Downloads               *FileMetadata  `json:"downloads"`
	CurrentRevision         string         `json:"release_tag"`
	PreviousRevision        string
	CVEList                 []notes.CVEData
}

// FileMetadata contains metadata about files associated with the release.
type FileMetadata struct {
	// Files containing source code
	Source []File

	// Client binaries
	Client []File

	// Server binaries
	Server []File

	// Node binaries
	Node []File
}

// fetchMetadata generates file metadata for k8s binaries in `dir`. Returns nil
// if `dir` is not given or when there are no matching well known k8s binaries
// in `dir`.
func fetchMetadata(dir, urlPrefix, tag string) (*FileMetadata, error) {
	if dir == "" {
		return nil, nil
	}
	if tag == "" {
		return nil, errors.New("release tags not specified")
	}
	if urlPrefix == "" {
		return nil, errors.New("url prefix not specified")
	}

	fm := new(FileMetadata)
	m := map[*[]File][]string{
		&fm.Source: {"kubernetes.tar.gz", "kubernetes-src.tar.gz"},
		&fm.Client: {"kubernetes-client*.tar.gz"},
		&fm.Server: {"kubernetes-server*.tar.gz"},
		&fm.Node:   {"kubernetes-node*.tar.gz"},
	}

	var fileCount int
	for fileType, patterns := range m {
		fInfo, err := fileInfo(dir, patterns, urlPrefix, tag)
		if err != nil {
			return nil, errors.Wrap(err, "fetching file info")
		}
		*fileType = append(*fileType, fInfo...)
		fileCount += len(fInfo)
	}

	if fileCount == 0 {
		return nil, nil
	}
	return fm, nil
}

// fileInfo fetches file metadata for files in `dir` matching `patterns`
func fileInfo(dir string, patterns []string, urlPrefix, tag string) ([]File, error) {
	var files []File
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, err
		}

		for _, filePath := range matches {
			f, err := os.Open(filePath)
			if err != nil {
				return nil, err
			}
			defer f.Close()

			h := sha512.New()
			if _, err := io.Copy(h, f); err != nil {
				return nil, err
			}

			fileName := filepath.Base(filePath)
			files = append(files, File{
				Checksum: fmt.Sprintf("%x", h.Sum(nil)),
				Name:     fileName,
				URL:      fmt.Sprintf("%s/%s/%s", urlPrefix, tag, fileName),
			})
		}
	}
	return files, nil
}

// A File is a downloadable file.
type File struct {
	Checksum, Name, URL string
}

// NoteCategory contains notes of the same `Kind` (i.e category).
type NoteCategory struct {
	Kind        notes.Kind
	NoteEntries *notes.Notes
}

// NoteCollection is a collection of note categories.
type NoteCollection []NoteCategory

// Sort sorts the collection by priority order.
func (n *NoteCollection) Sort(kindPriority []notes.Kind) {
	indexOf := func(kind notes.Kind) int {
		for i, prioKind := range kindPriority {
			if kind == prioKind {
				return i
			}
		}
		return -1
	}

	noteSlice := (*n)
	sort.Slice(noteSlice, func(i, j int) bool {
		return indexOf(noteSlice[i].Kind) < indexOf(noteSlice[j].Kind)
	})
}

var kindPriority = []notes.Kind{
	notes.KindDeprecation,
	notes.KindAPIChange,
	notes.KindFeature,
	notes.KindDesign,
	notes.KindDocumentation,
	notes.KindFailingTest,
	notes.KindBug,
	notes.KindRegression,
	notes.KindCleanup,
	notes.KindFlake,
	notes.KindOther,
	notes.KindUncategorized,
}

var kindMap = map[notes.Kind]notes.Kind{
	notes.KindRegression: notes.KindBug,
	notes.KindCleanup:    notes.KindOther,
	notes.KindFlake:      notes.KindOther,
}

// GatherReleaseNotesDocument creates a new gatherer and collects the release
// notes into a fresh document
func GatherReleaseNotesDocument(
	opts *options.Options, previousRev, currentRev string,
) (*Document, error) {
	releaseNotes, err := notes.GatherReleaseNotes(opts)
	if err != nil {
		return nil, errors.Wrapf(err, "gathering release notes")
	}

	doc, err := New(releaseNotes, previousRev, currentRev)
	if err != nil {
		return nil, errors.Wrapf(err, "creating release note document")
	}

	return doc, nil
}

// New assembles an organized document from an unorganized set of release notes
func New(
	releaseNotes *notes.ReleaseNotes,
	previousRev, currentRev string,
) (*Document, error) {
	doc := &Document{
		NotesWithActionRequired: notes.Notes{},
		Notes:                   NoteCollection{},
		CurrentRevision:         currentRev,
		PreviousRevision:        previousRev,
	}

	stripRE := regexp.MustCompile(`^([-\*]+\s+)`)
	// processNote encapsulates the pre-processing that might happen on a note
	// text before it gets bulleted during rendering.
	processNote := func(s string) string {
		return stripRE.ReplaceAllLiteralString(s, "")
	}

	kindCategory := make(map[notes.Kind]NoteCategory)
	for _, pr := range releaseNotes.History() {
		note := releaseNotes.Get(pr)

		cvedata, hasCVE := note.DataFields["cve"]
		if hasCVE {
			logrus.Infof("Release note for PR #%d has CVE vulnerability info", note.PrNumber)
			cve := notes.CVEData{}
			if val, ok := cvedata.(map[interface{}]interface{})["id"].(string); ok {
				cve.ID = val
			}
			if val, ok := cvedata.(map[interface{}]interface{})["title"].(string); ok {
				cve.Title = val
			}
			if val, ok := cvedata.(map[interface{}]interface{})["linkedPRs"].([]interface{}); ok {
				cve.LinkedPRs = []int{}
				for _, prid := range val {
					cve.LinkedPRs = append(cve.LinkedPRs, prid.(int))
				}
			}
			if val, ok := cvedata.(map[interface{}]interface{})["published"].(string); ok {
				cve.Published = val
			}
			if val, ok := cvedata.(map[interface{}]interface{})["score"].(float64); ok {
				cve.Score = float32(val)
			}
			if val, ok := cvedata.(map[interface{}]interface{})["rating"].(string); ok {
				cve.Rating = val
			}
			if val, ok := cvedata.(map[interface{}]interface{})["description"].(string); ok {
				cve.Description = val
			}
			doc.CVEList = append(doc.CVEList, cve)
		}

		// TODO: Refactor the logic here and add testing.
		if note.DuplicateKind {
			kind := mapKind(highestPriorityKind(note.Kinds))
			if existing, ok := kindCategory[kind]; ok {
				*existing.NoteEntries = append(*existing.NoteEntries, processNote(note.Markdown))
			} else {
				kindCategory[kind] = NoteCategory{Kind: kind, NoteEntries: &notes.Notes{processNote(note.Markdown)}}
			}
		} else if note.ActionRequired {
			doc.NotesWithActionRequired = append(doc.NotesWithActionRequired, processNote(note.Markdown))
		} else {
			for _, kind := range note.Kinds {
				mappedKind := mapKind(notes.Kind(kind))

				if existing, ok := kindCategory[mappedKind]; ok {
					*existing.NoteEntries = append(*existing.NoteEntries, processNote(note.Markdown))
				} else {
					kindCategory[mappedKind] = NoteCategory{Kind: mappedKind, NoteEntries: &notes.Notes{processNote(note.Markdown)}}
				}
			}

			if len(note.Kinds) == 0 {
				// the note has not been categorized so far
				kind := notes.KindUncategorized
				if existing, ok := kindCategory[kind]; ok {
					*existing.NoteEntries = append(*existing.NoteEntries, processNote(note.Markdown))
				} else {
					kindCategory[kind] = NoteCategory{Kind: kind, NoteEntries: &notes.Notes{processNote(note.Markdown)}}
				}
			}
		}
	}

	for _, category := range kindCategory {
		sort.Strings(*category.NoteEntries)
		doc.Notes = append(doc.Notes, category)
	}

	doc.Notes.Sort(kindPriority)
	sort.Strings(doc.NotesWithActionRequired)
	return doc, nil
}

// RenderMarkdownTemplate renders a document using the golang template in
// `templateSpec`. If `templateSpec` is set to `options.GoTemplateDefault`,
// then it renders in the default template markdown format.
func (d *Document) RenderMarkdownTemplate(bucket, fileDir, templateSpec string) (string, error) {
	urlPrefix := release.URLPrefixForBucket(bucket)

	fileMetadata, err := fetchMetadata(fileDir, urlPrefix, d.CurrentRevision)
	if err != nil {
		return "", errors.Wrap(err, "fetching downloads metadata")
	}
	d.Downloads = fileMetadata

	goTemplate, err := d.template(templateSpec)
	if err != nil {
		return "", errors.Wrap(err, "fetching template")
	}
	tmpl, err := template.New("markdown").
		Funcs(template.FuncMap{"prettyKind": prettyKind}).
		Parse(goTemplate)
	if err != nil {
		return "", errors.Wrap(err, "parsing template")
	}

	var s strings.Builder
	if err := tmpl.Execute(&s, d); err != nil {
		return "", errors.Wrapf(err, "rendering with template")
	}
	return strings.TrimSpace(s.String()), nil
}

// template returns either the default template, a template from file or an
// inline string template. The `templateSpec` must be in the format of
// `go-template:{default|path/to/template.ext}` or
// `go-template:inline:string`
func (d *Document) template(templateSpec string) (string, error) {
	if templateSpec == options.GoTemplateDefault {
		return defaultReleaseNotesTemplate, nil
	}

	if !strings.HasPrefix(templateSpec, options.GoTemplatePrefix) {
		return "", errors.Errorf(
			"bad template format: expected %q, %q or %q. Got: %q",
			options.GoTemplateDefault,
			options.GoTemplatePrefix+"<file.template>",
			options.GoTemplateInline+"<template>",
			templateSpec,
		)
	}
	templatePathOrOnline := strings.TrimPrefix(templateSpec, options.GoTemplatePrefix)

	// Check for inline template
	if strings.HasPrefix(templatePathOrOnline, options.GoTemplatePrefixInline) {
		return strings.TrimPrefix(templatePathOrOnline, options.GoTemplatePrefixInline), nil
	}

	// Assume file-based template
	b, err := ioutil.ReadFile(templatePathOrOnline)
	if err != nil {
		return "", errors.Wrap(err, "reading template")
	}
	if len(b) == 0 {
		return "", errors.Errorf("template %q must be non-empty", templatePathOrOnline)
	}

	return string(b), nil
}

// CreateDownloadsTable creates the markdown table with the links to the tarballs.
// The function does nothing if the `tars` variable is empty.
func CreateDownloadsTable(w io.Writer, bucket, tars, prevTag, newTag string) error {
	if prevTag == "" || newTag == "" {
		return errors.New("release tags not specified")
	}

	urlPrefix := release.URLPrefixForBucket(bucket)
	fileMetadata, err := fetchMetadata(tars, urlPrefix, newTag)
	if fileMetadata == nil {
		// If directory is empty, doesn't contain matching files, or is not
		// given we will have a nil value. This is not an error in every
		// context. Return early so we do not modify markdown.
		fmt.Fprintf(w, "# %s\n\n", newTag)
		fmt.Fprintf(w, "## Changelog since %s\n\n", prevTag)
		return nil
	}

	if err != nil {
		return errors.Wrap(err, "fetching downloads metadata")
	}

	fmt.Fprintf(w, "# %s\n\n", newTag)
	fmt.Fprintf(w, "[Documentation](https://docs.k8s.io)\n\n")

	fmt.Fprintf(w, "## Downloads for %s\n\n", newTag)

	// Sort the files by their headers
	headers := [4]string{
		"", "Client Binaries", "Server Binaries", "Node Binaries",
	}
	files := map[string][]File{
		headers[0]: fileMetadata.Source,
		headers[1]: fileMetadata.Client,
		headers[2]: fileMetadata.Server,
		headers[3]: fileMetadata.Node,
	}

	for _, header := range headers {
		if header != "" {
			fmt.Fprintf(w, "### %s\n\n", header)
		}
		fmt.Fprintln(w, "filename | sha512 hash")
		fmt.Fprintln(w, "-------- | -----------")

		for _, f := range files[header] {
			fmt.Fprintf(w, "[%s](%s) | `%s`\n", f.Name, f.URL, f.Checksum)
		}
		fmt.Fprintln(w, "")
	}

	fmt.Fprintf(w, "## Changelog since %s\n\n", prevTag)
	return nil
}

func highestPriorityKind(kinds []string) notes.Kind {
	for _, prioKind := range kindPriority {
		for _, k := range kinds {
			kind := notes.Kind(k)
			if kind == prioKind {
				return kind
			}
		}
	}

	// Kind not in priority slice, returning the first one
	return notes.Kind(kinds[0])
}

func mapKind(kind notes.Kind) notes.Kind {
	if newKind, ok := kindMap[kind]; ok {
		return newKind
	}
	return kind
}

func prettyKind(kind notes.Kind) string {
	if kind == notes.KindAPIChange {
		return "API Change"
	} else if kind == notes.KindFailingTest {
		return "Failing Test"
	} else if kind == notes.KindBug {
		return "Bug or Regression"
	} else if kind == notes.KindOther {
		return string(notes.KindOther)
	}
	return strings.Title(string(kind))
}
