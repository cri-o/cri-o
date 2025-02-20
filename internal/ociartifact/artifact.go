package ociartifact

import (
	"fmt"

	"github.com/containers/image/v5/manifest"
	"github.com/opencontainers/go-digest"
	critypes "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Artifact references an OCI artifact without its data.
type Artifact struct {
	name     string
	manifest *manifest.OCI1
	digest   digest.Digest
}

// ArtifactData separates the artifact metadata from the actual content.
type ArtifactData struct {
	title  string
	digest digest.Digest
	data   []byte
}

// Name returns the name of the artifact.
func (a *Artifact) Name() string {
	return a.name
}

// Manifest returns the manifest of the artifact.
func (a *Artifact) Manifest() *manifest.OCI1 {
	return a.manifest
}

// Digest returns the digest of the artifact.
func (a *Artifact) Digest() digest.Digest {
	return a.digest
}

// CRIImage returns an CRI image version of the artifact.
func (a *Artifact) CRIImage() *critypes.Image {
	return &critypes.Image{
		Id:          a.Digest().Encoded(),
		Size_:       a.size(),
		RepoTags:    []string{a.Name()},
		RepoDigests: []string{fmt.Sprintf("%s@%s", a.name, a.digest)},
		Pinned:      true,
	}
}

// size calculates the artifact layer size.
func (a *Artifact) size() (res uint64) {
	for _, layer := range a.Manifest().Layers {
		res += uint64(layer.Size)
	}

	return res
}

// Title returns the title of the artifact data.
func (a *ArtifactData) Title() string {
	return a.title
}

// Digest returns the digest of the artifact data.
func (a *ArtifactData) Digest() digest.Digest {
	return a.digest
}

// Data returns the data of the artifact.
func (a *ArtifactData) Data() []byte {
	return a.data
}
