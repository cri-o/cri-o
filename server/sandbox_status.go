package server

import (
	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// PodSandboxStatus returns the Status of the PodSandbox.
func (s *Server) PodSandboxStatus(ctx context.Context, req *pb.PodSandboxStatusRequest) (*pb.PodSandboxStatusResponse, error) {
	logrus.Debugf("PodSandboxStatusRequest %+v", req)
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		return nil, err
	}

	podInfraContainer := sb.InfraContainer()
	if err = s.runtime.UpdateStatus(podInfraContainer); err != nil {
		return nil, err
	}

	cState := s.runtime.ContainerStatus(podInfraContainer)
	created := cState.Created.UnixNano()

	netNsPath, err := podInfraContainer.NetNsPath()
	if err != nil {
		return nil, err
	}
	podNamespace := ""
	ip, err := s.netPlugin.GetContainerNetworkStatus(netNsPath, podNamespace, sb.ID(), podInfraContainer.Name())
	if err != nil {
		// ignore the error on network status
		ip = ""
	}

	rStatus := pb.PodSandboxState_SANDBOX_NOTREADY
	if cState.Status == oci.ContainerStateRunning {
		rStatus = pb.PodSandboxState_SANDBOX_READY
	}

	sandboxID := sb.ID()
	resp := &pb.PodSandboxStatusResponse{
		Status: &pb.PodSandboxStatus{
			Id:        sandboxID,
			CreatedAt: created,
			Linux: &pb.LinuxPodSandboxStatus{
				Namespaces: &pb.Namespace{
					Network: netNsPath,
				},
			},
			Network:     &pb.PodSandboxNetworkStatus{Ip: ip},
			State:       rStatus,
			Labels:      sb.Labels(),
			Annotations: sb.Annotations(),
			Metadata:    sb.Metadata(),
		},
	}

	logrus.Infof("PodSandboxStatusResponse: %+v", resp)
	return resp, nil
}
