package server

import (
	"context"
	"errors"
	"fmt"

	storagetypes "github.com/containers/storage"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

// RemoveImage removes the image.
func (s *Server) RemoveImage(ctx context.Context, req *types.RemoveImageRequest) (*types.RemoveImageResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	imageRef := ""
	img := req.Image
	if img != nil {
		imageRef = img.Image
	}
	if imageRef == "" {
		return nil, errors.New("no image specified")
	}
	if err := s.removeImage(ctx, imageRef); err != nil {
		return nil, err
	}
	return &types.RemoveImageResponse{}, nil
}

func (s *Server) removeImage(ctx context.Context, imageRef string) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	if id := s.StorageImageServer().HeuristicallyTryResolvingStringAsIDPrefix(imageRef); id != nil {
		if err := s.StorageImageServer().DeleteImage(s.config.SystemContext, *id); err != nil {
			if errors.Is(err, storagetypes.ErrImageUnknown) {
				// The RemoveImage RPC is idempotent, and must not return an
				// error if the image has already been removed. Ref:
				// https://github.com/kubernetes/cri-api/blob/c20fa40/pkg/apis/runtime/v1/api.proto#L156-L157
				return nil
			}

			return fmt.Errorf("delete image: %w", err)
		}
		return nil
	}

	var (
		deleted   bool
		statusErr error
	)
	potentialMatches, err := s.StorageImageServer().CandidatesForPotentiallyShortImageName(s.config.SystemContext, imageRef)
	if err != nil {
		return err
	}
	for _, name := range potentialMatches {
		err = s.StorageImageServer().UntagImage(s.config.SystemContext, name)
		if err != nil {
			log.Debugf(ctx, "Error deleting image %s: %v", name, err)
			continue
		}
		deleted = true
		break
	}
	if !deleted && err != nil {
		return err
	}

	if errors.Is(statusErr, storagetypes.ErrNotAnImage) {
		// The RemoveImage RPC is idempotent, and must not return an
		// error if the image has already been removed. Ref:
		// https://github.com/kubernetes/cri-api/blob/c20fa40/pkg/apis/runtime/v1/api.proto#L156-L157
		return nil
	}

	return nil
}
