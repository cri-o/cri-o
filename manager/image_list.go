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
