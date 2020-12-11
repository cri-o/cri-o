package server

import (
	"context"
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/server/cri/types"
)

// RemoveImage removes the image.
func (s *Server) RemoveImage(ctx context.Context, req *types.RemoveImageRequest) error {
	image := ""
	img := req.Image
	if img != nil {
		image = img.Image
	}
	if image == "" {
		return fmt.Errorf("no image specified")
	}
	var deleted bool
	images, err := s.StorageImageServer().ResolveNames(s.config.SystemContext, image)
	if err != nil {
		if err == storage.ErrCannotParseImageID {
			images = append(images, image)
		} else {
			return err
		}
	}
	for _, img := range images {
		err = s.StorageImageServer().UntagImage(s.config.SystemContext, img)
		if err != nil {
			log.Debugf(ctx, "error deleting image %s: %v", img, err)
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
