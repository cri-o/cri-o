package annotations

import (
	"github.com/intel/goresctrl/pkg/rdt"

	v2 "github.com/cri-o/cri-o/pkg/annotations/v2"
)

// GetAnnotationValue returns the value for a V2 annotation, checking both the new V2 format
// and the deprecated V1 format for backwards compatibility. The V2 annotation is preferred.
// Returns the value and a boolean indicating whether the annotation was found.
//
// This function handles both base annotations (e.g., "userns-mode.crio.io") and container-specific
// annotations (e.g., "unified-cgroup.crio.io/containerName" or "seccomp-profile.crio.io/containerName").
//
// Deprecated: Import and use v2.GetAnnotationValue instead. This wrapper is kept for backwards compatibility.
func GetAnnotationValue(annotations map[string]string, newKey string) (string, bool) {
	return v2.GetAnnotationValue(annotations, newKey)
}

const (
	// V1 annotations (deprecated) - re-exported from v2 package for backwards compatibility.

	// Deprecated: Use v2.V1UsernsMode or preferably v2.UsernsMode instead.
	// UsernsMode is the user namespace mode to use.
	UsernsModeAnnotation = v2.V1UsernsMode

	// Deprecated: Use v2.V1Cgroup2MountHierarchyRW or preferably v2.Cgroup2MountHierarchyRW instead.
	// CgroupRW specifies mounting v2 cgroups as an rw filesystem.
	Cgroup2RWAnnotation = v2.V1Cgroup2MountHierarchyRW

	// Deprecated: Use v2.V1UnifiedCgroup or preferably v2.UnifiedCgroup instead.
	// UnifiedCgroupAnnotation specifies the unified configuration for cgroup v2.
	UnifiedCgroupAnnotation = v2.V1UnifiedCgroup

	// Deprecated: Use v2.V1Spoofed or preferably v2.Spoofed instead.
	// SpoofedContainer indicates a container was spoofed in the runtime.
	SpoofedContainer = v2.V1Spoofed

	// Deprecated: Use v2.V1ShmSize or preferably v2.ShmSize instead.
	// ShmSizeAnnotation is the K8S annotation used to set custom shm size.
	ShmSizeAnnotation = v2.V1ShmSize

	// Deprecated: Use v2.V1Devices or preferably v2.Devices instead.
	// DevicesAnnotation is a set of devices to give to the container.
	DevicesAnnotation = v2.V1Devices

	// Deprecated: Use v2.CPULoadBalancing instead.
	// CPULoadBalancingAnnotation indicates that load balancing should be disabled for CPUs used by the container.
	CPULoadBalancingAnnotation = v2.CPULoadBalancing

	// Deprecated: Use v2.CPUQuota instead.
	// CPUQuotaAnnotation indicates that CPU quota should be disabled for CPUs used by the container.
	CPUQuotaAnnotation = v2.CPUQuota

	// Deprecated: Use v2.IRQLoadBalancing instead.
	// IRQLoadBalancingAnnotation controls IRQ load balancing for container CPUs.
	// Set to "disable" to turn off IRQ balancing on all container CPUs.
	// Set to "housekeeping" to preserve interrupts on the first CPU core and its siblings, but to turn off on all other
	// container CPUs.
	IRQLoadBalancingAnnotation = v2.IRQLoadBalancing

	// Deprecated: Use v2.OCISeccompBPFHook instead.
	// OCISeccompBPFHookAnnotation is the annotation used by the OCI seccomp BPF hook for tracing container syscalls.
	OCISeccompBPFHookAnnotation = v2.OCISeccompBPFHook

	// Deprecated: Use v2.V1TrySkipVolumeSELinuxLabel or preferably v2.TrySkipVolumeSELinuxLabel instead.
	// TrySkipVolumeSELinuxLabelAnnotation is the annotation used for optionally skipping relabeling a volume
	// with the specified SELinux label.  The relabeling will be skipped if the top layer is already labeled correctly.
	TrySkipVolumeSELinuxLabelAnnotation = v2.V1TrySkipVolumeSELinuxLabel

	// Deprecated: Use v2.CPUCStates instead.
	// CPUCStatesAnnotation indicates that c-states should be enabled or disabled for CPUs used by the container.
	CPUCStatesAnnotation = v2.CPUCStates

	// Deprecated: Use v2.CPUFreqGovernor instead.
	// CPUFreqGovernorAnnotation sets the cpufreq governor for CPUs used by the container.
	CPUFreqGovernorAnnotation = v2.CPUFreqGovernor

	// Deprecated: Use v2.CPUShared instead.
	// CPUSharedAnnotation indicate that a container which is part of a guaranteed QoS pod,
	// wants access to shared cpus.
	// the container name should be appended at the end of the annotation
	// example:  cpu-shared.crio.io/containerA
	CPUSharedAnnotation = v2.CPUShared

	// Deprecated: Use v2.V1SeccompNotifierAction or preferably v2.SeccompNotifierAction instead.
	// SeccompNotifierActionAnnotation indicates a container is allowed to use the seccomp notifier feature.
	SeccompNotifierActionAnnotation = v2.V1SeccompNotifierAction

	// Deprecated: Use v2.V1Umask or preferably v2.Umask instead.
	// UmaskAnnotation is the umask to use in the container init process.
	UmaskAnnotation = v2.V1Umask

	// Deprecated: Use v2.SeccompNotifierActionStop instead.
	// SeccompNotifierActionStop indicates that a container should be stopped if used via the SeccompNotifierActionAnnotation key.
	SeccompNotifierActionStop = v2.SeccompNotifierActionStop

	// Deprecated: Use v2.V1PodLinuxOverhead or preferably v2.PodLinuxOverhead instead.
	// PodLinuxOverhead indicates the overheads associated with the pod.
	PodLinuxOverhead = v2.V1PodLinuxOverhead

	// Deprecated: Use v2.V1PodLinuxResources or preferably v2.PodLinuxResources instead.
	// PodLinuxResources indicates the sum of container resources for this pod.
	PodLinuxResources = v2.V1PodLinuxResources

	// Deprecated: Use v2.V1LinkLogs or preferably v2.LinkLogs instead.
	// LinkLogsAnnotations indicates that CRI-O should link the pod containers logs into the specified
	// emptyDir volume.
	LinkLogsAnnotation = v2.V1LinkLogs

	// Deprecated: Use v2.V1PlatformRuntimePath or preferably v2.PlatformRuntimePath instead.
	// PlatformRuntimePath indicates the runtime path that CRI-O should use for a specific platform.
	PlatformRuntimePath = v2.V1PlatformRuntimePath

	// Deprecated: Use v2.V1SeccompProfile or preferably v2.SeccompProfile instead.
	// SeccompProfileAnnotation can be used to set the seccomp profile for:
	// - a specific container by using: `seccomp-profile.kubernetes.cri-o.io/<CONTAINER_NAME>`
	// - a whole pod by using: `seccomp-profile.kubernetes.cri-o.io/POD`
	// Note that the annotation works on containers as well as on images.
	// For images, the plain annotation `seccomp-profile.kubernetes.cri-o.io`
	// can be used without the required `/POD` suffix or a container name.
	SeccompProfileAnnotation = v2.V1SeccompProfile

	// Deprecated: Use v2.V1DisableFIPS or preferably v2.DisableFIPS instead.
	// DisableFIPSAnnotation is used to disable FIPS mode for a pod within a FIPS-enabled Kubernetes cluster.
	DisableFIPSAnnotation = v2.V1DisableFIPS

	// Deprecated: Use v2.StopSignal instead.
	// StopSignalAnnotation represents the stop signal used for the image
	// this key is defined in image-spec conversion document at https://github.com/opencontainers/image-spec/pull/492/files#diff-8aafbe2c3690162540381b8cdb157112R57
	StopSignalAnnotation = v2.StopSignal
)

var AllAllowedAnnotations = append(
	append(
		[]string{
			// External annotations
			rdt.RdtContainerAnnotation,

			// Keep in sync with
			// https://github.com/opencontainers/runc/blob/3db0871f1cf25c7025861ba0d51d25794cb21623/features.go#L67
			// Once runc 1.2 is released, we can use the `runc features` command to get this programmatically,
			// but we should hardcode these for now to prevent misuse.
			"bundle",
			"org.systemd.property.",
			"org.criu.config",

			// Similarly, keep in sync with
			// https://github.com/containers/crun/blob/475a3fd0be/src/libcrun/container.c#L362-L366
			"module.wasm.image/variant",
			"io.kubernetes.cri.container-type",
			"run.oci.",
		},
		// V2 annotations (recommended)
		v2.AllAnnotations...,
	),
	// V1 annotations (deprecated, kept for backwards compatibility)
	v2.AllV1Annotations...,
)
