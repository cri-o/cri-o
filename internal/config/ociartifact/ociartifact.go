package ociartifact

import (
	"context"
	"fmt"

	"github.com/containers/common/libimage"
	"github.com/containers/common/pkg/config"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage"

	"github.com/cri-o/cri-o/internal/log"
)

// Impl is the main implementation interface of this package.
type Impl interface {
	Pull(context.Context, *types.SystemContext, string) (*Artifact, error)
}

// New returns a new OCI artifact implementation.
func New() Impl {
	return &defaultImpl{}
}

// Artifact can be used to manage OCI artifacts.
type Artifact struct {
	// MountPath is the local path containing the artifact data.
	MountPath string

	// Cleanup has to be called if the artifact is not used any more.
	Cleanup func()
}

// defaultImpl is the default implementation for the OCI artifact handling.
type defaultImpl struct{}

// Pull downloads and mounts the artifact content by using the provided ref.
func (*defaultImpl) Pull(ctx context.Context, sys *types.SystemContext, ref string) (*Artifact, error) {
	log.Infof(ctx, "Pulling OCI artifact from ref: %s", ref)

	storeOpts, err := storage.DefaultStoreOptions(false, 0)
	if err != nil {
		return nil, fmt.Errorf("get default storage options: %w", err)
	}

	store, err := storage.GetStore(storeOpts)
	if err != nil {
		return nil, fmt.Errorf("get container storage: %w", err)
	}

	runtime, err := libimage.RuntimeFromStore(store, &libimage.RuntimeOptions{SystemContext: sys})
	if err != nil {
		return nil, fmt.Errorf("create libimage runtime: %w", err)
	}

	images, err := runtime.Pull(ctx, ref, config.PullPolicyAlways, &libimage.PullOptions{})
	if err != nil {
		return nil, fmt.Errorf("pull OCI artifact: %w", err)
	}
	image := images[0]

	mountPath, err := image.Mount(ctx, nil, "")
	if err != nil {
		return nil, fmt.Errorf("mount OCI artifact: %w", err)
	}

	cleanup := func() {
		if err := image.Unmount(true); err != nil {
			log.Warnf(ctx, "Unable to unmount OCI artifact path %s: %v", mountPath, err)
		}
	}

	return &Artifact{
		MountPath: mountPath,
		Cleanup:   cleanup,
	}, nil
}
