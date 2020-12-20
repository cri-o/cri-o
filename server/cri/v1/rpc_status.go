package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) Status(
	ctx context.Context, req *pb.StatusRequest,
) (*pb.StatusResponse, error) {
	r := &types.StatusRequest{
		Verbose: req.Verbose,
	}
	res, err := s.server.Status(ctx, r)
	if err != nil {
		return nil, err
	}
	resp := &pb.StatusResponse{
		Info:   res.Info,
		Status: &pb.RuntimeStatus{},
	}
	if res.Status != nil {
		conditions := []*pb.RuntimeCondition{}
		for _, x := range res.Status.Conditions {
			conditions = append(conditions, &pb.RuntimeCondition{
				Type:    x.Type,
				Status:  x.Status,
				Reason:  x.Reason,
				Message: x.Message,
			})
		}
		resp.Status.Conditions = conditions
	}
	return resp, nil
}
