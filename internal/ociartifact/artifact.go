package ociartifact

import (
	"context"
	"fmt"

	"github.com/opencontainers/go-digest"
	"go.podman.io/common/pkg/libartifact"
	"go.podman.io/image/v5/docker/reference"
	critypes "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

type unknownRef struct{}

func (unknownRef) String() string {
	return "unknown"
}

func (u unknownRef) Name() string {
	return u.String()
}

// Artifact references an OCI artifact without its data.
type Artifact struct {
	*libartifact.Artifact

	rootPath string
	namedRef reference.Named
}

// NewArtifact creates a new Artifact from a libartifact.Artifact.
func NewArtifact(art *libartifact.Artifact, rootPath string) *Artifact {
	artifact := &Artifact{
		Artifact: art,
		rootPath: rootPath,
		namedRef: unknownRef{},
	}

	if art.Name != "" {
		namedRef, err := reference.ParseNormalizedNamed(art.Name)
		if err != nil {
			log.Warnf(context.Background(), "Failed to parse artifact name %s with the error %s", art.Name, err)

			namedRef = unknownRef{}
		}

		artifact.namedRef = namedRef
	}

	return artifact
}

// Reference returns the reference of the artifact.
func (a *Artifact) Reference() string {
	return a.namedRef.String()
}

func (a *Artifact) CanonicalName() string {
	return fmt.Sprintf("%s@%s", a.namedRef.Name(), a.Artifact.Digest)
}

// Digest returns the digest of the artifact.
func (a *Artifact) Digest() digest.Digest {
	return a.Artifact.Digest
}

// RootPath returns the root path where the artifact is stored.
func (a *Artifact) RootPath() string {
	return a.rootPath
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
