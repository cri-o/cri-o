package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ListPodSandbox(
	ctx context.Context, req *pb.ListPodSandboxRequest,
) (*pb.ListPodSandboxResponse, error) {
	r := &types.ListPodSandboxRequest{}

	if req.Filter != nil {
		r.Filter = &types.PodSandboxFilter{
			ID:            req.Filter.Id,
			LabelSelector: req.Filter.LabelSelector,
		}
		if req.Filter.State != nil {
			r.Filter.State = &types.PodSandboxStateValue{
				State: types.PodSandboxState(req.Filter.State.State),
			}
		}
	}

	res, err := s.server.ListPodSandbox(ctx, r)
	if err != nil {
		return nil, err
	}

	resp := &pb.ListPodSandboxResponse{
		Items: []*pb.PodSandbox{},
	}

	for _, x := range res.Items {
		if x == nil {
			continue
		}
		sandbox := &pb.PodSandbox{
			Id:             x.ID,
			State:          pb.PodSandboxState(x.State),
			CreatedAt:      x.CreatedAt,
			Labels:         x.Labels,
			Annotations:    x.Annotations,
			RuntimeHandler: x.RuntimeHandler,
		}
		if x.Metadata != nil {
			sandbox.Metadata = &pb.PodSandboxMetadata{
				Name:      x.Metadata.Name,
				Namespace: x.Metadata.Namespace,
				Uid:       x.Metadata.UID,
				Attempt:   x.Metadata.Attempt,
			}
		}

		resp.Items = append(resp.Items, sandbox)
	}

	return resp, nil
}
