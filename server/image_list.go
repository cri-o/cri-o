package server

import (
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/server/cri/types"
	"golang.org/x/net/context"
)

// ListImages lists existing images.
func (s *Server) ListImages(ctx context.Context, req *types.ListImagesRequest) (*types.ListImagesResponse, error) {
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

func ConvertImage(from *storage.ImageResult) *types.Image {
	if from == nil {
		return nil
	}

	repoTags := []string{"<none>:<none>"}
	if len(from.RepoTags) > 0 {
		repoTags = from.RepoTags
	} else if from.PreviousName != "" {
		repoTags = []string{from.PreviousName + ":<none>"}
	}

	repoDigests := []string{"<none>@<none>"}
	if len(from.RepoDigests) > 0 {
		repoDigests = from.RepoDigests
	}

	to := &types.Image{
		ID:          from.ID,
		RepoTags:    repoTags,
		RepoDigests: repoDigests,
	}

	uid, username := getUserFromImage(from.User)
	to.Username = username

	if uid != nil {
		to.UID = &types.Int64Value{Value: *uid}
	}
	if from.Size != nil {
		to.Size = *from.Size
	}

	return to
}
