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
package spdx

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	intoto "github.com/in-toto/in-toto-golang/in_toto"
	purl "github.com/package-url/packageurl-go"
	"github.com/sirupsen/logrus"

	"sigs.k8s.io/release-utils/hash"
	"sigs.k8s.io/release-utils/util"
)

// Object is an interface that dictates the common methods of spdx
// objects. Currently this includes files and packages.
type Object interface {
	SPDXID() string
	SetSPDXID(string)
	ReadSourceFile(string) error
	Render() (string, error)
	BuildID(seeds ...string)
	SetEntity(*Entity)
	AddRelationship(*Relationship)
	GetRelationships() *[]*Relationship
	ToProvenanceSubject() *intoto.Subject
	getProvenanceSubjects(opts *ProvenanceOptions, seen *map[string]struct{}) []intoto.Subject
	GetElementByID(string) Object
}

type Entity struct {
	ID               string            // Identifier string  for the object in the doc
	SourceFile       string            // Local file to read for information
	Name             string            // Name of the package
	DownloadLocation string            // Download point for the entity
	CopyrightText    string            // NOASSERTION
	FileName         string            // Name of the file
	LicenseConcluded string            // LicenseID o NOASSERTION
	LicenseComments  string            // record any relevant background information or analysis that went in to arriving at the Concluded License
	Opts             *ObjectOptions    // Entity options
	Relationships    []*Relationship   // List of objects that have a relationship woth this package
	Checksum         map[string]string // Colection of source file checksums
}

type ObjectOptions struct {
	Prefix  string
	WorkDir string
}

func (e *Entity) Options() *ObjectOptions {
	return e.Opts
}

// SPDXID returns the SPDX reference string for the object
func (e *Entity) SPDXID() string {
	return e.ID
}

// SPDXID returns the SPDX reference string for the object
func (e *Entity) SetSPDXID(id string) {
	e.ID = id
}

// BuildID sets the file ID, optionally from a series of strings
func (e *Entity) BuildID(seeds ...string) {
	if len(seeds) <= 1 {
		seeds = append(seeds, e.Name)
	}
	e.ID = buildIDString(seeds...)
}

// AddRelated this adds a related object to the file to be rendered
// on the document. The exact output depends on the related obj options
func (e *Entity) AddRelationship(rel *Relationship) {
	e.Relationships = append(e.Relationships, rel)
}

// ReadChecksums receives a path to a file and calculates its checksums
func (e *Entity) ReadChecksums(filePath string) error {
	if e.Checksum == nil {
		e.Checksum = map[string]string{}
	}

	// Hash the file contents
	for algo, fn := range map[string]func(string) (string, error){
		"SHA1":   hash.SHA1ForFile,
		"SHA256": hash.SHA256ForFile,
		"SHA512": hash.SHA512ForFile,
	} {
		csum, err := fn(filePath)
		if err != nil {
			return fmt.Errorf("hashing %s file %s: %w", algo, filePath, err)
		}
		e.Checksum[algo] = csum
	}

	return nil
}

// ReadSourceFile reads the source file for the package and populates
//
//	the fields derived from it (Checksums and FileName)
func (e *Entity) ReadSourceFile(path string) error {
	if !util.Exists(path) {
		return errors.New("unable to find package source file")
	}

	if err := e.ReadChecksums(path); err != nil {
		return fmt.Errorf("reading file checksums: %w", err)
	}

	e.SourceFile = path

	// If the entity name is blank, we set it to the file path
	e.FileName = strings.TrimPrefix(
		path, e.Options().WorkDir+string(filepath.Separator),
	)

	if e.Name == "" {
		e.Name = e.FileName
	}

	return nil
}

// Render is overridden by Package and File with their own variants
func (e *Entity) Render() (string, error) {
	return "", nil
}

func (e *Entity) GetRelationships() *[]*Relationship {
	return &e.Relationships
}

// ToProvenanceSubject converts the element to an intoto subject, suitable
// to use inprovenance attestaions
func (e *Entity) ToProvenanceSubject() *intoto.Subject {
	location := ""
	if e.DownloadLocation != "" {
		location = e.DownloadLocation
	} else if e.FileName != "" {
		location = e.FileName
	}

	if location == "" {
		logrus.Warnf("%+v", e)
		logrus.Warnf(
			"Unable to convert element %s to provenance subject, no location found",
			e.SPDXID(),
		)
		return nil
	}
	if len(e.Checksum) == 0 {
		logrus.Warnf(
			"Unable to convert element %s to provenance subject, no checksums found",
			e.SPDXID(),
		)
		return nil
	}

	sub := &intoto.Subject{
		Name:   location,
		Digest: map[string]string{},
	}

	for algo, hashVal := range e.Checksum {
		sub.Digest[strings.ToLower(algo)] = hashVal
	}
	return sub
}

// getProvenanceSubjects regturns all provenance subjects found in this
// entity by scanning all relationships recursively
//
//nolint:gocritic // seen needs to be a pointer as it is used recursively
func (e *Entity) getProvenanceSubjects(opts *ProvenanceOptions, seen *map[string]struct{}) []intoto.Subject {
	ret := []intoto.Subject{}

	if _, ok := (*seen)[e.SPDXID()]; !ok {
		esub := e.ToProvenanceSubject()
		if esub != nil {
			ret = append(ret, *esub)
		}
	}

mloop:
	for _, rel := range *e.GetRelationships() {
		if rel.Peer == nil {
			continue mloop
		}

		// If peer is external, skip
		if rel.PeerExtReference != "" {
			continue
		}
		// If the peer has already been added, skip
		if _, ok := (*seen)[rel.Peer.SPDXID()]; ok {
			continue
		}

		// If relationships filters are set
		if opts.Relationships != nil {
			// Version is useful for dependencies, so add it:
			found := false
			for exclusion, rels := range opts.Relationships {
				for _, relt := range rels {
					// If rel is excluded, we can ignore
					if exclusion == "exclude" && relt == rel.Type {
						logrus.Debugf("Relationships of type %s are excluded from provenance", rel.Type)
						continue mloop
					}

					if exclusion == "include" && relt == rel.Type {
						found = true
						break
					}
				}
			}

			// Now if rel was not found, we don't use it but only if we have a
			// list of relationships we DO want:
			if _, ok := opts.Relationships["include"]; ok {
				if !found && len(opts.Relationships["include"]) > 0 {
					logrus.Infof("Relationships of type %s not included in provenance", rel.Type)
					continue
				}
			}
		}

		// Convert entity to subject
		var subject *intoto.Subject
		if p, ok := rel.Peer.(*Package); ok {
			subject = p.ToProvenanceSubject()
		}
		if f, ok := rel.Peer.(*File); ok {
			subject = f.ToProvenanceSubject()
		}

		if subject != nil {
			ret = append(ret, *subject)
			(*seen)[rel.Peer.SPDXID()] = struct{}{}
		}
	}
	return ret
}

// GetElementByID nil function to be overridden by package and file
func (e *Entity) GetElementByID(string) Object { return nil }

// GetPackagesByPurl queries the package and returns all the nodes it is
// connected to that match the specified purl bits
func (p *Package) GetPackagesByPurl(purlSpec *purl.PackageURL, opts ...PurlSearchOption) []*Package {
	seen := map[string]struct{}{}
	return recursivePurlSearch(purlSpec, p, &seen, opts...)
}
