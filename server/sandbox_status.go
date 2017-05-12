package server

import (
	"time"

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

	podInfraContainer := sb.infraContainer
	if err = s.runtime.UpdateStatus(podInfraContainer); err != nil {
		logrus.Debugf("failed to get sandbox status for %s: %v", sb.id, err)
	}

	cState := s.runtime.ContainerStatus(podInfraContainer)
	created := time.Time{}.UnixNano()
	if cState != nil {
		created = cState.Created.UnixNano()
	}

	netNsPath, err := podInfraContainer.NetNsPath()
	if err != nil {
		return nil, err
	}
	ip, err := s.netPlugin.GetContainerNetworkStatus(netNsPath, sb.namespace, sb.kubeName, sb.id)
	if err != nil {
		// ignore the error on network status
		ip = ""
	}

	rStatus := pb.PodSandboxState_SANDBOX_NOTREADY
	if cState != nil && cState.Status == oci.ContainerStateRunning {
		rStatus = pb.PodSandboxState_SANDBOX_READY
	}

	sandboxID := sb.id
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
			Labels:      sb.labels,
			Annotations: sb.annotations,
			Metadata:    sb.metadata,
		},
	}

	logrus.Infof("PodSandboxStatusResponse: %+v", resp)
	return resp, nil
}
