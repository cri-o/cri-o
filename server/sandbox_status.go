package server

import (
	"time"

	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// PodSandboxStatus returns the Status of the PodSandbox.
func (s *Server) PodSandboxStatus(ctx context.Context, req *pb.PodSandboxStatusRequest) (resp *pb.PodSandboxStatusResponse, err error) {
	const operation = "pod_sandbox_status"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("PodSandboxStatusRequest %+v", req)
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		return nil, err
	}

	podInfraContainer := sb.InfraContainer()
	cState := s.Runtime().ContainerStatus(podInfraContainer)

	rStatus := pb.PodSandboxState_SANDBOX_NOTREADY
	if cState.Status == oci.ContainerStateRunning {
		rStatus = pb.PodSandboxState_SANDBOX_READY
	}

	sandboxID := sb.ID()
	resp = &pb.PodSandboxStatusResponse{
		Status: &pb.PodSandboxStatus{
			Id:          sandboxID,
			CreatedAt:   podInfraContainer.CreatedAt().UnixNano(),
			Network:     &pb.PodSandboxNetworkStatus{Ip: sb.IP()},
			State:       rStatus,
			Labels:      sb.Labels(),
			Annotations: sb.Annotations(),
			Metadata:    sb.Metadata(),
		},
	}

	logrus.Debugf("PodSandboxStatusResponse: %+v", resp)
	return resp, nil
}
