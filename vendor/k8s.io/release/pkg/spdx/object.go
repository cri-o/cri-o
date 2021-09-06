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
	"crypto/sha1"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/release-utils/hash"
	"sigs.k8s.io/release-utils/util"
)

// Object is an interface that dictates the common methods of spdx
// objects. Currently this includes files and packages.
type Object interface {
	SPDXID() string
	ReadSourceFile(string) error
	Render() (string, error)
	BuildID(seeds ...string)
}

type Entity struct {
	ID               string            // Identifier string  for the object in the doc
	SourceFile       string            // Local file to read for information
	Name             string            // Name of the package
	DownloadLocation string            // Download point for the entity
	CopyrightText    string            // NOASSERTION
	FileName         string            // Name of the file
	LicenseConcluded string            // LicenseID o NOASSERTION
	Opts             *ObjectOptions    // Entity options
	Relationships    []*Relationship   // List of objects that have a relationship woth this package
	Checksum         map[string]string // Colection of source file checksums
}

type ObjectOptions struct {
	WorkDir string
}

func (e *Entity) Options() *ObjectOptions {
	return e.Opts
}

// SPDXID returns the SPDX reference string for the object
func (e *Entity) SPDXID() string {
	return e.ID
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
	file, err := os.Open(filePath)
	if err != nil {
		return errors.Wrap(err, "opening file for reading: "+filePath)
	}
	defer file.Close()
	// TODO: Make this line like the others once this PR is
	// included in a k-sigs/release-util release:
	// https://github.com/kubernetes-sigs/release-utils/pull/16
	s1, err := hash.ForFile(filePath, sha1.New())
	if err != nil {
		return errors.Wrap(err, "getting sha1 sum for file")
	}
	s256, err := hash.SHA256ForFile(filePath)
	if err != nil {
		return errors.Wrap(err, "getting file checksums")
	}
	s512, err := hash.SHA512ForFile(filePath)
	if err != nil {
		return errors.Wrap(err, "getting file checksums")
	}

	e.Checksum = map[string]string{
		"SHA1":   s1,
		"SHA256": s256,
		"SHA512": s512,
	}
	return nil
}

// ReadSourceFile reads the source file for the package and populates
//  the fields derived from it (Checksums and FileName)
func (e *Entity) ReadSourceFile(path string) error {
	if !util.Exists(path) {
		return errors.New("unable to find package source file")
	}

	if err := e.ReadChecksums(path); err != nil {
		return errors.Wrap(err, "reading file checksums")
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
