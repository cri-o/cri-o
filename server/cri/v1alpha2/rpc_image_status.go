package v1alpha2

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ImageStatus(
	ctx context.Context, req *pb.ImageStatusRequest,
) (*pb.ImageStatusResponse, error) {
	r := &types.ImageStatusRequest{
		Image:   &types.ImageSpec{},
		Verbose: req.Verbose,
	}
	if req.Image != nil {
		r.Image = &types.ImageSpec{
			Image:       req.Image.Image,
			Annotations: req.Image.Annotations,
		}
	}

	res, err := s.server.ImageStatus(ctx, r)
	if err != nil {
		return nil, err
	}

	resp := &pb.ImageStatusResponse{Info: res.Info}
	if res.Image != nil {
		resp.Image = &pb.Image{
			Id:          res.Image.ID,
			RepoTags:    res.Image.RepoTags,
			RepoDigests: res.Image.RepoDigests,
			Size_:       res.Image.Size,
			Username:    res.Image.Username,
		}
		if res.Image.UID != nil {
			resp.Image.Uid = &pb.Int64Value{
				Value: res.Image.UID.Value,
			}
		}
		if res.Image.Spec != nil {
			resp.Image.Spec = &pb.ImageSpec{
				Image:       res.Image.Spec.Image,
				Annotations: res.Image.Spec.Annotations,
			}
		}
	}

	return resp, nil
}
