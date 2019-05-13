package server

import (
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
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
	results, err := s.StorageImageServer().ListImages(s.systemContext, filter)
	if err != nil {
		return nil, err
	}
	resp = &pb.ListImagesResponse{}
	for i := range results {
		result := &results[i]
		resImg := &pb.Image{
			Id:          result.ID,
			RepoTags:    result.RepoTags,
			RepoDigests: result.RepoDigests,
		}
		uid, username := getUserFromImage(result.User)
		if uid != nil {
			resImg.Uid = &pb.Int64Value{Value: *uid}
		}
		resImg.Username = username
		if result.Size != nil {
			resImg.Size_ = *result.Size
		}
		resp.Images = append(resp.Images, resImg)
	}
	logrus.Debugf("ListImagesResponse: %+v", resp)
	return resp, nil
}
