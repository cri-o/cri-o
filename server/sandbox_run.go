package server

import (
	"os"

	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
)

const (
	// PodInfraOOMAdj is the value that we set for oom score adj for
	// the pod infra container.
	// TODO: Remove this const once this value is provided over CRI
	// See https://github.com/kubernetes/kubernetes/issues/47938
	PodInfraOOMAdj int = -998
	// PodInfraCPUshares is default cpu shares for sandbox container.
	PodInfraCPUshares = 2
)

// privilegedSandbox returns true if the sandbox configuration
// requires additional host privileges for the sandbox.
func (s *Server) privilegedSandbox(req *pb.RunPodSandboxRequest) bool {
	securityContext := req.GetConfig().GetLinux().GetSecurityContext()
	if securityContext == nil {
		return false
	}

	if securityContext.Privileged {
		return true
	}

	namespaceOptions := securityContext.GetNamespaceOptions()
	if namespaceOptions == nil {
		return false
	}

	if namespaceOptions.GetNetwork() == pb.NamespaceMode_NODE ||
		namespaceOptions.GetPid() == pb.NamespaceMode_NODE ||
		namespaceOptions.GetIpc() == pb.NamespaceMode_NODE {
		return true
	}

	return false
}

// runtimeHandler returns the runtime handler key provided by CRI if the key
// does exist and the associated data are valid. If the key is empty, there
// is nothing to do, and the empty key is returned. For every other case, this
// function will return an empty string with the error associated.
func (s *Server) runtimeHandler(req *pb.RunPodSandboxRequest) (string, error) {
	handler := req.GetRuntimeHandler()
	if handler == "" {
		return handler, nil
	}

	if _, err := s.Runtime().ValidateRuntimeHandler(handler); err != nil {
		return "", err
	}

	return handler, nil
}

// RunPodSandbox creates and runs a pod-level sandbox.
func (s *Server) RunPodSandbox(ctx context.Context, req *pb.RunPodSandboxRequest) (resp *pb.RunPodSandboxResponse, err error) {
	// platform dependent call
	return s.runPodSandbox(ctx, req)
}

func convertPortMappings(in []*pb.PortMapping) []*hostport.PortMapping {
	out := make([]*hostport.PortMapping, 0, len(in))
	for _, v := range in {
		if v.HostPort <= 0 {
			continue
		}
		out = append(out, &hostport.PortMapping{
			HostPort:      v.HostPort,
			ContainerPort: v.ContainerPort,
			Protocol:      v1.Protocol(v.Protocol.String()),
			HostIP:        v.HostIp,
		})
	}
	return out
}

func getHostname(id, hostname string, hostNetwork bool) (string, error) {
	if hostNetwork {
		if hostname == "" {
			h, err := os.Hostname()
			if err != nil {
				return "", err
			}
			hostname = h
		}
	} else {
		if hostname == "" {
			hostname = id[:12]
		}
	}
	return hostname, nil
}

func (s *Server) setPodSandboxMountLabel(id, mountLabel string) error {
	storageMetadata, err := s.StorageRuntimeServer().GetContainerMetadata(id)
	if err != nil {
		return err
	}
	storageMetadata.SetMountLabel(mountLabel)
	return s.StorageRuntimeServer().SetContainerMetadata(id, &storageMetadata)
}
