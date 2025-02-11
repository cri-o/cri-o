//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package ociartifact

// SetData can be used to set the artifact data for tests.
func (a *ArtifactData) SetData(data []byte) {
	a.data = data
}
