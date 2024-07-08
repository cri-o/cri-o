package server

import (
	"context"
	"errors"

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
	var deleted bool
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	// FIXME: The CRI API definition says
	//      This call is idempotent, and must not return an error if the image has
	//      already been removed.
	// and this code doesn’t seem to conform to that.

	// Actually Kubelet is only ever calling this with full image IDs.
	// So we don't really need to accept ID prefixes nor short names;
	// or is there another user?!

	if id := s.StorageImageServer().HeuristicallyTryResolvingStringAsIDPrefix(imageRef); id != nil {
		return s.StorageImageServer().DeleteImage(s.config.SystemContext, *id)
	}

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
	return nil
}
