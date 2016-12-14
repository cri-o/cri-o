package server

import (
	"github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// ListImages lists existing images.
func (s *Server) ListImages(ctx context.Context, req *pb.ListImagesRequest) (*pb.ListImagesResponse, error) {
	logrus.Debugf("ListImages: %+v", req)
	// TODO
	// containers/storage will take care of this by looking inside /var/lib/ocid/images
	// and listing images.
	return &pb.ListImagesResponse{}, nil
}

// PullImage pulls a image with authentication config.
func (s *Server) PullImage(ctx context.Context, req *pb.PullImageRequest) (*pb.PullImageResponse, error) {
	logrus.Debugf("PullImage: %+v", req)

	if err := s.manager.PullImage(req.GetImage(), req.GetAuth(), req.GetSandboxConfig()); err != nil {
		return nil, err
	}

	return &pb.PullImageResponse{}, nil
}

// RemoveImage removes the image.
func (s *Server) RemoveImage(ctx context.Context, req *pb.RemoveImageRequest) (*pb.RemoveImageResponse, error) {
	logrus.Debugf("RemoveImage: %+v", req)
	return &pb.RemoveImageResponse{}, nil
}

// ImageStatus returns the status of the image.
func (s *Server) ImageStatus(ctx context.Context, req *pb.ImageStatusRequest) (*pb.ImageStatusResponse, error) {
	logrus.Debugf("ImageStatus: %+v", req)
	// TODO
	// containers/storage will take care of this by looking inside /var/lib/ocid/images
	// and getting the image status
	return &pb.ImageStatusResponse{}, nil
}
