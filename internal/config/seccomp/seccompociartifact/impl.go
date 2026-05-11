package seccompociartifact

import (
	"context"

	"github.com/cri-o/cri-o/internal/ociartifact/datastore"
)

// Impl is the main implementation interface of this package.
type Impl interface {
	PullData(context.Context, string, *datastore.PullOptions) ([]datastore.ArtifactData, error)
}
