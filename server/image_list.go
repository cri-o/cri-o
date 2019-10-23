package server

import (
	"github.com/cri-o/cri-o/internal/pkg/storage"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// ListImages lists existing images.
func (s *Server) ListImages(ctx context.Context, req *pb.ListImagesRequest) (resp *pb.ListImagesResponse, err error) {
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
		image := ConvertImage(&results[i])
		resp.Images = append(resp.Images, image)
	}
	return resp, nil
}

func ConvertImage(from *storage.ImageResult) *pb.Image {
	if from == nil {
		return nil
	}

	repoTags := []string{"<none>:<none>"}
	if len(from.RepoTags) > 0 {
		repoTags = from.RepoTags
	}

	repoDigests := []string{"<none>@<none>"}
	if len(from.RepoDigests) > 0 {
		repoDigests = from.RepoDigests
	}

	to := &pb.Image{
		Id:          from.ID,
		RepoTags:    repoTags,
		RepoDigests: repoDigests,
	}

	uid, username := getUserFromImage(from.User)
	to.Username = username

	if uid != nil {
		to.Uid = &pb.Int64Value{Value: *uid}
	}
	if from.Size != nil {
		to.Size_ = *from.Size
	}

	return to
}
