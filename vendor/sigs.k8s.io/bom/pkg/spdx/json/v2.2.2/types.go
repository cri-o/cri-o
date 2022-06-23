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

package v222

const (
	NOASSERTION = "NOASSERTION"
	Version     = "SPDX-2.2"
)

type Document struct {
	ID                string         `json:"SPDXID"`
	Name              string         `json:"name"`
	Version           string         `json:"spdxVersion"`
	CreationInfo      CreationInfo   `json:"creationInfo"`
	DataLicense       string         `json:"dataLicense"`
	Namespace         string         `json:"documentNamespace"`
	DocumentDescribes []string       `json:"documentDescribes"`
	Files             []File         `json:"files,omitempty"`
	Packages          []Package      `json:"packages"`
	Relationships     []Relationship `json:"relationships"`
}

type CreationInfo struct {
	Created            string   `json:"created"` // Date
	Creators           []string `json:"creators"`
	LicenseListVersion string   `json:"licenseListVersion,omitempty"`
}

type Package struct {
	ID                   string                  `json:"SPDXID"`
	Name                 string                  `json:"name"`
	Version              string                  `json:"versionInfo"`
	FilesAnalyzed        bool                    `json:"filesAnalyzed"`
	LicenseDeclared      string                  `json:"licenseDeclared"`
	LicenseConcluded     string                  `json:"licenseConcluded"`
	Description          string                  `json:"description,omitempty"`
	DownloadLocation     string                  `json:"downloadLocation"`
	Originator           string                  `json:"originator,omitempty"`
	SourceInfo           string                  `json:"sourceInfo,omitempty"`
	CopyrightText        string                  `json:"copyrightText"`
	HasFiles             []string                `json:"hasFiles,omitempty"`
	LicenseInfoFromFiles []string                `json:"licenseInfoFromFiles,omitempty"`
	Checksums            []Checksum              `json:"checksums"`
	ExternalRefs         []ExternalRef           `json:"externalRefs,omitempty"`
	VerificationCode     PackageVerificationCode `json:"packageVerificationCode,omitempty"`
}

type PackageVerificationCode struct {
	Value         string   `json:"packageVerificationCodeValue"`
	ExcludedFiles []string `json:"packageVerificationCodeExcludedFiles,omitempty"`
}

type File struct {
	ID                string     `json:"SPDXID"`
	Name              string     `json:"fileName"`
	CopyrightText     string     `json:"copyrightText"`
	NoticeText        string     `json:"noticeText,omitempty"`
	LicenseConcluded  string     `json:"licenseConcluded"`
	Description       string     `json:"description,omitempty"`
	FileTypes         []string   `json:"fileTypes,omitempty"`
	LicenseInfoInFile []string   `json:"licenseInfoInFiles"` // List of licenses
	Checksums         []Checksum `json:"checksums"`
}

type Checksum struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"checksumValue"`
}

type ExternalRef struct {
	Category string `json:"referenceCategory"`
	Locator  string `json:"referenceLocator"`
	Type     string `json:"referenceType"`
}

type Relationship struct {
	Element string `json:"spdxElementId"`
	Type    string `json:"relationshipType"`
	Related string `json:"relatedSpdxElement"`
}
