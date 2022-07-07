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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	spdxJSON "sigs.k8s.io/bom/pkg/spdx/json/v2.2.2"
)

// Regexp to match the tag-value spdx expressions
var (
	tagRegExp          = regexp.MustCompile(`^([a-z0-9A-Z]+):\s+(.+)`)
	relationshioRegExp = regexp.MustCompile(`^*(\S+)\s+([_A-Z]+)\s+(\S+)`)
)

// OpenDoc opens a file, parses a SPDX tag-value file and returns a loaded
// spdx.Document object. This functions has the cyclomatic chec disabled as
// it spans specific cases for each of the tags it recognizes.
func OpenDoc(path string) (doc *Document, err error) {
	// support reading SBOMs from STDIN
	var file *os.File
	var isTemp bool
	if path == "-" {
		isTemp = true
		file, err = bufferSTDIN()
		if err != nil {
			return nil, fmt.Errorf("reading STDIN: %w", err)
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

	format, err := DetectSBOMEncoding(file)
	if err != nil {
		return nil, fmt.Errorf("detecting sbom encoding: %w", err)
	}

	switch format {
	case "spdx":
		return parseTagValue(file)
	case "spdx+json":
		return parseJSON(file)
	}

	return nil, errors.New("unknown SBOM encoding")
}

// parseJSON parses an SPDX document encoded in json
// nolint:gocyclo
func parseJSON(file *os.File) (doc *Document, err error) {
	jsonDoc := &spdxJSON.Document{
		CreationInfo: spdxJSON.CreationInfo{
			Creators: []string{},
		},
		DocumentDescribes:    []string{},
		Files:                []spdxJSON.File{},
		Packages:             []spdxJSON.Package{},
		Relationships:        []spdxJSON.Relationship{},
		ExternalDocumentRefs: []spdxJSON.ExternalDocumentRef{},
	}

	// Read the SPDX doc into the json struct
	var data []byte
	data, err = os.ReadFile(file.Name())
	if err != nil {
		return nil, fmt.Errorf("reading SBOM file: %w", err)
	}
	if err := json.Unmarshal(data, jsonDoc); err != nil {
		return nil, fmt.Errorf("parsing SBOM json: %w", err)
	}

	doc = &Document{
		Version:     jsonDoc.Version,
		DataLicense: jsonDoc.DataLicense,
		ID:          jsonDoc.ID,
		Name:        jsonDoc.Name,
		Creator: struct {
			Person       string
			Organization string
			Tool         []string
		}{
			Tool: []string{},
		},
		Namespace:       jsonDoc.Namespace,
		Packages:        map[string]*Package{},
		Files:           map[string]*File{},
		ExternalDocRefs: []ExternalDocumentRef{},
	}

	for _, c := range jsonDoc.CreationInfo.Creators {
		// Technical limitation in bom: We only have one person and one org
		ps := strings.SplitN(c, ":", 2)
		if len(ps) != 2 {
			logrus.Errorf("unable to parse creator data: %s", c)
			continue
		}
		ps[1] = strings.TrimSpace(ps[1])

		switch ps[0] {
		case entPerson:
			if doc.Creator.Person == "" {
				doc.Creator.Person = ps[1]
			} else {
				logrus.Warnf("Ignoring additional SBOM Creator Person")
			}
		case entOrganization:
			if doc.Creator.Organization == "" {
				doc.Creator.Organization = ps[1]
			} else {
				logrus.Warnf("Ignoring additional SBOM Creator Organization")
			}
		case entTool:
			doc.Creator.Tool = append(doc.Creator.Tool, ps[1])
		default:
			logrus.Errorf("Unknown creator record: %s", ps[0])
		}
	}

	doc.LicenseListVersion = jsonDoc.CreationInfo.LicenseListVersion
	if jsonDoc.CreationInfo.Created != "" {
		t, err := time.Parse("2006-01-02T15:04:05Z", jsonDoc.CreationInfo.Created)
		if err != nil {
			logrus.Errorf("unable to parse creation time: %s: %s", jsonDoc.CreationInfo.Created, err)
		} else {
			doc.Created = t
		}
	}

	allPackages := map[string]*Package{}
	for i := range jsonDoc.Packages {
		pData := &jsonDoc.Packages[i]
		allPackages[pData.ID] = &Package{
			Entity: Entity{
				ID:               pData.ID,
				Name:             pData.Name,
				DownloadLocation: pData.DownloadLocation,
				CopyrightText:    pData.CopyrightText,
				LicenseConcluded: pData.LicenseConcluded,
				// LicenseComments:  pData.LicenseComments,
				Relationships: []*Relationship{},
				Checksum:      map[string]string{},
			},
			FilesAnalyzed:        pData.FilesAnalyzed,
			LicenseInfoFromFiles: []string{},
			LicenseDeclared:      pData.LicenseDeclared,
			Version:              pData.Version,
			VerificationCode:     pData.VerificationCode.Value,
			// Comment:              pData.Comment,
			// HomePage:             pData.HomePage,
			Supplier: struct {
				Person       string
				Organization string
			}{},
			Originator: struct {
				Person       string
				Organization string
			}{},
			ExternalRefs: []ExternalRef{},
		}

		if pData.Checksums != nil {
			for _, cs := range pData.Checksums {
				allPackages[pData.ID].Checksum[cs.Algorithm] = cs.Value
			}
		}

		if pData.ExternalRefs != nil {
			for _, eref := range pData.ExternalRefs {
				allPackages[pData.ID].ExternalRefs = append(allPackages[pData.ID].ExternalRefs, ExternalRef{
					Category: eref.Category,
					Type:     eref.Type,
					Locator:  eref.Locator,
				})
			}
		}
	}

	allFiles := map[string]*File{}
	for i := range jsonDoc.Files {
		fData := &jsonDoc.Files[i]
		allFiles[fData.ID] = &File{
			Entity: Entity{
				ID:               fData.ID,
				Name:             fData.Name,
				CopyrightText:    fData.CopyrightText,
				LicenseConcluded: fData.LicenseConcluded,
				// LicenseComments:  pData.LicenseComments,
				Relationships: []*Relationship{},
				Checksum:      map[string]string{},
			},
			FileType:          []string{},
			LicenseInfoInFile: strings.Join(fData.LicenseInfoInFile, " AND "),
		}

		if fData.Checksums != nil {
			for _, cs := range fData.Checksums {
				allFiles[fData.ID].Checksum[cs.Algorithm] = cs.Value
			}
		}
	}

	seenObjects := map[string]string{}

	// Populate the package and file relationships before adding
	// the root level elements
	for _, r := range jsonDoc.Relationships {
		var source Object
		var peer Object
		var relatedID string
		var externalID string

		// Look for the source element
		if _, ok := allPackages[r.Element]; ok {
			source = allPackages[r.Element]
		} else if _, ok := allFiles[r.Element]; ok {
			source = allFiles[r.Element]
		}
		if source == nil {
			logrus.Warnf("unable to find SPDX source element %s", r.Element)
			continue
		}

		// Look for the peer element, exception: peer may be
		// an external reference
		if strings.HasPrefix(r.Related, "DocumentRef-") {
			externalID = r.Related
			parts := strings.SplitN(r.Related, ":", 2)
			if len(parts) != 2 {
				logrus.Errorf("Unable to parse external reference %s", r.Related)
				continue
			}
			relatedID = parts[1]
		} else {
			if _, ok := allPackages[r.Related]; ok {
				peer = allPackages[r.Related]
			} else if _, ok := allFiles[r.Related]; ok {
				peer = allFiles[r.Related]
			}
			if peer == nil {
				logrus.Warnf("unable to find SPDX related element %s", r.Related)
				continue
			}
			relatedID = peer.SPDXID()
		}

		rel := Relationship{
			PeerReference:    relatedID,
			PeerExtReference: externalID,
			Comment:          "",
			Type:             RelationshipType(r.Type),
			Peer:             peer,
		}
		source.AddRelationship(&rel)

		// Note those objects we've seen to keep track of any loose items
		if peer != nil {
			seenObjects[peer.SPDXID()] = peer.SPDXID()
		}
	}

	// Add the top level packages
	for _, el := range jsonDoc.DocumentDescribes {
		var p *Package
		var f *File
		var ok bool

		if p, ok = allPackages[el]; ok {
			doc.Packages[p.SPDXID()] = p
			seenObjects[el] = el
			continue
		}

		if f, ok = allFiles[el]; ok {
			doc.Files[p.SPDXID()] = f
			seenObjects[el] = el
			continue
		}
		logrus.Errorf("unable to find package %s described by sbom", el)
	}

	// Delete everything from the all maps to see if we missed anything
	for _, id := range seenObjects {
		delete(allPackages, id)
		delete(allFiles, id)
	}

	if l := len(allPackages); l > 0 {
		logrus.Warnf("%d packages could not be assigned to the SBOM", l)
	}

	if l := len(allFiles); l > 0 {
		logrus.Warnf("%d files could not be assigned to the SBOM", l)
	}

	// Assign external references
	for _, ref := range jsonDoc.ExternalDocumentRefs {
		extRef := ExternalDocumentRef{
			ID:  ref.ExternalDocumentID,
			URI: ref.SPDXDocument,
			Checksums: map[string]string{
				ref.Checksum.Algorithm: ref.Checksum.Value,
			},
		}
		doc.ExternalDocRefs = append(doc.ExternalDocRefs, extRef)
	}

	return doc, nil
}

// parseTagValue parses an SPDX SBOM in tag-value format
// nolint:gocyclo
func parseTagValue(file *os.File) (doc *Document, err error) {
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
			case entPerson:
				currentObject.(*Package).Supplier.Person = match[2]
			case entOrganization:
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
			case entPerson:
				doc.Creator.Person = match[2]
			case entTool:
				doc.Creator.Tool = append(doc.Creator.Tool, match[2])
			case entOrganization:
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
		return nil, fmt.Errorf("invalid file %s", file.Name())
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

// detectSBOMEncoding reads a few bytes from the SBOM and returns
func DetectSBOMEncoding(f *os.File) (format string, err error) {
	bs := make([]byte, 512)
	if _, err := f.Read(bs); err != nil {
		return "", fmt.Errorf("reading SBOM to get format: %w", err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		return "", fmt.Errorf("rewinding sbom pointer: %w", err)
	}

	// In JSON, the spdx version fiel would be quoted
	if strings.Contains(string(bs), "\"spdxVersion\"") {
		return "spdx+json", nil
	} else if strings.Contains(string(bs), "SPDXVersion:") {
		return "spdx", nil
	}
	logrus.Warn("Unable to detect SBOM encoding")
	return "", nil
}

// buyfferSTDIN buffers all of STDIN to a temp file
func bufferSTDIN() (*os.File, error) {
	file, err := os.CreateTemp("", "temp-sbom")
	if err != nil {
		return nil, fmt.Errorf("creating temp file to buffer sbom: %w", err)
	}
	if _, err := io.Copy(file, os.Stdin); err != nil {
		return nil, fmt.Errorf("writing SBOM to temporary file: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("rewinding temporary file: %w", err)
	}
	return file, nil
}
