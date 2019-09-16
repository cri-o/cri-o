package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/pkg/log"
	"github.com/cri-o/cri-o/internal/pkg/storage"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// RemoveImage removes the image.
func (s *Server) RemoveImage(ctx context.Context, req *pb.RemoveImageRequest) (resp *pb.RemoveImageResponse, err error) {
	image := ""
	img := req.GetImage()
	if img != nil {
		image = img.Image
	}
	if image == "" {
		return nil, fmt.Errorf("no image specified")
	}
	var (
		images  []string
		deleted bool
	)
	images, err = s.StorageImageServer().ResolveNames(s.systemContext, image)
	if err != nil {
		if err == storage.ErrCannotParseImageID {
			images = append(images, image)
		} else {
			return nil, err
		}
	}
	for _, img := range images {
		err = s.StorageImageServer().UntagImage(s.systemContext, img)
		if err != nil {
			log.Debugf(ctx, "error deleting image %s: %v", img, err)
			continue
		}
		deleted = true
		break
	}
	if !deleted && err != nil {
		return nil, err
	}
	resp = &pb.RemoveImageResponse{}
	return resp, nil
}
