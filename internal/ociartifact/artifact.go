package ociartifact

import (
	"fmt"

	"github.com/opencontainers/go-digest"
	"go.podman.io/common/pkg/libartifact"
	"go.podman.io/image/v5/docker/reference"
	critypes "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Artifact references an OCI artifact without its data.
type Artifact struct {
	*libartifact.Artifact

	namedRef reference.Named
	digest   digest.Digest
}

// ArtifactData separates the artifact metadata from the actual content.
type ArtifactData struct {
	data []byte
}

func NewArtifact(art *libartifact.Artifact) (*Artifact, error) {
	dgst, err := art.GetDigest()
	if err != nil {
		return nil, fmt.Errorf("get digest: %w", err)
	}

	artifact := &Artifact{
		Artifact: art,
		namedRef: unknownRef{},
		digest:   *dgst,
	}

	artifact.namedRef, err = reference.ParseNormalizedNamed(art.Name)
	if err != nil {
		// We assume the artifact name is a valid reference.
		return nil, fmt.Errorf("parse reference: %w", err)
	}

	return artifact, nil
}

// Reference returns the reference of the artifact.
func (a *Artifact) Reference() string {
	return a.namedRef.String()
}

func (a *Artifact) CanonicalName() string {
	return fmt.Sprintf("%s@%s", a.namedRef.Name(), a.digest)
}

// Digest returns the digest of the artifact.
func (a *Artifact) Digest() digest.Digest {
	return a.digest
}

// CRIImage returns an CRI image version of the artifact.
func (a *Artifact) CRIImage() *critypes.Image {
	var repoTags []string
	if taggedRef, ok := a.namedRef.(reference.Tagged); ok {
		repoTags = []string{taggedRef.String()}
	}

	return &critypes.Image{
		Id:          a.Digest().Encoded(),
		Size:        uint64(a.TotalSizeBytes()),
		RepoTags:    repoTags,
		RepoDigests: []string{a.CanonicalName()},
		Pinned:      true,
	}
}

// Data returns the data of the artifact.
func (a *ArtifactData) Data() []byte {
	return a.data
}
