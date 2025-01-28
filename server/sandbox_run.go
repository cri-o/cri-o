package server

import (
	"context"
	"os"

	v1 "k8s.io/api/core/v1"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/log"
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
func (s *Server) privilegedSandbox(req *types.RunPodSandboxRequest) bool {
	securityContext := req.Config.Linux.SecurityContext
	if securityContext == nil {
		return false
	}

	if securityContext.Privileged {
		return true
	}

	namespaceOptions := securityContext.NamespaceOptions
	if namespaceOptions == nil {
		return false
	}

	if namespaceOptions.Network == types.NamespaceMode_NODE ||
		namespaceOptions.Pid == types.NamespaceMode_NODE ||
		namespaceOptions.Ipc == types.NamespaceMode_NODE {
		return true
	}

	return false
}

// runtimeHandler returns the runtime handler key provided by CRI if the key
// does exist and the associated data are valid. If the key is empty, there
// is nothing to do, and the empty key is returned. For every other case, this
// function will return an empty string with the error associated.
func (s *Server) runtimeHandler(req *types.RunPodSandboxRequest) (string, error) {
	handler := req.RuntimeHandler
	if handler == "" {
		return handler, nil
	}

	if _, err := s.ContainerServer.Runtime().ValidateRuntimeHandler(handler); err != nil {
		return "", err
	}

	return handler, nil
}

// RunPodSandbox creates and runs a pod-level sandbox.
func (s *Server) RunPodSandbox(ctx context.Context, req *types.RunPodSandboxRequest) (*types.RunPodSandboxResponse, error) {
	// platform dependent call
	return s.runPodSandbox(ctx, req)
}

func convertPortMappings(in []*types.PortMapping) []*hostport.PortMapping {
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

func (s *Server) setPodSandboxMountLabel(ctx context.Context, id, mountLabel string) error {
	_, span := log.StartSpan(ctx)
	defer span.End()

	storageMetadata, err := s.ContainerServer.StorageRuntimeServer().GetContainerMetadata(id)
	if err != nil {
		return err
	}

	storageMetadata.SetMountLabel(mountLabel)

	return s.ContainerServer.StorageRuntimeServer().SetContainerMetadata(id, &storageMetadata)
}
