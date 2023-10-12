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

package v22

import "sigs.k8s.io/bom/pkg/spdx/json/document"

const (
	NOASSERTION = "NOASSERTION"
	Version     = "SPDX-2.2"
)

type Document struct {
	ID                   string                `json:"SPDXID"`
	Name                 string                `json:"name"`
	Version              string                `json:"spdxVersion"`
	CreationInfo         CreationInfo          `json:"creationInfo"`
	DataLicense          string                `json:"dataLicense"`
	Namespace            string                `json:"documentNamespace"`
	DocumentDescribes    []string              `json:"documentDescribes"`
	Files                []File                `json:"files,omitempty"`
	Packages             []Package             `json:"packages"`
	Relationships        []Relationship        `json:"relationships"`
	ExternalDocumentRefs []ExternalDocumentRef `json:"externalDocumentRefs,omitempty"`
}

func (d *Document) GetVersion() string                     { return d.Version }
func (d *Document) GetDataLicense() string                 { return d.DataLicense }
func (d *Document) GetID() string                          { return d.ID }
func (d *Document) GetName() string                        { return d.Name }
func (d *Document) GetNamespace() string                   { return d.Namespace }
func (d *Document) GetCreationInfo() document.CreationInfo { return &d.CreationInfo }
func (d *Document) GetDocumentDescribes() []string         { return d.DocumentDescribes }

func (d *Document) GetPackages() []document.Package {
	packages := make([]document.Package, len(d.Packages))
	for i := range d.Packages {
		packages[i] = &d.Packages[i]
	}
	return packages
}

func (d *Document) GetFiles() []document.File {
	files := make([]document.File, len(d.Files))
	for i := range d.Files {
		files[i] = &d.Files[i]
	}
	return files
}

func (d *Document) GetRelationships() []document.Relationship {
	relationships := make([]document.Relationship, len(d.Relationships))
	for i := range d.Relationships {
		relationships[i] = &d.Relationships[i]
	}
	return relationships
}

func (d *Document) GetExternalDocumentRefs() []document.ExternalDocumentRef {
	externalDocumentRefs := make([]document.ExternalDocumentRef, len(d.ExternalDocumentRefs))
	for i := range d.ExternalDocumentRefs {
		externalDocumentRefs[i] = &d.ExternalDocumentRefs[i]
	}
	return externalDocumentRefs
}

type CreationInfo struct {
	Created            string   `json:"created"` // Date
	Creators           []string `json:"creators"`
	LicenseListVersion string   `json:"licenseListVersion,omitempty"`
}

func (c *CreationInfo) GetCreators() []string         { return c.Creators }
func (c *CreationInfo) GetLicenseListVersion() string { return c.LicenseListVersion }
func (c *CreationInfo) GetCreated() string            { return c.Created }

type Package struct {
	ID                   string                   `json:"SPDXID"`
	Name                 string                   `json:"name"`
	Version              string                   `json:"versionInfo"`
	FilesAnalyzed        bool                     `json:"filesAnalyzed"`
	LicenseDeclared      string                   `json:"licenseDeclared"`
	LicenseConcluded     string                   `json:"licenseConcluded"`
	Description          string                   `json:"description,omitempty"`
	DownloadLocation     string                   `json:"downloadLocation"`
	Originator           string                   `json:"originator,omitempty"`
	SourceInfo           string                   `json:"sourceInfo,omitempty"`
	CopyrightText        string                   `json:"copyrightText"`
	HasFiles             []string                 `json:"hasFiles,omitempty"`
	LicenseInfoFromFiles []string                 `json:"licenseInfoFromFiles,omitempty"`
	Checksums            []Checksum               `json:"checksums"`
	ExternalRefs         []ExternalRef            `json:"externalRefs,omitempty"`
	VerificationCode     *PackageVerificationCode `json:"packageVerificationCode,omitempty"`
}

func (p *Package) GetID() string               { return p.ID }
func (p *Package) GetName() string             { return p.Name }
func (p *Package) GetDownloadLocation() string { return p.DownloadLocation }
func (p *Package) GetCopyrightText() string    { return p.CopyrightText }
func (p *Package) GetLicenseConcluded() string { return p.LicenseConcluded }
func (p *Package) GetFilesAnalyzed() bool      { return p.FilesAnalyzed }
func (p *Package) GetLicenseDeclared() string  { return p.LicenseDeclared }
func (p *Package) GetVersion() string          { return p.Version }
func (p *Package) GetPrimaryPurpose() string   { return "" }

func (p *Package) GetVerificationCode() document.PackageVerificationCode {
	if p.VerificationCode == nil {
		return &PackageVerificationCode{}
	}
	return p.VerificationCode
}

func (p *Package) GetChecksums() []document.Checksum {
	checksums := make([]document.Checksum, len(p.Checksums))
	for i := range p.Checksums {
		checksums[i] = &p.Checksums[i]
	}
	return checksums
}

func (p *Package) GetExternalRefs() []document.ExternalRef {
	externalRefs := make([]document.ExternalRef, len(p.ExternalRefs))
	for i := range p.ExternalRefs {
		externalRefs[i] = &p.ExternalRefs[i]
	}
	return externalRefs
}

type PackageVerificationCode struct {
	Value         string   `json:"packageVerificationCodeValue"`
	ExcludedFiles []string `json:"packageVerificationCodeExcludedFiles,omitempty"`
}

func (p *PackageVerificationCode) GetValue() string { return p.Value }

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

func (f *File) GetID() string                  { return f.ID }
func (f *File) GetName() string                { return f.Name }
func (f *File) GetLicenseConcluded() string    { return f.LicenseConcluded }
func (f *File) GetLicenseInfoInFile() []string { return f.LicenseInfoInFile }
func (f *File) GetCopyrightText() string       { return f.CopyrightText }

func (f *File) GetChecksums() []document.Checksum {
	checksums := make([]document.Checksum, len(f.Checksums))
	for i := range f.Checksums {
		checksums[i] = &f.Checksums[i]
	}
	return checksums
}

type Checksum struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"checksumValue"`
}

func (c *Checksum) GetAlgorithm() string { return c.Algorithm }
func (c *Checksum) GetValue() string     { return c.Value }

type ExternalRef struct {
	Category string `json:"referenceCategory"`
	Locator  string `json:"referenceLocator"`
	Type     string `json:"referenceType"`
}

func (e *ExternalRef) GetCategory() string { return e.Category }
func (e *ExternalRef) GetLocator() string  { return e.Locator }
func (e *ExternalRef) GetType() string     { return e.Type }

type ExternalDocumentRef struct {
	Checksum           Checksum `json:"checksum"`
	ExternalDocumentID string   `json:"externalDocumentId"`
	SPDXDocument       string   `json:"spdxDocument"`
}

func (e *ExternalDocumentRef) GetChecksum() document.Checksum { return &e.Checksum }
func (e *ExternalDocumentRef) GetExternalDocumentID() string  { return e.ExternalDocumentID }
func (e *ExternalDocumentRef) GetSPDXDocument() string        { return e.SPDXDocument }

type Relationship struct {
	Element string `json:"spdxElementId"`
	Type    string `json:"relationshipType"`
	Related string `json:"relatedSpdxElement"`
}

func (r *Relationship) GetElement() string { return r.Element }
func (r *Relationship) GetType() string    { return r.Type }
func (r *Relationship) GetRelated() string { return r.Related }
