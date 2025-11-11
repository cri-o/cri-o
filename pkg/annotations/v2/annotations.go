package v2

const (
	// V2 annotations (recommended format: *.crio.io).

	// Cgroup2MountHierarchyRW specifies mounting v2 cgroups as an rw filesystem.
	Cgroup2MountHierarchyRW = "cgroup2-mount-hierarchy-rw.crio.io"

	// Devices is a set of devices to give to the container.
	Devices = "devices.crio.io"

	// DisableFIPS is used to disable FIPS mode for a pod within a FIPS-enabled Kubernetes cluster.
	DisableFIPS = "disable-fips.crio.io"

	// LinkLogs indicates that CRI-O should link the pod containers logs into the specified
	// emptyDir volume.
	LinkLogs = "link-logs.crio.io"

	// PlatformRuntimePath indicates the runtime path that CRI-O should use for a specific platform.
	PlatformRuntimePath = "platform-runtime-path.crio.io"

	// PodLinuxOverhead indicates the overheads associated with the pod.
	PodLinuxOverhead = "pod-linux-overhead.crio.io"

	// PodLinuxResources indicates the sum of container resources for this pod.
	PodLinuxResources = "pod-linux-resources.crio.io"

	// SeccompNotifierAction indicates a container is allowed to use the seccomp notifier feature.
	SeccompNotifierAction = "seccomp-notifier-action.crio.io"

	// SeccompProfile can be used to set the seccomp profile for:
	// - a specific container by using: `seccomp-profile.crio.io/<CONTAINER_NAME>`
	// - a whole pod by using: `seccomp-profile.crio.io/POD`
	// Note that the annotation works on containers as well as on images.
	// For images, the plain annotation `seccomp-profile.crio.io`
	// can be used without the required `/POD` suffix or a container name.
	SeccompProfile = "seccomp-profile.crio.io"

	// ShmSize is the annotation used to set custom shm size.
	ShmSize = "shm-size.crio.io"

	// Spoofed indicates a container was spoofed in the runtime.
	Spoofed = "spoofed.crio.io"

	// TrySkipVolumeSELinuxLabel is the annotation used for optionally skipping relabeling a volume
	// with the specified SELinux label.  The relabeling will be skipped if the top layer is already labeled correctly.
	TrySkipVolumeSELinuxLabel = "try-skip-volume-selinux-label.crio.io"

	// Umask is the umask to use in the container init process.
	Umask = "umask.crio.io"

	// UnifiedCgroup specifies the unified configuration for cgroup v2.
	UnifiedCgroup = "unified-cgroup.crio.io"

	// UsernsMode is the user namespace mode to use.
	UsernsMode = "userns-mode.crio.io"

	// V1 annotations (deprecated, legacy format: io.kubernetes.cri-o.*)
	// These are kept in v2 package to avoid circular dependencies.

	// V1UsernsMode is the deprecated V1 version of UsernsMode.
	//
	// Deprecated: Use UsernsMode instead.
	V1UsernsMode = "io.kubernetes.cri-o.userns-mode"

	// V1Cgroup2MountHierarchyRW is the deprecated V1 version of Cgroup2MountHierarchyRW.
	//
	// Deprecated: Use Cgroup2MountHierarchyRW instead.
	V1Cgroup2MountHierarchyRW = "io.kubernetes.cri-o.cgroup2-mount-hierarchy-rw"

	// V1UnifiedCgroup is the deprecated V1 version of UnifiedCgroup.
	//
	// Deprecated: Use UnifiedCgroup instead.
	V1UnifiedCgroup = "io.kubernetes.cri-o.UnifiedCgroup"

	// V1Spoofed is the deprecated V1 version of Spoofed.
	//
	// Deprecated: Use Spoofed instead.
	V1Spoofed = "io.kubernetes.cri-o.Spoofed"

	// V1ShmSize is the deprecated V1 version of ShmSize.
	//
	// Deprecated: Use ShmSize instead.
	V1ShmSize = "io.kubernetes.cri-o.ShmSize"

	// V1Devices is the deprecated V1 version of Devices.
	//
	// Deprecated: Use Devices instead.
	V1Devices = "io.kubernetes.cri-o.Devices"

	// V1TrySkipVolumeSELinuxLabel is the deprecated V1 version of TrySkipVolumeSELinuxLabel.
	//
	// Deprecated: Use TrySkipVolumeSELinuxLabel instead.
	V1TrySkipVolumeSELinuxLabel = "io.kubernetes.cri-o.TrySkipVolumeSELinuxLabel"

	// V1SeccompNotifierAction is the deprecated V1 version of SeccompNotifierAction.
	//
	// Deprecated: Use SeccompNotifierAction instead.
	V1SeccompNotifierAction = "io.kubernetes.cri-o.seccompNotifierAction"

	// V1Umask is the deprecated V1 version of Umask.
	//
	// Deprecated: Use Umask instead.
	V1Umask = "io.kubernetes.cri-o.umask"

	// V1PodLinuxOverhead is the deprecated V1 version of PodLinuxOverhead.
	//
	// Deprecated: Use PodLinuxOverhead instead.
	V1PodLinuxOverhead = "io.kubernetes.cri-o.PodLinuxOverhead"

	// V1PodLinuxResources is the deprecated V1 version of PodLinuxResources.
	//
	// Deprecated: Use PodLinuxResources instead.
	V1PodLinuxResources = "io.kubernetes.cri-o.PodLinuxResources"

	// V1LinkLogs is the deprecated V1 version of LinkLogs.
	//
	// Deprecated: Use LinkLogs instead.
	V1LinkLogs = "io.kubernetes.cri-o.LinkLogs"

	// V1PlatformRuntimePath is the deprecated V1 version of PlatformRuntimePath.
	//
	// Deprecated: Use PlatformRuntimePath instead.
	V1PlatformRuntimePath = "io.kubernetes.cri-o.PlatformRuntimePath"

	// V1SeccompProfile is the deprecated V1 version of SeccompProfile.
	//
	// Deprecated: Use SeccompProfile instead.
	V1SeccompProfile = "seccomp-profile.kubernetes.cri-o.io"

	// V1DisableFIPS is the deprecated V1 version of DisableFIPS.
	//
	// Deprecated: Use DisableFIPS instead.
	V1DisableFIPS = "io.kubernetes.cri-o.DisableFIPS"

	// SeccompNotifierActionStop indicates that a container should be stopped if used via the SeccompNotifierAction annotation.
	SeccompNotifierActionStop = "stop"
)

