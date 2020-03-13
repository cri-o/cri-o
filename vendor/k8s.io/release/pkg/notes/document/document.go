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
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/release/pkg/notes"
)

// Document represents the underlying structure of a release notes document.
type Document struct {
	NotesWithActionRequired Notes       `json:"action_required"`
	NotesUncategorized      Notes       `json:"uncategorized"`
	NotesByKind             NotesByKind `json:"kinds"`
}

type Kind string
type NotesByKind map[Kind]Notes
type Notes []string

const (
	KindAPIChange       Kind = "api-change"
	KindBug             Kind = "bug"
	KindCleanup         Kind = "cleanup"
	KindDeprecation     Kind = "deprecation"
	KindDesign          Kind = "design"
	KindDocumentation   Kind = "documentation"
	KindFailingTest     Kind = "failing-test"
	KindFeature         Kind = "feature"
	KindFlake           Kind = "flake"
	KindBugCleanupFlake Kind = "Other (Bug, Cleanup or Flake)"
)

var kindPriority = []Kind{
	KindDeprecation,
	KindAPIChange,
	KindFeature,
	KindDesign,
	KindDocumentation,
	KindFailingTest,
	KindBug,
	KindCleanup,
	KindFlake,
	KindBugCleanupFlake,
}

var kindMap = map[Kind]Kind{
	KindBug:     KindBugCleanupFlake,
	KindCleanup: KindBugCleanupFlake,
	KindFlake:   KindBugCleanupFlake,
}

// CreateDocument assembles an organized document from an unorganized set of
// release notes
func CreateDocument(releaseNotes notes.ReleaseNotes, history notes.ReleaseNotesHistory) (*Document, error) {
	doc := &Document{
		NotesWithActionRequired: Notes{},
		NotesUncategorized:      Notes{},
		NotesByKind:             NotesByKind{},
	}

	for _, pr := range history {
		note := releaseNotes[pr]

		if note.DuplicateKind {
			kind := mapKind(highestPriorityKind(note.Kinds))
			existingNotes, ok := doc.NotesByKind[kind]
			if ok {
				doc.NotesByKind[kind] = append(existingNotes, note.Markdown)
			} else {
				doc.NotesByKind[kind] = []string{note.Markdown}
			}
		} else if note.ActionRequired {
			doc.NotesWithActionRequired = append(doc.NotesWithActionRequired, note.Markdown)
		} else {
			for _, kind := range note.Kinds {
				mappedKind := mapKind(Kind(kind))
				notesForKind, ok := doc.NotesByKind[mappedKind]
				if ok {
					doc.NotesByKind[mappedKind] = append(notesForKind, note.Markdown)
				} else {
					doc.NotesByKind[mappedKind] = []string{note.Markdown}
				}
			}

			if len(note.Kinds) == 0 {
				// the note has not been categorized so far
				doc.NotesUncategorized = append(doc.NotesUncategorized, note.Markdown)
			}
		}
	}

	sort.Strings(doc.NotesUncategorized)
	sort.Strings(doc.NotesWithActionRequired)
	return doc, nil
}

// RenderMarkdown accepts a Document and writes a version of that document to
// supplied io.Writer in markdown format.
func (d *Document) RenderMarkdown(bucket, tars, prevTag, newTag string) (string, error) {
	o := &strings.Builder{}
	if err := createDownloadsTable(o, bucket, tars, prevTag, newTag); err != nil {
		return "", err
	}

	nl := func() {
		o.WriteRune('\n')
	}
	nlnl := func() {
		nl()
		nl()
	}

	// writeNote encapsulates the pre-processing that might happen on a note text
	// before it gets bulleted and written to the io.Writer
	writeNote := func(s string) {
		const prefix = "- "
		if !strings.HasPrefix(s, prefix) {
			o.WriteString(prefix)
		}
		o.WriteString(s)
		nl()
	}

	// notes with action required get their own section
	if len(d.NotesWithActionRequired) > 0 {
		o.WriteString("## Urgent Upgrade Notes")
		nlnl()
		o.WriteString("### (No, really, you MUST read this before you upgrade)")
		nlnl()
		for _, note := range d.NotesWithActionRequired {
			writeNote(note)
			nl()
		}
	}

	// each Kind gets a section
	sortedKinds := sortKinds(d.NotesByKind)
	if len(sortedKinds) > 0 {
		o.WriteString("## Changes by Kind")
		nlnl()
		for _, kind := range sortedKinds {
			o.WriteString("### ")
			o.WriteString(prettyKind(kind))
			nlnl()

			sort.Strings(d.NotesByKind[kind])
			for _, note := range d.NotesByKind[kind] {
				writeNote(note)
			}
			nl()
		}
		nlnl()
	}

	// We call the uncategorized notes "Other Changes". These are changes
	// without any kind
	if len(d.NotesUncategorized) > 0 {
		o.WriteString("## Other Changes")
		nlnl()
		for _, note := range d.NotesUncategorized {
			writeNote(note)
		}
		nlnl()
	}

	return strings.TrimSpace(o.String()), nil
}

