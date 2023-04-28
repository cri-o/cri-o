package server

import (
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/storage"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ListImages lists existing images.
func (s *Server) ListImages(ctx context.Context, req *types.ListImagesRequest) (*types.ListImagesResponse, error) {
	_, span := log.StartSpan(ctx)
	defer span.End()
	filter := ""
	reqFilter := req.Filter
	if reqFilter != nil {
		filterImage := reqFilter.Image
		if filterImage != nil {
			filter = filterImage.Image
		}
	}
	results, err := s.StorageImageServer().ListImages(s.config.SystemContext, filter)
	if err != nil {
		return nil, err
	}
	resp := &types.ListImagesResponse{}
	for i := range results {
		image := ConvertImage(&results[i])
		resp.Images = append(resp.Images, image)
	}
	return resp, nil
}

// ConvertImage takes an containers/storage ImageResult and converts it into a
// CRI protobuf type. More information about the "why"s of this function can be
// found in ../cri.md.
func ConvertImage(from *storage.ImageResult) *types.Image {
	if from == nil {
		return nil
	}

	repoTags := []string{}
	repoDigests := []string{}

	if len(from.RepoTags) > 0 {
		repoTags = from.RepoTags
	}

	if len(from.RepoDigests) > 0 {
		repoDigests = from.RepoDigests
	} else if from.PreviousName != "" && from.Digest != "" {
		repoDigests = []string{from.PreviousName + "@" + string(from.Digest)}
	}

	to := &types.Image{
		Id:          from.ID,
		RepoTags:    repoTags,
		RepoDigests: repoDigests,
		Pinned:      from.Pinned,
	}

	uid, username := getUserFromImage(from.User)
	to.Username = username

	if uid != nil {
		to.Uid = &types.Int64Value{Value: *uid}
	}
	if from.Size != nil {
		to.Size_ = *from.Size
	}

	return to
}
