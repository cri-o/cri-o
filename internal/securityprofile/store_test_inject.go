//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package securityprofile

import (
	"context"
	"time"

	"github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/docker/reference"
	imagetypes "go.podman.io/image/v5/types"
)

// SetOpenSource replaces how the store opens a registry.
func (s *Store) SetOpenSource(
	open func(context.Context, reference.Named, *imagetypes.SystemContext) (imagetypes.ImageSource, error),
) {
	s.openSource = open
}

// SetJoinedHook sets a function that runs once a caller joined a pull.
func (s *Store) SetJoinedHook(joined func()) {
	s.joined = joined
}

// SetLastUsed changes when a pulled profile was used last.
func (s *Store) SetLastUsed(dgst digest.Digest, lastUsed time.Time) {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()

	if e := s.cache.entries[dgst]; e != nil {
		e.LastUsed = lastUsed
	}
}

// SetBeforeCommitHook sets a function that runs between writing the blobs of a
// pull and committing them.
func (s *Store) SetBeforeCommitHook(hook func()) {
	s.cache.beforeCommit = hook
}
