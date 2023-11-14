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

package document

type Document interface {
	GetVersion() string
	GetDataLicense() string
	GetID() string
	GetName() string
	GetNamespace() string
	GetCreationInfo() CreationInfo
	GetPackages() []Package
	GetFiles() []File
	GetRelationships() []Relationship
	GetDocumentDescribes() []string
	GetExternalDocumentRefs() []ExternalDocumentRef
}

type CreationInfo interface {
	GetCreators() []string
	GetLicenseListVersion() string
	GetCreated() string
}

type File interface {
	GetID() string
	GetName() string
	GetCopyrightText() string
	GetLicenseConcluded() string
	GetLicenseInfoInFile() []string
	GetChecksums() []Checksum
}

type Relationship interface {
	GetElement() string
	GetType() string
	GetRelated() string
}

type ExternalDocumentRef interface {
	GetChecksum() Checksum
	GetExternalDocumentID() string
	GetSPDXDocument() string
}

type Package interface {
	GetID() string
	GetName() string
	GetDownloadLocation() string
	GetCopyrightText() string
	GetLicenseConcluded() string
	GetFilesAnalyzed() bool
	GetLicenseDeclared() string
	GetVersion() string
	GetVerificationCode() PackageVerificationCode
	GetPrimaryPurpose() string
	GetChecksums() []Checksum
	GetExternalRefs() []ExternalRef
}

type PackageVerificationCode interface {
	GetValue() string
}

type Checksum interface {
	GetAlgorithm() string
	GetValue() string
}

type ExternalRef interface {
	GetCategory() string
	GetLocator() string
	GetType() string
}
