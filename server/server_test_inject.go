//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package server

import (
	"context"

	"github.com/cri-o/cri-o/internal/ociartifact"
)

// SetRuntimeServer sets the runtime server for the ContainerServer.
func (s *StreamService) SetRuntimeServer(server *Server) {
	s.runtimeServer = server
}

// SetArtifactStoreForTest replaces the artifact store, allowing tests to
// inject a store pre-configured with mock internals.
func (s *Server) SetArtifactStoreForTest(store *ociartifact.Store) {
	s.artifactStore = store
}

// SetPinnedArtifactsForTest overwrites the server's pinned artifact list so
// tests can configure it after server construction.
func (s *Server) SetPinnedArtifactsForTest(refs []string) {
	s.config.PinnedArtifacts = append([]string(nil), refs...)
}

// PullPinnedArtifactsForTest exposes pullPinnedArtifacts for unit tests.
func (s *Server) PullPinnedArtifactsForTest(ctx context.Context) {
	s.pullPinnedArtifacts(ctx, append([]string(nil), s.config.PinnedArtifacts...))
}