// reverseAnnotationMigrationMap maps V2 annotations to their V1 equivalents.
// This is used for efficient backwards compatibility lookups.
var reverseAnnotationMigrationMap = map[string]string{
	Cgroup2MountHierarchyRW:   V1Cgroup2MountHierarchyRW,
	Devices:                   V1Devices,
	DisableFIPS:               V1DisableFIPS,
	LinkLogs:                  V1LinkLogs,
	PlatformRuntimePath:       V1PlatformRuntimePath,
	PodLinuxOverhead:          V1PodLinuxOverhead,
	PodLinuxResources:         V1PodLinuxResources,
	SeccompNotifierAction:     V1SeccompNotifierAction,
	SeccompProfile:            V1SeccompProfile,
	ShmSize:                   V1ShmSize,
	Spoofed:                   V1Spoofed,
	TrySkipVolumeSELinuxLabel: V1TrySkipVolumeSELinuxLabel,
	Umask:                     V1Umask,
	UnifiedCgroup:             V1UnifiedCgroup,
	UsernsMode:                V1UsernsMode,
}

// GetAnnotationValue returns the value for a V2 annotation, checking both the new V2 format
// and the deprecated V1 format for backwards compatibility. The V2 annotation is preferred.
// Returns the value and a boolean indicating whether the annotation was found.
//
// This function handles both base annotations (e.g., "userns-mode.crio.io") and container-specific
// annotations (e.g., "unified-cgroup.crio.io/containerName" or "seccomp-profile.crio.io/containerName").
func GetAnnotationValue(annotations map[string]string, newKey string) (string, bool) {
	value, _, found := GetAnnotationValueWithKey(annotations, newKey)

	return value, found
}

