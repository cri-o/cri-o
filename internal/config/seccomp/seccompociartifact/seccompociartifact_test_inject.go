//go:build test
// +build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package seccompociartifact

import (
	"github.com/cri-o/cri-o/internal/config/ociartifact"
)

// SetOCIArtifactImpl sets the OCI artifact implementation.
func (s *SeccompOCIArtifact) SetOCIArtifactImpl(impl ociartifact.Impl) {
	s.ociArtifactImpl = impl
}
