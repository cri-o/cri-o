package seccompociartifact

import (
	"context"

	"github.com/cri-o/cri-o/internal/ociartifact"
)

// Impl is the main implementation interface of this package.
type Impl interface {
	PullData(context.Context, string, *ociartifact.PullOptions) ([]ociartifact.ArtifactData, error)
}
