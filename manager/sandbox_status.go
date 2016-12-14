package manager

import (
	"github.com/kubernetes-incubator/cri-o/oci"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// PodSandboxStatus returns the Status of the PodSandbox.
func (m *Manager) PodSandboxStatus(sbID string) (*pb.PodSandboxStatus, error) {
	sb, err := m.getPodSandboxWithPartialID(sbID)
	if err != nil {
		return nil, err
	}

	podInfraContainer := sb.infraContainer
	if err = m.runtime.UpdateStatus(podInfraContainer); err != nil {
		return nil, err
	}

	cState := m.runtime.ContainerStatus(podInfraContainer)
	created := cState.Created.UnixNano()

	netNsPath, err := podInfraContainer.NetNsPath()
	if err != nil {
		return nil, err
	}
	podNamespace := ""
	ip, err := m.netPlugin.GetContainerNetworkStatus(netNsPath, podNamespace, sb.id, podInfraContainer.Name())
	if err != nil {
		// ignore the error on network status
		ip = ""
	}

	rStatus := pb.PodSandboxState_SANDBOX_NOTREADY
	if cState.Status == oci.ContainerStateRunning {
		rStatus = pb.PodSandboxState_SANDBOX_READY
	}

	sandboxID := sb.id
	status := &pb.PodSandboxStatus{
		Id:        &sandboxID,
		CreatedAt: int64Ptr(created),
		Linux: &pb.LinuxPodSandboxStatus{
			Namespaces: &pb.Namespace{
				Network: sPtr(netNsPath),
			},
		},
		Network:     &pb.PodSandboxNetworkStatus{Ip: &ip},
		State:       &rStatus,
		Labels:      sb.labels,
		Annotations: sb.annotations,
		Metadata:    sb.metadata,
	}

	return status, nil
}
