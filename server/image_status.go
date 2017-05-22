package server

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/containers/storage"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// ImageStatus returns the status of the image.
func (s *Server) ImageStatus(ctx context.Context, req *pb.ImageStatusRequest) (*pb.ImageStatusResponse, error) {
	logrus.Debugf("ImageStatusRequest: %+v", req)
	image := ""
	img := req.GetImage()
	if img != nil {
		image = img.Image
	}
	if image == "" {
		return nil, fmt.Errorf("no image specified")
	}
	status, err := s.storageImageServer.ImageStatus(s.imageContext, image)
	if err != nil {
		if err == storage.ErrImageUnknown {
			return &pb.ImageStatusResponse{}, nil
		}
		return nil, err
	}
	resp := &pb.ImageStatusResponse{
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
