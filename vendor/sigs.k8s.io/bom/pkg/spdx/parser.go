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
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Regexp to match the tag-value spdx expressions
var (
	tagRegExp          = regexp.MustCompile(`^([a-z0-9A-Z]+):\s+(.+)`)
	relationshioRegExp = regexp.MustCompile(`^*(\S+)\s+([_A-Z]+)\s+(\S+)`)
)

// OpenDoc opens a file, parses a SPDX tag-value file and returns a loaded
// spdx.Document object. This functions has the cyclomatic chec disabled as
// it spans specific cases for each of the tags it recognizes.
// nolint:gocyclo
func OpenDoc(path string) (doc *Document, err error) {
	// support reading SBOMs from STDIN
	var file *os.File
	var isTemp bool
	if path == "-" {
		file, err = os.CreateTemp("", "temp-sbom")
		if err != nil {
			return nil, fmt.Errorf("creating temp file to buffer sbom: %w", err)
		}
		if _, err := io.Copy(file, os.Stdin); err != nil {
			return nil, fmt.Errorf("writing SBOM to temporary file: %w", err)
		}
		isTemp = true
		if _, err := file.Seek(0, 0); err != nil {
			return doc, fmt.Errorf("rewinding temporary file: %w", err)
		}
	} else {
		file, err = os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("opening document from %s: %w", path, err)
		}
	}
	defer func() {
		file.Close()
		if isTemp {
			os.Remove(file.Name())
		}
	}()

	// Create a blank document
	doc = &Document{
		Packages:        map[string]*Package{},
		Files:           map[string]*File{},
		ExternalDocRefs: []ExternalDocumentRef{},
	}

	// Scan the file, looking for tags
	scanner := bufio.NewScanner(file)
	i := 0 // Line counter
	var currentEntity *Entity
	var currentObject Object
	var value, tag, textValue string
	var captureMultiline bool
	objects := map[string]Object{}
	rels := []struct {
		Source       string
		Relationship string
		Peer         string
		ExtDoc       string
	}{}
	for scanner.Scan() {
		// If we are capturing text for a multiline value, read and add
		// the line to the buffer
		if captureMultiline {
			textValue += scanner.Text() + "\n"
			// If we closing tag is not here, continue to the next line
			if !strings.Contains(scanner.Text(), "</text>") {
				continue
			}

			// If closing tag found, remove it from value
			value = strings.ReplaceAll(textValue, "</text>", "")
			textValue = ""
		}

		// Check if line matches if we are not reading multiline values
		if !captureMultiline {
			match := tagRegExp.FindStringSubmatch(scanner.Text())
			if len(match) != 3 {
				continue
			}
			tag = match[1]
			value = match[2]

			// If it is a multiline value, start buffering it
			if strings.HasPrefix(value, "<text>") {
				textValue = strings.ReplaceAll(value, "<text>", "") + "\n"
				captureMultiline = true

				// It may be that the closing tag is right in the same
				// line. If so, capture and finish buffering
				if strings.Contains(scanner.Text(), "</text>") {
					value = strings.ReplaceAll(textValue, "</text>", "")
					textValue = ""
				} else {
					continue
				}
			}
		}

		captureMultiline = false

		switch tag {
		case "FileName", "PackageName":
			// Both FileName or PackageName signal the start of a new entity

			// If we have an object, we store it and continue
			if currentObject != nil {
				currentObject.SetEntity(currentEntity)

				if _, ok := objects[currentObject.SPDXID()]; ok {
					return nil, fmt.Errorf("duplicate SPDXID %s", currentObject.SPDXID())
				}

				objects[currentObject.SPDXID()] = currentObject
			}

			// Create the new entity:
			currentEntity = &Entity{}

			// And the new SPDX object:
			if tag == "FileName" {
				currentObject = &File{}
				currentEntity.FileName = value
			}
			if tag == "PackageName" {
				currentObject = &Package{}
			}
			currentEntity.Name = value

		case "SPDXID":
			logrus.Debugf("Entity ID %s", value)
			if currentEntity == nil {
				doc.ID = value
			} else {
				currentEntity.ID = value
			}
		case "PackageLicenseConcluded", "LicenseConcluded":
			if value != NOASSERTION {
				currentEntity.LicenseConcluded = value
			}
		case "PackageCopyrightText", "FileCopyrightText":
			if value != NOASSERTION {
				currentEntity.CopyrightText = value
			}
			// Tags for packages
		case "FilesAnalyzed":
			currentObject.(*Package).FilesAnalyzed = value == "true"
		case "PackageVersion":
			currentObject.(*Package).Version = value
		case "PackageLicenseDeclared":
			currentObject.(*Package).LicenseDeclared = value
		case "PackageVerificationCode":
			currentObject.(*Package).VerificationCode = value
		case "PackageComment":
			currentObject.(*Package).Comment = value
		case "PackageFileName":
			currentObject.(*Package).FileName = value
		case "PackageHomePage":
			currentObject.(*Package).HomePage = value
		case "PackageLicenseInfoFromFiles":
			have := false
			// Check if we already have the license
			for _, licid := range currentObject.(*Package).LicenseInfoFromFiles {
				if licid == value {
					have = true
					break
				}
			}
			if !have {
				currentObject.(*Package).LicenseInfoFromFiles = append(currentObject.(*Package).LicenseInfoFromFiles, value)
			}
		case "PackageSupplier":
			// Supplier has a tag/value format inside
			match := tagRegExp.FindStringSubmatch(value)
			if len(match) != 3 {
				return nil, fmt.Errorf("invalid creator tag syntax at line %d", i)
			}
			switch match[1] {
			case "Person":
				currentObject.(*Package).Supplier.Person = match[2]
			case "Organization":
				currentObject.(*Package).Supplier.Organization = match[2]
			default:
				return nil, fmt.Errorf(
					"invalid supplier tag '%s' syntax at line %d, valid values are 'Organization' or 'Person'",
					match[1], i,
				)
			}
		case "LicenseInfoInFile":
			if value != NONE {
				currentObject.(*File).LicenseInfoInFile = value
			}
		case "FileChecksum", "PackageChecksum":
			// Checksums are also tag/value -> algo/hash
			match := tagRegExp.FindStringSubmatch(value)
			if len(match) != 3 {
				return nil, fmt.Errorf("invalid checksum tag syntax at line %d", i)
			}
			if currentEntity.Checksum == nil {
				currentEntity.Checksum = map[string]string{}
			}
			currentEntity.Checksum[match[1]] = match[2]
		case "Relationship":
			matches := relationshioRegExp.FindStringSubmatch(value)
			if len(matches) != 4 {
				return nil, fmt.Errorf("invalid SPDX relationship on line %d: %s", i, value)
			}

			// Check if the relationship is external
			ext := ""
			if strings.HasPrefix(matches[3], "DocumentRef-") && strings.Contains(matches[3], ":") {
				parts := strings.Split(matches[3], ":")
				if len(parts) != 2 {
					return nil, fmt.Errorf("unable to parse external document reference %s: %w", matches[3], err)
				}
				matches[3] = parts[0]
				ext = parts[1]
			}

			// Parse the relationship
			rels = append(rels, struct {
				Source       string
				Relationship string
				Peer         string
				ExtDoc       string
			}{
				matches[1], matches[2], matches[3], ext,
			})
		case "PackageDownloadLocation":
			if value != NONE {
				currentEntity.DownloadLocation = value
			}
		case "PackageLicenseComments", "LicenseComments":
			if value != NONE {
				currentEntity.LicenseComments = value
			}
			// Tags that apply top the doc
		case "Created":
			t, err := time.Parse("2006-01-02T15:04:05Z", value)
			if err != nil {
				return nil, fmt.Errorf("parsing time string in file: %s: %w", value, err)
			}
			doc.Created = t
		case "Creator":
			// Creator has a tag/value format inside
			match := tagRegExp.FindStringSubmatch(value)
			if len(match) != 3 {
				return nil, fmt.Errorf("invalid creator tag syntax at line %d", i)
			}
			switch match[1] {
			case "Person":
				doc.Creator.Person = match[2]
			case "Tool":
				doc.Creator.Tool = append(doc.Creator.Tool, match[2])
			case "Organization":
				doc.Creator.Organization = match[2]
			default:
				return nil, fmt.Errorf(
					"invalid creator tag '%s' syntax at line %d, valid values are 'Tool', 'Organization' or 'Person'",
					match[1], i,
				)
			}
		case "DataLicense":
			doc.DataLicense = value
		case "DocumentName":
			doc.Name = value
		case "DocumentNamespace":
			doc.Namespace = value
		case "SPDXVersion":
			doc.Version = value
		case "ExternalRef":
			if _, ok := currentObject.(*Package); ok {
				parts := strings.Split(value, " ")
				if len(parts) != 3 {
					return nil, errors.New("malformed external reference")
				}
				currentObject.(*Package).ExternalRefs = append(currentObject.(*Package).ExternalRefs, ExternalRef{
					Category: parts[0],
					Type:     parts[1],
					Locator:  parts[2],
				})
			} else {
				return nil, errors.New("external reference found outside of package")
			}
		case "LicenseListVersion":
			doc.LicenseListVersion = value
		default:
			logrus.Debugf("Unknown tag: %s", tag)
		}
		i++
	}

	if currentEntity == nil {
		return nil, fmt.Errorf("invalid file %s", path)
	}
	// Add the last object from the doc
	currentObject.SetEntity(currentEntity)
	if _, ok := objects[currentObject.SPDXID()]; ok {
		return nil, fmt.Errorf("duplicate SPDXID %s", currentObject.SPDXID())
	}
	objects[currentObject.SPDXID()] = currentObject

	// If somehow the scanner returned an error. Kill it.
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanned through spdx file, but got an error: %w", err)
	}

	// Now assign the relationships to the proper objects
	owned := map[string]struct{}{}
	for _, rdata := range rels {
		logrus.Debugf("Procesing %s %s %s", rdata.Source, rdata.Relationship, rdata.Peer)
		// If the source is the doc. Add them
		if rdata.Source == doc.ID {
			if p, ok := objects[rdata.Peer].(*Package); ok {
				logrus.Debugf("doc %s describes package %s", doc.ID, rdata.Peer)
				doc.Packages[rdata.Peer] = p
			}

			if f, ok := objects[rdata.Peer].(*File); ok {
				logrus.Debugf("doc %s describes file %s", doc.ID, rdata.Peer)
				doc.Files[(objects[rdata.Peer]).(*File).SPDXID()] = f
			}
			continue
		}

		// Check if the source object is defined
		if _, ok := objects[rdata.Source]; !ok {
			return nil, fmt.Errorf("unable to find source object with SPDXID %s", rdata.Source)
		}

		// Check that the peer exists
		if _, ok := objects[rdata.Peer]; !ok {
			// ... but only if it is not an external document
			if rdata.ExtDoc == "" {
				return nil, fmt.Errorf("unable to find peer object with SPDXID %s", rdata.Peer)
			}
		}

		if (objects[rdata.Source]).SPDXID() == "" {
			logrus.Fatalf("No ID in object %s:\n%+v", rdata.Source, objects[rdata.Source])
		}
		(objects[rdata.Source]).AddRelationship(&Relationship{
			FullRender:       false,
			PeerReference:    rdata.Peer,
			Type:             RelationshipType(rdata.Relationship),
			Peer:             objects[rdata.Peer],
			PeerExtReference: rdata.ExtDoc,
			// Comment:          "",
		})
		owned[rdata.Peer] = struct{}{}
	}

	// Now, finally any objects not referenced should be made
	// leafs of the document
	for id, obj := range objects {
		if _, ok := owned[id]; !ok {
			if p, ok := obj.(*Package); ok {
				doc.Packages[id] = p
			}

			if f, ok := obj.(*File); ok {
				doc.Files[id] = f
			}
		}
	}

	return doc, nil
}
