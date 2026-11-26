/*
Copyright The Kubernetes Authors.

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

package seccomp

import (
	specs "github.com/opencontainers/runtime-spec/specs-go"

	"sigs.k8s.io/security-profiles-merger/internal/strictjson"
	"sigs.k8s.io/security-profiles-merger/spm"
)

// ErrDuplicateKey, ErrUnknownField, ErrMisspelledField, ErrMissingField,
// ErrInvalidUTF8, ErrUnexpectedData and ErrNotAnObject are the spm
// sentinels UnmarshalStrict returns, as its documentation lists.
var (
	ErrDuplicateKey    = spm.ErrDuplicateKey
	ErrUnknownField    = spm.ErrUnknownField
	ErrMisspelledField = spm.ErrMisspelledField
	ErrMissingField    = spm.ErrMissingField
	ErrInvalidUTF8     = spm.ErrInvalidUTF8
	ErrUnexpectedData  = spm.ErrUnexpectedData
	ErrNotAnObject     = spm.ErrNotAnObject
)

// UnmarshalStrict decodes a seccomp profile and rejects what encoding/json
// accepts silently: a member no field reads ([ErrUnknownField]) or that
// names a field only ignoring case ([ErrMisspelledField]), a member repeated
// within one object ([ErrDuplicateKey]), bytes that are not valid UTF-8
// ([ErrInvalidUTF8]), data behind the profile ([ErrUnexpectedData]), a
// syscall entry without its names or action and an argument condition
// without its index or op, or with a null one, and a null entry or
// condition ([ErrMissingField]), and a document that is not a JSON object,
// such as null ([ErrNotAnObject]). A condition may leave out value and
// valueTwo, which default to 0. Error messages are bounded in length.
//
// The document is decoded into a fresh profile, which replaces the one given
// only on success: a member the document omits is zero in the result, and an
// error leaves the profile as it was. Use it instead of json.Unmarshal for
// an artifact, bound the size of the document first, as the Limits section
// of
// https://github.com/kubernetes-sigs/security-profiles-merger/blob/main/docs/integration.md#limits
// says, and validate the result with ValidateArtifact, or use DecodeArtifact
// for both.
func UnmarshalStrict(data []byte, profile *specs.LinuxSeccomp) error {
	if profile == nil {
		return ErrNilProfile
	}

	return strictjson.Unmarshal(data, profile)
}

// DecodeArtifact decodes an artifact with UnmarshalStrict and checks it with
// ValidateArtifact. It returns the profile only when both pass, and the
// error of the first that fails otherwise. Bound the size of data first. It
// depends on the native architecture as ValidateArtifact does
// ([Options.DecodeArtifact]).
func DecodeArtifact(data []byte) (*specs.LinuxSeccomp, error) {
	return ForHost().DecodeArtifact(data)
}
