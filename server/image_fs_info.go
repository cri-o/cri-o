package server

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// ImageFsInfo returns information of the filesystem that is used to store images.
func (s *Server) ImageFsInfo(ctx context.Context, req *pb.ImageFsInfoRequest) (resp *pb.ImageFsInfoResponse, err error) {
	const operation = "image_fs_info"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	return nil, fmt.Errorf("not implemented")
}
