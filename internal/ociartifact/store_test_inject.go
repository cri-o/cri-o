//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package ociartifact

import (
	ociartifactmock "github.com/cri-o/cri-o/test/mocks/ociartifact"
)

func (s *Store) SetFakeStore(l LibartifactStore) {
	s.libartifactStore = l
}

func (s *Store) SetFakeImpl(impl Impl) {
	s.impl = impl
}

type FakeLibartifactStore struct {
	*ociartifactmock.MockLibartifactStore
}

type FakeAdditionalStore struct {
	Path  string
	Store LibartifactStore
}

func (s *Store) SetFakeAdditionalStores(stores ...FakeAdditionalStore) {
	s.additionalStores = nil
	for _, fas := range stores {
		s.additionalStores = append(s.additionalStores, additionalStore{
			path:  fas.Path,
			store: fas.Store,
		})
	}
}
