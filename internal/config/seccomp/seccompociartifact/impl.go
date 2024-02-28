package seccompociartifact

import (
	"context"

	"github.com/cri-o/cri-o/internal/config/ociartifact"
)

// Impl is the main implementation interface of this package.
type Impl interface {
	Pull(context.Context, string, *ociartifact.PullOptions) (*ociartifact.Artifact, error)
}
