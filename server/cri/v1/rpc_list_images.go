package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ListImages(
	ctx context.Context, req *pb.ListImagesRequest,
) (*pb.ListImagesResponse, error) {
	r := &types.ListImagesRequest{}

	if req.Filter != nil && req.Filter.Image != nil {
		r.Filter = &types.ImageFilter{
			Image: &types.ImageSpec{
				Image:       req.Filter.Image.Image,
				Annotations: req.Filter.Image.Annotations,
			},
		}
	}

	res, err := s.server.ListImages(ctx, r)
	if err != nil {
		return nil, err
	}

	resp := &pb.ListImagesResponse{
		Images: []*pb.Image{},
	}

	for _, x := range res.Images {
		if x == nil {
			continue
		}
		image := &pb.Image{
			Id:          x.ID,
			RepoTags:    x.RepoTags,
			RepoDigests: x.RepoDigests,
			Size_:       x.Size,
			Username:    x.Username,
		}
		if x.UID != nil {
			image.Uid = &pb.Int64Value{
				Value: x.UID.Value,
			}
		}
		if x.Spec != nil {
			image.Spec = &pb.ImageSpec{
				Image:       x.Spec.Image,
				Annotations: x.Spec.Annotations,
			}
		}

		resp.Images = append(resp.Images, image)
	}

	return resp, nil
}
