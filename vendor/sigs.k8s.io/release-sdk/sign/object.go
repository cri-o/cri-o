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

package sign

// SignedObject contains additional metadata from the signing and verification
// process.
type SignedObject struct {
	image *SignedImage
	file  *SignedFile
}

// Image returns the image of the signed object and nil if it's a file.
func (s *SignedObject) Image() *SignedImage {
	return s.image
}

// File returns the file of the signed object and nil if it's an image.
func (s *SignedObject) File() *SignedFile {
	return s.file
}

// SignedFile contains additional metadata from the signing and verification
// process.
type SignedFile struct {
	path            string
	sha256          string
	signaturePath   string
	certificatePath string
}

// Path returns the path hash of the signed file.
func (s *SignedFile) Path() string {
	return s.path
}

// SHA256 returns the SHA256 hash of the signed file.
func (s *SignedFile) SHA256() string {
	return s.sha256
}

// SignaturePath returns the path to the Signature output of the signed file.
func (s *SignedFile) SignaturePath() string {
	return s.signaturePath
}

// CertificatePath returns the path to the Certificate output of the signed file.
func (s *SignedFile) CertificatePath() string {
	return s.certificatePath
}

// SignedImage contains additional metadata from the signing and verification
// process.
type SignedImage struct {
	reference string
	digest    string
	signature string
}

// Reference returns the OCI registry reference of the object.
func (s *SignedImage) Reference() string {
	return s.reference
}

// Digest returns the digest of the signed object.
func (s *SignedImage) Digest() string {
	return s.digest
}

// Signature returns the signature of the signed object.
func (s *SignedImage) Signature() string {
	return s.signature
}
