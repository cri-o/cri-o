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
	reference string
	digest    string
	signature string
}

// Reference returns the OCI registry reference of the object.
func (m *SignedObject) Reference() string {
	return m.reference
}

// Digest returns the digest of the signed object.
func (m *SignedObject) Digest() string {
	return m.digest
}

// Signature returns the signature of the signed object.
func (m *SignedObject) Signature() string {
	return m.signature
}