// GetAnnotationValueWithKey returns the value for a V2 annotation, checking both the new V2 format
// and the deprecated V1 format for backwards compatibility. The V2 annotation is preferred.
// Returns the value, the actual key that was found, and a boolean indicating whether the annotation was found.
//
// This function handles both base annotations (e.g., "userns-mode.crio.io") and container-specific
// annotations (e.g., "unified-cgroup.crio.io/containerName" or "seccomp-profile.crio.io/containerName").
func GetAnnotationValueWithKey(annotations map[string]string, newKey string) (value, key string, found bool) {
	// Prefer V2 annotation
	if value, ok := annotations[newKey]; ok {
		return value, newKey, true
	}

	// Fall back to V1 annotation if it exists
	if oldKey, ok := reverseAnnotationMigrationMap[newKey]; ok {
		if value, ok := annotations[oldKey]; ok {
			return value, oldKey, true
		}
	}

	// Handle container-specific annotations (e.g., "unified-cgroup.crio.io/containerName")
	// Try to extract the base key and suffix to construct the V1 equivalent
	oldKey := findV1KeyForContainerSpecific(newKey)
	if oldKey != "" {
		if value, ok := annotations[oldKey]; ok {
			return value, oldKey, true
		}
	}

	return "", "", false
}

// findV1KeyForContainerSpecific attempts to find the V1 annotation key for container-specific
// annotations by checking if the key starts with a known V2 base annotation.
func findV1KeyForContainerSpecific(newKey string) string {
	for v2Base, v1Base := range reverseAnnotationMigrationMap {
		// Check for slash-separated pattern (e.g., "unified-cgroup.crio.io/containerName")
		if len(newKey) > len(v2Base)+1 && newKey[:len(v2Base)] == v2Base && newKey[len(v2Base)] == '/' {
			suffix := newKey[len(v2Base):]

			return v1Base + suffix
		}
		// For backwards compatibility, also check for dot-separated pattern (deprecated)
		// This supports migration from the earlier implementation that used dots
		if len(newKey) > len(v2Base)+1 && newKey[:len(v2Base)] == v2Base && newKey[len(v2Base)] == '.' {
			suffix := newKey[len(v2Base):]

			return v1Base + suffix
		}
	}

	return ""
}

// AllAnnotations lists all V2 annotations.
var AllAnnotations = []string{
	Cgroup2MountHierarchyRW,
	Devices,
	DisableFIPS,
	LinkLogs,
	PlatformRuntimePath,
	PodLinuxOverhead,
	PodLinuxResources,
	SeccompNotifierAction,
	SeccompProfile,
	ShmSize,
	Spoofed,
	TrySkipVolumeSELinuxLabel,
	Umask,
	UnifiedCgroup,
	UsernsMode,
}

// AllV1Annotations lists all deprecated V1 annotations for backwards compatibility.
var AllV1Annotations = []string{
	V1Cgroup2MountHierarchyRW,
	V1Devices,
	V1DisableFIPS,
	V1LinkLogs,
	V1PlatformRuntimePath,
	V1PodLinuxOverhead,
	V1PodLinuxResources,
	V1SeccompNotifierAction,
	V1SeccompProfile,
	V1ShmSize,
	V1Spoofed,
	V1TrySkipVolumeSELinuxLabel,
	V1Umask,
	V1UnifiedCgroup,
	V1UsernsMode,
}
