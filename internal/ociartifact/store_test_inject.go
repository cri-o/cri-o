//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package ociartifact

import (
	ociartifactmock "github.com/cri-o/cri-o/test/mocks/ociartifact"
)

// SetOCIArtifactImpl sets the OCI artifact implementation.
func (s *Store) SetImpl(impl Impl) {
	s.impl = impl
}

func (s *Store) SetFakeStore(l LibartifactStore) {
	s.LibartifactStore = l
}

type FakeLibartifactStore struct {
	*ociartifactmock.MockLibartifactStore
}
