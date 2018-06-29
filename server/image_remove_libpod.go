// +build libpod

package server

import (
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

// RemoveImage removes the image.
func (s *Server) RemoveImage(ctx context.Context, req *pb.RemoveImageRequest) (resp *pb.RemoveImageResponse, err error) {
	const operation = "remove_image"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("RemoveImageRequest: %+v", req)

	image, err := s.ImageRuntime().NewFromLocal(req.Image.Image)
	if err != nil {
		return nil, err
	}

	// TODO Followup: do we not check for the existence of containers using the image. But this requires implementing
	// the libpod runtime which we are not ready for.  When we do, it might be better to call runtime.RemoveImage as
	// that will remove some of the logic below.

	// We have an image known by multiple names and the input is not an image ID
	if len(image.Names()) > 1 && !image.InputIsID() {
		repoName, err := image.MatchRepoTag(image.InputName)
		if err != nil {
			return nil, err
		}
		if err := image.UntagImage(repoName); err != nil {
			return nil, nil
		}
	} else {
		if err := image.Remove(false); err != nil {
			return nil, err
		}
	}
	resp = &pb.RemoveImageResponse{}
	logrus.Debugf("RemoveImageResponse: %+v", resp)
	return resp, nil
}
