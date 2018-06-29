// +build libpod

package server

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

// ListImages lists existing images.
func (s *Server) ListImages(ctx context.Context, req *pb.ListImagesRequest) (resp *pb.ListImagesResponse, err error) {
	const operation = "list_images"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("ListImagesRequest: %+v", req)
	filter := ""
	reqFilter := req.GetFilter()
	if reqFilter != nil {
		filterImage := reqFilter.GetImage()
		if filterImage != nil {
			filter = filterImage.Image
		}
	}

	// TODO fix the filtering.
	_ = filter

	results, err := s.ImageRuntime().GetImages()
	if err != nil {
		return nil, err
	}

	resp = &pb.ListImagesResponse{}
	for _, result := range results {
		resImg := &pb.Image{
			Id:          result.ID(),
			RepoTags:    result.Names(),
			RepoDigests: result.RepoDigests(),
		}
		uid, username := getUserFromImage(result.User)
		if uid != nil {
			resImg.Uid = &pb.Int64Value{Value: *uid}
		}
		resImg.Username = username
		size, err := result.Size(ctx)

		// Dont think size should be a fatal error
		if err != nil {
			resImg.Size_ = 0
		} else {
			resImg.Size_ = *size
		}

		resp.Images = append(resp.Images, resImg)
	}
	logrus.Debugf("ListImagesResponse: %+v", resp)
	return resp, nil
}
