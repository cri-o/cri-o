package server

import (
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// PodSandboxStatus returns the Status of the PodSandbox.
func (s *Server) PodSandboxStatus(ctx context.Context, req *pb.PodSandboxStatusRequest) (*pb.PodSandboxStatusResponse, error) {
	logrus.Debugf("PodSandboxStatusRequest %+v", req)
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		return nil, err
	}

	podInfraContainer := sb.InfraContainer()
	if err = s.Runtime().UpdateStatus(podInfraContainer); err != nil {
		return nil, err
	}
	s.ContainerStateToDisk(podInfraContainer)

	cState := s.Runtime().ContainerStatus(podInfraContainer)

	netNsPath, err := podInfraContainer.NetNsPath()
	if err != nil {
		return nil, err
	}
	ip, err := s.netPlugin.GetContainerNetworkStatus(netNsPath, sb.Namespace(), sb.KubeName(), sb.ID())
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
			Id:          sandboxID,
			CreatedAt:   podInfraContainer.CreatedAt().UnixNano(),
			Network:     &pb.PodSandboxNetworkStatus{Ip: ip},
			State:       rStatus,
			Labels:      sb.Labels(),
			Annotations: sb.Annotations(),
			Metadata:    sb.Metadata(),
		},
	}

	logrus.Debugf("PodSandboxStatusResponse: %+v", resp)
	return resp, nil
}
