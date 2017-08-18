package libpod

import (
	"fmt"

	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/libpod/ctr"
	"github.com/kubernetes-incubator/cri-o/libpod/pod"
)

var (
	runtimeNotImplemented = func(rt *Runtime) error {
		return fmt.Errorf("NOT IMPLEMENTED")
	}
	ctrNotImplemented = func(c *ctr.Container) error {
		return fmt.Errorf("NOT IMPLEMENTED")
	}
)

const (
	// IPCNamespace represents the IPC namespace
	IPCNamespace = "ipc"
	// MountNamespace represents the mount namespace
	MountNamespace = "mount"
	// NetNamespace represents the network namespace
	NetNamespace = "net"
	// PIDNamespace represents the PID namespace
	PIDNamespace = "pid"
	// UserNamespace represents the user namespace
	UserNamespace = "user"
	// UTSNamespace represents the UTS namespace
	UTSNamespace = "uts"
)

// Runtime Creation Options

// WithStorageConfig uses the given configuration to set up container storage
// If this is not specified, the system default configuration will be used
// instead
func WithStorageConfig(config *storage.StoreOptions) RuntimeOption {
	return runtimeNotImplemented
}

// WithImageConfig uses the given configuration to set up image handling
// If this is not specified, the system default configuration will be used
// instead
func WithImageConfig(defaultTransport string, insecureRegistries, registries []string) RuntimeOption {
	return runtimeNotImplemented
}

// WithSignaturePolicy specifies the path of a file which decides how trust is
// managed for images we've pulled.
// If this is not specified, the system default configuration will be used
// instead
func WithSignaturePolicy(path string) RuntimeOption {
	return runtimeNotImplemented
}

// WithOCIRuntime specifies an OCI runtime to use for running containers
func WithOCIRuntime(runtimePath string) RuntimeOption {
	return runtimeNotImplemented
}

// WithConmonPath specifies the path to the conmon binary which manages the
// runtime
func WithConmonPath(path string) RuntimeOption {
	return runtimeNotImplemented
}

// WithConmonEnv specifies the environment variable list for the conmon process
func WithConmonEnv(environment []string) RuntimeOption {
	return runtimeNotImplemented
}

// WithCgroupManager specifies the manager implementation name which is used to
// handle cgroups for containers
func WithCgroupManager(manager string) RuntimeOption {
	return runtimeNotImplemented
}

// WithSELinux enables SELinux on the container server
func WithSELinux() RuntimeOption {
	return runtimeNotImplemented
}

// WithApparmorProfile specifies the apparmor profile name which will be used as
// the default for created containers
func WithApparmorProfile(profile string) RuntimeOption {
	return runtimeNotImplemented
}

// WithSeccompProfile specifies the seccomp profile which will be used as the
// default for created containers
func WithSeccompProfile(profilePath string) RuntimeOption {
	return runtimeNotImplemented
}

// WithPidsLimit specifies the maximum number of processes each container is
// restricted to
func WithPidsLimit(limit int64) RuntimeOption {
	return runtimeNotImplemented
}

// Container Creation Options

// WithRootFSFromPath uses the given path as a container's root filesystem
// No further setup is performed on this path
func WithRootFSFromPath(path string) CtrCreateOption {
	return ctrNotImplemented
}

// WithRootFSFromImage sets up a fresh root filesystem using the given image
// If useImageConfig is specified, image volumes, environment variables, and
// other configuration from the image will be added to the config
func WithRootFSFromImage(image string, useImageConfig bool) CtrCreateOption {
	return ctrNotImplemented
}

// WithSharedNamespaces sets a container to share namespaces with another
// container. If the from container belongs to a pod, the new container will
// be added to the pod.
// By default no namespaces are shared. To share a namespace, add the Namespace
// string constant to the map as a key
func WithSharedNamespaces(from *ctr.Container, namespaces map[string]string) CtrCreateOption {
	return ctrNotImplemented
}

// WithPod adds the container to a pod
func WithPod(pod *pod.Pod) CtrCreateOption {
	return ctrNotImplemented
}

// WithLabels adds labels to the pod
func WithLabels(labels map[string]string) CtrCreateOption {
	return ctrNotImplemented
}

// WithAnnotations adds annotations to the pod
func WithAnnotations(annotations map[string]string) CtrCreateOption {
	return ctrNotImplemented
}

// WithName sets the container's name
func WithName(name string) CtrCreateOption {
	return ctrNotImplemented
}

// WithStopSignal sets the signal that will be sent to stop the container
func WithStopSignal(signal uint) CtrCreateOption {
	return ctrNotImplemented
}
