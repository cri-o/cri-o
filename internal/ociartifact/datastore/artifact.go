package datastore

import (
	"context"
	"fmt"

	"github.com/opencontainers/go-digest"
	libart "go.podman.io/common/pkg/libartifact"
	"go.podman.io/image/v5/docker/reference"

	"github.com/cri-o/cri-o/internal/log"
)

// unknownArtifactRef is a sentinel Named reference used when the artifact name cannot be parsed.
type unknownArtifactRef struct{}

func (u unknownArtifactRef) String() string { return "unknown" }
func (u unknownArtifactRef) Name() string   { return u.String() }

// Artifact wraps a libartifact.Artifact to expose the named-reference methods expected
// by runtimePulledImageService (Reference, CanonicalName, Digest).  libartifact.Artifact
// stores the name and digest as plain fields; this wrapper parses the name into a
// reference.Named once so that callers get well-formed, normalised strings without
// duplicating that logic throughout image_vm.go.
type Artifact struct {
	*libart.Artifact

	namedRef reference.Named
}

func newArtifact(art *libart.Artifact) *Artifact {
	a := &Artifact{Artifact: art, namedRef: unknownArtifactRef{}}

	if art.Name != "" {
		if ref, err := reference.ParseNormalizedNamed(art.Name); err == nil {
			a.namedRef = ref
		} else {
			log.Warnf(context.Background(), "Failed to parse artifact name %q: %v", art.Name, err)
		}
	}

	return a
}

// Reference returns the normalised string form of the artifact's named reference
// (e.g. "docker.io/library/ubuntu:latest").
func (a *Artifact) Reference() string {
	return a.namedRef.String()
}

// CanonicalName returns the name-plus-digest form used as a stable cache key
// (e.g. "docker.io/library/ubuntu@sha256:...").
func (a *Artifact) CanonicalName() string {
	return fmt.Sprintf("%s@%s", a.namedRef.Name(), a.Artifact.Digest)
}

// Digest returns the manifest digest of the artifact.
func (a *Artifact) Digest() digest.Digest {
	return a.Artifact.Digest
}
