package v1alpha2

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) PodSandboxStatus(
	ctx context.Context, req *pb.PodSandboxStatusRequest,
) (*pb.PodSandboxStatusResponse, error) {
	r := &types.PodSandboxStatusRequest{
		PodSandboxID: req.PodSandboxId,
		Verbose:      req.Verbose,
	}
	res, err := s.server.PodSandboxStatus(ctx, r)
	if err != nil {
		return nil, err
	}
	resp := &pb.PodSandboxStatusResponse{
		Info: res.Info,
	}
	if res.Status != nil {
		resp.Status = &pb.PodSandboxStatus{
			Id:             res.Status.ID,
			State:          pb.PodSandboxState(res.Status.State),
			CreatedAt:      res.Status.CreatedAt,
			Labels:         res.Status.Labels,
			Annotations:    res.Status.Annotations,
			RuntimeHandler: res.Status.RuntimeHandler,
		}
		if res.Status.Metadata != nil {
			resp.Status.Metadata = &pb.PodSandboxMetadata{
				Name:      res.Status.Metadata.Name,
				Uid:       res.Status.Metadata.UID,
				Namespace: res.Status.Metadata.Namespace,
				Attempt:   res.Status.Metadata.Attempt,
			}
		}
		if res.Status.Network != nil {
			resp.Status.Network = &pb.PodSandboxNetworkStatus{
				Ip: res.Status.Network.IP,
			}
			additionalIps := []*pb.PodIP{}
			for _, x := range res.Status.Network.AdditionalIps {
				additionalIps = append(additionalIps, &pb.PodIP{Ip: x.IP})
			}
			resp.Status.Network.AdditionalIps = additionalIps
		}
		if res.Status.Linux != nil {
			resp.Status.Linux = &pb.LinuxPodSandboxStatus{}
			if res.Status.Linux.Namespaces != nil {
				resp.Status.Linux.Namespaces = &pb.Namespace{}
				if res.Status.Linux.Namespaces.Options != nil {
					resp.Status.Linux.Namespaces.Options = &pb.NamespaceOption{
						Network:  pb.NamespaceMode(res.Status.Linux.Namespaces.Options.Network),
						Pid:      pb.NamespaceMode(res.Status.Linux.Namespaces.Options.Pid),
						Ipc:      pb.NamespaceMode(res.Status.Linux.Namespaces.Options.Ipc),
						TargetId: res.Status.Linux.Namespaces.Options.TargetID,
					}
				}
			}
		}
	}
	return resp, nil
}
