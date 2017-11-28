package server

import (
	"fmt"
	"time"

	"github.com/containers/storage"
	pkgstorage "github.com/kubernetes-incubator/cri-o/pkg/storage"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// ImageStatus returns the status of the image.
func (s *Server) ImageStatus(ctx context.Context, req *pb.ImageStatusRequest) (resp *pb.ImageStatusResponse, err error) {
	const operation = "image_status"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("ImageStatusRequest: %+v", req)
	image := ""
	img := req.GetImage()
	if img != nil {
		image = img.Image
	}
	if image == "" {
		return nil, fmt.Errorf("no image specified")
	}
	images, err := s.StorageImageServer().ResolveNames(image)
	if err != nil {
		if err == pkgstorage.ErrCannotParseImageID {
			images = append(images, image)
		} else {
			return nil, err
		}
	}
	// match just the first registry as that's what kube meant
	image = images[0]
	status, err := s.StorageImageServer().ImageStatus(s.ImageContext(), image)
	if err != nil {
		if errors.Cause(err) == storage.ErrImageUnknown {
			return &pb.ImageStatusResponse{}, nil
		}
		return nil, err
	}
	resp = &pb.ImageStatusResponse{
		Image: &pb.Image{
			Id:       status.ID,
			RepoTags: status.Names,
			Size_:    *status.Size,
			// TODO: https://github.com/kubernetes-incubator/cri-o/issues/531
		},
	}
	logrus.Debugf("ImageStatusResponse: %+v", resp)
	return resp, nil
}
