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

package merge

import "sigs.k8s.io/security-profiles-merger/spm"

// LimitError returns a sentinel for one MaxArtifact limit: an error with
// the given text that errors.Is also matches against
// spm.ErrArtifactTooLarge, so that a caller can treat every oversized
// artifact alike without listing each limit of each package.
func LimitError(text string) error {
	return &limitError{text: text}
}

// AliasError returns an error with the given text that errors.Is also
// matches against target, for a package that reports a shared sentinel in
// words of its own.
func AliasError(text string, target error) error {
	return &aliasError{text: text, target: target}
}

// aliasError is the error AliasError returns.
type aliasError struct {
	text   string
	target error
}

func (a *aliasError) Error() string { return a.text }

// Is reports whether target is the one the error stands for.
func (a *aliasError) Is(target error) bool {
	return target == a.target
}

// limitError is the sentinel LimitError returns.
type limitError struct {
	text string
}

func (l *limitError) Error() string { return l.text }

// Is reports whether target is spm.ErrArtifactTooLarge. errors.Is compares
// the error with its own sentinel before calling Is.
func (l *limitError) Is(target error) bool {
	return target == spm.ErrArtifactTooLarge
}
