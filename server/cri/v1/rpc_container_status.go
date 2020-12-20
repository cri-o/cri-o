package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ContainerStatus(
	ctx context.Context, req *pb.ContainerStatusRequest,
) (*pb.ContainerStatusResponse, error) {
	r := &types.ContainerStatusRequest{
		ContainerID: req.ContainerId,
		Verbose:     req.Verbose,
	}

	res, err := s.server.ContainerStatus(ctx, r)
	if err != nil {
		return nil, err
	}

	resp := &pb.ContainerStatusResponse{
		Info:   res.Info,
		Status: &pb.ContainerStatus{},
	}

	if res.Status != nil {
		resp.Status = &pb.ContainerStatus{
			Id:          res.Status.ID,
			State:       pb.ContainerState(res.Status.State),
			CreatedAt:   res.Status.CreatedAt,
			StartedAt:   res.Status.StartedAt,
			FinishedAt:  res.Status.FinishedAt,
			ExitCode:    res.Status.ExitCode,
			ImageRef:    res.Status.ImageRef,
			Reason:      res.Status.Reason,
			Message:     res.Status.Message,
			Labels:      res.Status.Labels,
			Annotations: res.Status.Annotations,
			LogPath:     res.Status.LogPath,
			Metadata:    &pb.ContainerMetadata{},
			Image:       &pb.ImageSpec{},
		}
		if res.Status.Image != nil {
			resp.Status.Image = &pb.ImageSpec{
				Image:       res.Status.Image.Image,
				Annotations: res.Status.Image.Annotations,
			}
		}
		if res.Status.Metadata != nil {
			resp.Status.Metadata = &pb.ContainerMetadata{
				Name:    res.Status.Metadata.Name,
				Attempt: res.Status.Metadata.Attempt,
			}
		}

		mounts := []*pb.Mount{}
		for _, x := range res.Status.Mounts {
			mounts = append(mounts, &pb.Mount{
				ContainerPath:  x.ContainerPath,
				HostPath:       x.HostPath,
				Readonly:       x.Readonly,
				SelinuxRelabel: x.SelinuxRelabel,
				Propagation:    pb.MountPropagation(x.Propagation),
			})
		}
		resp.Status.Mounts = mounts
	}

	return resp, nil
}
