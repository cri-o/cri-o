package server

import (
	"context"
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/storage"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// RemoveImage removes the image.
func (s *Server) RemoveImage(ctx context.Context, req *types.RemoveImageRequest) error {
	imageRef := ""
	img := req.Image
	if img != nil {
		imageRef = img.Image
	}
	if imageRef == "" {
		return fmt.Errorf("no image specified")
	}
	return s.removeImage(ctx, imageRef)
}

func (s *Server) untagImageForAllImageServers(img string) error {
	iServer, err := s.StorageImageServerPerImage(img)
	if err != nil {
		return err
	}
	for _, is := range iServer {
		err = is.UntagImage(s.config.SystemContext, img)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) removeImage(ctx context.Context, imageRef string) error {
	var deleted bool
	images, err := s.MultiStorageImageServer().ResolveNames(s.config.SystemContext, imageRef)
	if err != nil {
		if err == storage.ErrCannotParseImageID {
			images = append(images, imageRef)
		} else {
			return err
		}
	}
	for _, img := range images {
		err = s.untagImageForAllImageServers(img)
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
