//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package datastore

// SetImpl sets the datastore implementation.
func (s *Store) SetImpl(impl Impl) {
	s.impl = impl
}

// SetData can be used to set the artifact data for tests.
func (a *ArtifactData) SetData(data []byte) {
	a.data = data
}
