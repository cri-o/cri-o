package v1alpha2

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ListContainers(
	ctx context.Context, req *pb.ListContainersRequest,
) (*pb.ListContainersResponse, error) {
	r := &types.ListContainersRequest{}

	if req.Filter != nil {
		r.Filter = &types.ContainerFilter{
			ID:            req.Filter.Id,
			LabelSelector: req.Filter.LabelSelector,
			PodSandboxID:  req.Filter.PodSandboxId,
		}
		if req.Filter.State != nil {
			r.Filter.State = &types.ContainerStateValue{
				State: types.ContainerState(req.Filter.State.State),
			}
		}
	}

	res, err := s.server.ListContainers(ctx, r)
	if err != nil {
		return nil, err
	}

	resp := &pb.ListContainersResponse{
		Containers: []*pb.Container{},
	}

	for _, x := range res.Containers {
		if x == nil {
			continue
		}
		container := &pb.Container{
			Id:           x.ID,
			PodSandboxId: x.PodSandboxID,
			State:        pb.ContainerState(x.State),
			ImageRef:     x.ImageRef,
			CreatedAt:    x.CreatedAt,
			Labels:       x.Labels,
			Annotations:  x.Annotations,
		}
		if x.Metadata != nil {
			container.Metadata = &pb.ContainerMetadata{
				Name:    x.Metadata.Name,
				Attempt: x.Metadata.Attempt,
			}
		}
		if x.Image != nil {
			container.Image = &pb.ImageSpec{
				Image:       x.Image.Image,
				Annotations: x.Image.Annotations,
			}
		}

		resp.Containers = append(resp.Containers, container)
	}

	return resp, nil
}
