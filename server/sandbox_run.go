package server

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kubernetes-sigs/cri-o/oci"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
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

	runtime, ok := s.Runtime().(*oci.Runtime)
	if !ok {
		return "", fmt.Errorf("runtime interface conversion error")
	}

	if _, err := runtime.ValidateRuntimeHandler(handler); err != nil {
		return "", err
	}

	return handler, nil
}

var (
	conflictRE = regexp.MustCompile(`already reserved for pod "([0-9a-z]+)"`)
)

func (s *Server) configureIntermediateNamespace(g *generate.Generator, container *oci.Container, infraContainer *oci.Container) error {
	intermediateMountPoint, err := ioutil.TempDir("/var/run/crio", "intermediate-mount")
	if err != nil {
		return errors.Wrapf(err, "failed to create intermediate directory")
	}
	defer func() {
		if err != nil {
			os.RemoveAll(intermediateMountPoint)
		}
	}()

	resolvedIntermediateMountPoint, err := filepath.EvalSymlinks(intermediateMountPoint)
	if err != nil {
		return errors.Wrapf(err, "failed to eval symlinks for %s", intermediateMountPoint)
	}

	container.SetIntermediateMountPoint(resolvedIntermediateMountPoint)

	g.SetRootPath(filepath.Join(resolvedIntermediateMountPoint, "root"))

	newRunDir := filepath.Join(resolvedIntermediateMountPoint, "rundir")
	mounts := g.Mounts()
	g.ClearMounts()
	for _, mount := range mounts {
		if strings.HasPrefix(mount.Source, container.BundlePath()) {
			mount.Source = filepath.Join(newRunDir, mount.Source[len(container.BundlePath()):])
		} else if infraContainer != nil && strings.HasPrefix(mount.Source, infraContainer.BundlePath()) {
			newInfraRunDir := filepath.Join(resolvedIntermediateMountPoint, "infra-rundir")
			mount.Source = filepath.Join(newInfraRunDir, mount.Source[len(infraContainer.BundlePath()):])
		}
		g.AddMount(mount)
	}
	return nil
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
		if v.Protocol != pb.Protocol_TCP && v.Protocol != pb.Protocol_UDP {
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
	return s.StorageRuntimeServer().SetContainerMetadata(id, storageMetadata)
}

func getSELinuxLabels(selinuxOptions *pb.SELinuxOption, privileged bool) (string, string, error) {
	labels := []string{}
	if selinuxOptions != nil {
		if selinuxOptions.User != "" {
			labels = append(labels, "user:"+selinuxOptions.User)
		}
		if selinuxOptions.Role != "" {
			labels = append(labels, "role:"+selinuxOptions.Role)
		}
		if selinuxOptions.Type != "" {
			labels = append(labels, "type:"+selinuxOptions.Type)
		}
		if selinuxOptions.Level != "" {
			labels = append(labels, "level:"+selinuxOptions.Level)
		}
	}
	var (
		processLabel, mountLabel string
		err                      error
	)
	processLabel, mountLabel, err = label.InitLabels(labels)
	if err != nil {
		return "", "", err
	}
	if privileged {
		processLabel = ""
	}
	return processLabel, mountLabel, nil
}

// convertCgroupFsNameToSystemd converts an expanded cgroupfs name to its systemd name.
// For example, it will convert test.slice/test-a.slice/test-a-b.slice to become test-a-b.slice
// NOTE: this is public right now to allow its usage in dockermanager and dockershim, ideally both those
// code areas could use something from libcontainer if we get this style function upstream.
func convertCgroupFsNameToSystemd(cgroupfsName string) string {
	// TODO: see if libcontainer systemd implementation could use something similar, and if so, move
	// this function up to that library.  At that time, it would most likely do validation specific to systemd
	// above and beyond the simple assumption here that the base of the path encodes the hierarchy
	// per systemd convention.
	return path.Base(cgroupfsName)
}