// sortKinds sorts kinds by their priority and returns the result in a string
// slice
func sortKinds(notesByKind NotesByKind) []Kind {
	res := []Kind{}
	for kind := range notesByKind {
		res = append(res, kind)
	}

	indexOf := func(kind Kind) int {
		for i, prioKind := range kindPriority {
			if kind == prioKind {
				return i
			}
		}
		return -1
	}

	sort.Slice(res, func(i, j int) bool {
		return indexOf(res[i]) < indexOf(res[j])
	})

	return res
}

// createDownloadsTable creates the markdown table with the links to the tarballs.
// The function does nothing if the `tars` variable is empty.
func createDownloadsTable(w io.Writer, bucket, tars, prevTag, newTag string) error {
	// Do not add the table if not explicitly requested
	if tars == "" {
		return nil
	}
	if prevTag == "" || newTag == "" {
		return errors.New("release tags not specified")
	}

	fmt.Fprintf(w, "# %s\n\n", newTag)
	fmt.Fprintf(w, "[Documentation](https://docs.k8s.io)\n\n")

	fmt.Fprintf(w, "## Downloads for %s\n\n", newTag)

	urlPrefix := fmt.Sprintf("https://storage.googleapis.com/%s/release", bucket)
	if bucket == "kubernetes-release" {
		urlPrefix = "https://dl.k8s.io"
	}

	for _, item := range []struct {
		heading  string
		patterns []string
	}{
		{"", []string{"kubernetes.tar.gz", "kubernetes-src.tar.gz"}},
		{"Client Binaries", []string{"kubernetes-client*.tar.gz"}},
		{"Server Binaries", []string{"kubernetes-server*.tar.gz"}},
		{"Node Binaries", []string{"kubernetes-node*.tar.gz"}},
	} {
		if item.heading != "" {
			fmt.Fprintf(w, "### %s\n\n", item.heading)
		}
		fmt.Fprintln(w, "filename | sha512 hash")
		fmt.Fprintln(w, "-------- | -----------")

		for _, pattern := range item.patterns {
			pattern := filepath.Join(tars, pattern)

			matches, err := filepath.Glob(pattern)
			if err != nil {
				return err
			}

			for _, file := range matches {
				f, err := os.Open(file)
				if err != nil {
					return err
				}
				defer f.Close()

				h := sha512.New()
				if _, err := io.Copy(h, f); err != nil {
					return err
				}

				fileName := filepath.Base(file)
				fmt.Fprintf(w,
					"[%s](%s/%s/%s) | `%x`\n",
					fileName, urlPrefix, newTag, fileName, h.Sum(nil),
				)
			}
		}

		fmt.Fprintln(w, "")
	}

	fmt.Fprintf(w, "## Changelog since %s\n\n", prevTag)
	return nil
}

func highestPriorityKind(kinds []string) Kind {
	for _, prioKind := range kindPriority {
		for _, k := range kinds {
			kind := Kind(k)
			if kind == prioKind {
				return kind
			}
		}
	}

	// Kind not in priority slice, returning the first one
	return Kind(kinds[0])
}

func mapKind(kind Kind) Kind {
	if newKind, ok := kindMap[kind]; ok {
		return newKind
	}
	return kind
}

func prettyKind(kind Kind) string {
	if kind == KindAPIChange {
		return "API Change"
	} else if kind == KindFailingTest {
		return "Failing Test"
	} else if kind == KindBugCleanupFlake {
		return string(KindBugCleanupFlake)
	}
	return strings.Title(string(kind))
}
