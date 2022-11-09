/*
Copyright 2022 The Kubernetes Authors.

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

package serialize

import (
	"fmt"
	"time"

	gojson "encoding/json"

	"sigs.k8s.io/bom/pkg/query"
	"sigs.k8s.io/bom/pkg/spdx"
	spdxJSON "sigs.k8s.io/bom/pkg/spdx/json/v2.3"
)

type Serializer interface {
	Serialize(*spdx.Document) (string, error)
}

type TagValue struct{}

// Serialize the documento into SPDX Tag-Value format. For now, the
// tag-value saerializer is just a wrapper around the old document.Render
// function. In future versions, the rendering logic should be moved here.
func (tv *TagValue) Serialize(doc *spdx.Document) (string, error) {
	return doc.Render()
}

type JSON struct{}

// Serialize serializes the document into a spdx JSON
func (json *JSON) Serialize(doc *spdx.Document) (string, error) {
	// The old Render() method finalizes the sbom before serializing
	// it. We still need to call it before building the JSON struct.
	if _, err := doc.Render(); err != nil {
		return "", fmt.Errorf("pre-rendering the document: %w", err)
	}
	jsonDoc := spdxJSON.Document{
		ID:      doc.ID,
		Name:    doc.Name,
		Version: spdxJSON.Version,
		CreationInfo: spdxJSON.CreationInfo{
			Created: time.Now().UTC().Format("2006-01-02T15:04:05Z07:00"),
			Creators: []string{
				"Tool: sigs.k8s.io/bom/pkg/spdx",
			},
			LicenseListVersion: "",
		},
		DataLicense:       doc.DataLicense,
		Namespace:         doc.Namespace,
		DocumentDescribes: []string{},
		Packages:          []spdxJSON.Package{},
		Relationships:     []spdxJSON.Relationship{},
	}

	// Generate the array for the cycler
	for _, p := range doc.Packages {
		jsonDoc.DocumentDescribes = append(jsonDoc.DocumentDescribes, p.SPDXID())
	}

	for _, p := range doc.Files {
		jsonDoc.DocumentDescribes = append(jsonDoc.DocumentDescribes, p.SPDXID())
	}

	q := query.New()
	q.Document = doc
	fp, err := q.Query("all")
	if err != nil {
		return "", fmt.Errorf("querying document: %w", err)
	}

	for _, o := range fp.Objects {
		if p, ok := o.(*spdx.Package); ok {
			jsonPackage, err := json.buildJSONPackage(p)
			if err != nil {
				return "", fmt.Errorf("serializing json package: %w", err)
			}
			jsonDoc.Packages = append(jsonDoc.Packages, jsonPackage)

			// Add the package's relationships to the doc
			for _, r := range *p.GetRelationships() {
				jsonDoc.Relationships = append(jsonDoc.Relationships, spdxJSON.Relationship{
					Element: p.SPDXID(),
					Type:    string(r.Type),
					Related: r.Peer.SPDXID(),
				})
			}
		}

		if f, ok := o.(*spdx.File); ok {
			jsonFile, err := json.buildJSONFile(f)
			if err != nil {
				return "", fmt.Errorf("serializing json package: %w", err)
			}
			jsonDoc.Files = append(jsonDoc.Files, jsonFile)

			// Add the package's relationships to the doc
			for _, r := range *f.GetRelationships() {
				jsonDoc.Relationships = append(jsonDoc.Relationships, spdxJSON.Relationship{
					Element: f.SPDXID(),
					Type:    string(r.Type),
					Related: r.Peer.SPDXID(),
				})
			}
		}
	}

	output, err := gojson.MarshalIndent(jsonDoc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling document json: %w", err)
	}
	return string(output), nil
}

// buildJSONPackage converts a SPDX package struct to a json package
// TODO(pueco): Validate package information to make sure its a valid package
func (json *JSON) buildJSONPackage(p *spdx.Package) (jsonPackage spdxJSON.Package, err error) {
	// Update the Verification code
	if err := p.ComputeVerificationCode(); err != nil {
		return jsonPackage, fmt.Errorf("computing verification code: %w", err)
	}

	// Update the license list
	if err := p.ComputeLicenseList(); err != nil {
		return jsonPackage, fmt.Errorf("computing license list from files: %w", err)
	}

	externalRefs := make([]spdxJSON.ExternalRef, len(p.ExternalRefs))
	for i, ref := range p.ExternalRefs {
		externalRefs[i].Category = ref.Category
		externalRefs[i].Locator = ref.Locator
		externalRefs[i].Type = ref.Type
	}
	jsonPackage = spdxJSON.Package{
		ID:                   p.SPDXID(),
		Name:                 p.Name,
		Version:              p.Version,
		FilesAnalyzed:        p.FilesAnalyzed,
		LicenseConcluded:     p.LicenseConcluded,
		LicenseDeclared:      p.LicenseDeclared,
		DownloadLocation:     p.DownloadLocation,
		LicenseInfoFromFiles: p.LicenseInfoFromFiles,
		PrimaryPurpose:       p.PrimaryPurpose,
		CopyrightText:        p.CopyrightText,
		HasFiles:             []string{},
		Checksums:            []spdxJSON.Checksum{},
		ExternalRefs:         externalRefs,
		VerificationCode: spdxJSON.PackageVerificationCode{
			Value: p.VerificationCode,
		},
	}
	if jsonPackage.LicenseConcluded == "" {
		jsonPackage.LicenseConcluded = spdx.NOASSERTION
	}
	if jsonPackage.LicenseDeclared == "" {
		jsonPackage.LicenseDeclared = spdx.NOASSERTION
	}
	if jsonPackage.CopyrightText == "" {
		jsonPackage.CopyrightText = spdx.NOASSERTION
	}
	if jsonPackage.DownloadLocation == "" {
		jsonPackage.DownloadLocation = spdx.NONE
	}

	for algo, value := range p.Checksum {
		jsonPackage.Checksums = append(jsonPackage.Checksums, spdxJSON.Checksum{
			Algorithm: algo,
			Value:     value,
		})
	}

	// If the package has files, we need to add them top hasFiles
	files := p.Files()
	if len(files) > 0 {
		for _, f := range files {
			if f.SPDXID() == "" {
				return jsonPackage, fmt.Errorf(
					"unable to compute has files array, file missing SPDX ID",
				)
			}
			jsonPackage.HasFiles = append(jsonPackage.HasFiles, f.SPDXID())
		}
	}
	return jsonPackage, nil
}

// buildJSONPackage converts a SPDX package struct to a json package
// TODO(pueco): Validate file information , eg check checksums are
// enum : [ "SHA256", "SHA1", "SHA384", "MD2", "MD4", "SHA512", "MD6", "MD5", "SHA224" ]
// "required" : [ "SPDXID", "copyrightText", "fileName", "licenseConcluded" ],
func (json *JSON) buildJSONFile(f *spdx.File) (jsonFile spdxJSON.File, err error) {
	if f.SPDXID() == "" {
		return jsonFile, fmt.Errorf("unamble to serialzie file, it has no SPDX ID defined")
	}
	jsonFile = spdxJSON.File{
		ID:            f.SPDXID(),
		Name:          f.Name,
		CopyrightText: f.CopyrightText,
		// NoticeText:        f.C,
		LicenseConcluded: f.LicenseConcluded,
		// Description:       f.Description,
		FileTypes:         f.FileType,
		LicenseInfoInFile: []string{f.LicenseInfoInFile},
		Checksums:         []spdxJSON.Checksum{},
	}
	if jsonFile.LicenseConcluded == "" {
		jsonFile.LicenseConcluded = spdx.NOASSERTION
	}
	if jsonFile.CopyrightText == "" {
		jsonFile.CopyrightText = spdx.NOASSERTION
	}
	for algo, value := range f.Checksum {
		jsonFile.Checksums = append(jsonFile.Checksums, spdxJSON.Checksum{
			Algorithm: algo,
			Value:     value,
		})
	}
	return jsonFile, nil
}
