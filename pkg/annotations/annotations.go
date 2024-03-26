package annotations

import (
	"github.com/intel/goresctrl/pkg/rdt"
)

const (
	// UsernsMode is the user namespace mode to use
	UsernsModeAnnotation = "io.kubernetes.cri-o.userns-mode"

	// CgroupRW specifies mounting v2 cgroups as an rw filesystem.
	Cgroup2RWAnnotation = "io.kubernetes.cri-o.cgroup2-mount-hierarchy-rw"

	// UnifiedCgroupAnnotation specifies the unified configuration for cgroup v2
	UnifiedCgroupAnnotation = "io.kubernetes.cri-o.UnifiedCgroup"

	// SpoofedContainer indicates a container was spoofed in the runtime
	SpoofedContainer = "io.kubernetes.cri-o.Spoofed"

	// ShmSizeAnnotation is the K8S annotation used to set custom shm size
	ShmSizeAnnotation = "io.kubernetes.cri-o.ShmSize"

	// DevicesAnnotation is a set of devices to give to the container
	DevicesAnnotation = "io.kubernetes.cri-o.Devices"

	// CPULoadBalancingAnnotation indicates that load balancing should be disabled for CPUs used by the container
	CPULoadBalancingAnnotation = "cpu-load-balancing.crio.io"

	// CPUQuotaAnnotation indicates that CPU quota should be disabled for CPUs used by the container
	CPUQuotaAnnotation = "cpu-quota.crio.io"

	// IRQLoadBalancingAnnotation indicates that IRQ load balancing should be disabled for CPUs used by the container
	IRQLoadBalancingAnnotation = "irq-load-balancing.crio.io"

	// OCISeccompBPFHookAnnotation is the annotation used by the OCI seccomp BPF hook for tracing container syscalls
	OCISeccompBPFHookAnnotation = "io.containers.trace-syscall"

	// TrySkipVolumeSELinuxLabelAnnotation is the annotation used for optionally skipping relabeling a volume
	// with the specified SELinux label.  The relabeling will be skipped if the top layer is already labeled correctly.
	TrySkipVolumeSELinuxLabelAnnotation = "io.kubernetes.cri-o.TrySkipVolumeSELinuxLabel"

	// CPUCStatesAnnotation indicates that c-states should be enabled or disabled for CPUs used by the container
	CPUCStatesAnnotation = "cpu-c-states.crio.io"

	// CPUFreqGovernorAnnotation sets the cpufreq governor for CPUs used by the container
	CPUFreqGovernorAnnotation = "cpu-freq-governor.crio.io"
)

var AllAllowedAnnotations = []string{
	UsernsModeAnnotation,
	Cgroup2RWAnnotation,
	UnifiedCgroupAnnotation,
	ShmSizeAnnotation,
	DevicesAnnotation,
	CPULoadBalancingAnnotation,
	CPUQuotaAnnotation,
	IRQLoadBalancingAnnotation,
	OCISeccompBPFHookAnnotation,
	rdt.RdtContainerAnnotation,
	TrySkipVolumeSELinuxLabelAnnotation,
	CPUCStatesAnnotation,
	CPUFreqGovernorAnnotation,
	// Keep in sync with
	// https://github.com/opencontainers/runc/blob/3db0871f1cf25c7025861ba0d51d25794cb21623/features.go#L67
	// Once runc 1.2 is released, we can use the `runc features` command to get this programatically,
	// but we should hardcode these for now to prevent misuse.
	"bundle",
	"org.systemd.property.",
	"org.criu.config",

	// Simiarly, keep in sync with
	// https://github.com/containers/crun/blob/475a3fd0be/src/libcrun/container.c#L362-L366
	"module.wasm.image/variant",
	"io.kubernetes.cri.container-type",
	"run.oci.",
}
