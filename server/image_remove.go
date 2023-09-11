package server

import (
	"context"
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
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
		return nil, fmt.Errorf("no image specified")
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

	var images []string
	if id := s.StorageImageServer().HeuristicallyTryResolvingStringAsIDPrefix(imageRef); id != nil {
		images = []string{id.IDStringForOutOfProcessConsumptionOnly()} // This violates rules of references.StorageImageID, but it will be removed soon.
	} else {
		potentialMatches, err := s.StorageImageServer().CandidatesForPotentiallyShortImageName(s.config.SystemContext, imageRef)
		if err != nil {
			return err
		}
		for _, ref := range potentialMatches {
			images = append(images, ref.StringForOutOfProcessConsumptionOnly()) // This violates rules of references.StorageImageID, but it will be removed soon.
		}
	}

	var err error
	for _, img := range images {
		err = s.StorageImageServer().UntagImage(s.config.SystemContext, img)
		if err != nil {
			log.Debugf(ctx, "Error deleting image %s: %v", img, err)
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
