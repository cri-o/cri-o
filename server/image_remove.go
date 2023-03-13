package server

import (
	"context"
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/storage"
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

	images, err := s.ImageServerList().ResolveNames(s.config.SystemContext, imageRef)
	if err != nil {
		if err == storage.ErrCannotParseImageID {
			images = append(images, imageRef)
		} else {
			return err
		}
	}
	for _, img := range images {
		err = s.ImageServerList().UntagImage(s.config.SystemContext, img)
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
