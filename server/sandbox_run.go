package server

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/containers/storage/pkg/idtools"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/pkg/annotations"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	"k8s.io/api/core/v1"
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

// trustedSandbox returns true if the sandbox will run trusted workloads.
func (s *Server) trustedSandbox(req *pb.RunPodSandboxRequest) bool {
	kubeAnnotations := req.GetConfig().GetAnnotations()

	trustedAnnotation, ok := kubeAnnotations[annotations.TrustedSandbox]
	if !ok {
		// A sandbox is trusted by default.
		return true
	}

	return isTrue(trustedAnnotation)
}

func (s *Server) createContainer(container *oci.Container, infraContainer *oci.Container, cgroupParent string) error {
	intermediateMountPoint := container.IntermediateMountPoint()

	if intermediateMountPoint == "" {
		return s.Runtime().CreateContainer(container, cgroupParent)
	}

	errc := make(chan error)
	go func() {
		// We create a new mount namespace before running the container as the rootfs of the
		// container is accessible only to the root user.  We use the intermediate mount
		// namespace to bind mount the root to a directory that is accessible to the user which
		// maps to root inside of the container/
		// We carefully unlock the OS thread only if no errors happened.  The thread might have failed
		// to restore the original mount namespace, and unlocking it will let it keep running
		// in a different context than the other threads.  A thread that is still locked when the
		// goroutine terminates is automatically destroyed.
		var err error
		runtime.LockOSThread()
		defer func() {
			if err == nil {
				runtime.UnlockOSThread()
			}
			errc <- err
		}()

		fd, err := os.Open(fmt.Sprintf("/proc/%d/task/%d/ns/mnt", os.Getpid(), unix.Gettid()))
		if err != nil {
			return
		}
		defer fd.Close()

		// create a new mountns on the current thread
		if err = unix.Unshare(unix.CLONE_NEWNS); err != nil {
			return
		}
		defer unix.Setns(int(fd.Fd()), unix.CLONE_NEWNS)

		// don't spread our mounts around
		err = unix.Mount("/", "/", "none", unix.MS_REC|unix.MS_SLAVE, "")
		if err != nil {
			return
		}

		rootUID, rootGID, err := idtools.GetRootUIDGID(container.IDMappings().UIDs(), container.IDMappings().GIDs())
		if err != nil {
			return
		}

		err = os.Chown(intermediateMountPoint, rootUID, rootGID)
		if err != nil {
			return
		}

		mountPoint := container.MountPoint()
		err = os.Chown(mountPoint, rootUID, rootGID)
		if err != nil {
			return
		}

		rootPath := filepath.Join(intermediateMountPoint, "root")
		err = idtools.MkdirAllAs(rootPath, 0700, rootUID, rootGID)
		if err != nil {
			return
		}

		err = unix.Mount(mountPoint, rootPath, "none", unix.MS_BIND, "")
		if err != nil {
			return
		}

		if infraContainer != nil {
			infraRunDir := filepath.Join(intermediateMountPoint, "infra-rundir")
			err = idtools.MkdirAllAs(infraRunDir, 0700, rootUID, rootGID)
			if err != nil {
				return
			}

			err = unix.Mount(infraContainer.BundlePath(), infraRunDir, "none", unix.MS_BIND, "")
			if err != nil {
				return
			}
			err = os.Chown(infraRunDir, rootUID, rootGID)
			if err != nil {
				return
			}
		}

		runDirPath := filepath.Join(intermediateMountPoint, "rundir")
		err = os.MkdirAll(runDirPath, 0700)
		if err != nil {
			return
		}

		err = unix.Mount(container.BundlePath(), runDirPath, "none", unix.MS_BIND, "suid")
		if err != nil {
			return
		}
		err = os.Chown(runDirPath, rootUID, rootGID)
		if err != nil {
			return
		}

		err = s.Runtime().CreateContainer(container, cgroupParent)
	}()

	err := <-errc
	return err
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
	if in == nil {
		return nil
	}
	out := make([]*hostport.PortMapping, len(in))
	for i, v := range in {
		out[i] = &hostport.PortMapping{
			HostPort:      v.HostPort,
			ContainerPort: v.ContainerPort,
			Protocol:      v1.Protocol(v.Protocol.String()),
			HostIP:        v.HostIp,
		}
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

func getSELinuxLabels(selinuxOptions *pb.SELinuxOption, privileged bool) (processLabel string, mountLabel string, err error) {
	if privileged {
		return "", "", nil
	}
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
	return label.InitLabels(labels)
}

// convertCgroupFsNameToSystemd converts an expanded cgroupfs name to its systemd name.
// For example, it will convert test.slice/test-a.slice/test-a-b.slice to become test-a-b.slice
// NOTE: this is public right now to allow its usage in dockermanager and dockershim, ideally both those
// code areas could use something from libcontainer if we get this style function upstream.
func convertCgroupFsNameToSystemd(cgroupfsName string) (string, error) {
	// TODO: see if libcontainer systemd implementation could use something similar, and if so, move
	// this function up to that library.  At that time, it would most likely do validation specific to systemd
	// above and beyond the simple assumption here that the base of the path encodes the hierarchy
	// per systemd convention.
	return path.Base(cgroupfsName), nil
}
