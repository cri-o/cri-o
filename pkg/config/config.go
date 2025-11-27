package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	conmonrsClient "github.com/containers/conmon-rs/pkg/client"
	cpConfig "github.com/cri-o/crio-credential-provider/pkg/config"
	"github.com/cri-o/ocicni/pkg/ocicni"
	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go/features"
	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/sirupsen/logrus"
	"go.podman.io/common/pkg/hooks"
	"go.podman.io/image/v5/pkg/sysregistriesv2"
	"go.podman.io/image/v5/types"
	"go.podman.io/storage"
	"k8s.io/utils/cpuset"
	"k8s.io/utils/ptr"
	"tags.cncf.io/container-device-interface/pkg/cdi"

	"github.com/cri-o/cri-o/internal/config/apparmor"
	"github.com/cri-o/cri-o/internal/config/blockio"
	"github.com/cri-o/cri-o/internal/config/capabilities"
	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/config/cnimgr"
	"github.com/cri-o/cri-o/internal/config/conmonmgr"
	"github.com/cri-o/cri-o/internal/config/device"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/config/nri"
	"github.com/cri-o/cri-o/internal/config/nsmgr"
	"github.com/cri-o/cri-o/internal/config/rdt"
	"github.com/cri-o/cri-o/internal/config/seccomp"
	"github.com/cri-o/cri-o/internal/config/ulimits"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/storage/references"
	"github.com/cri-o/cri-o/pkg/annotations"
	"github.com/cri-o/cri-o/server/metrics/collectors"
	"github.com/cri-o/cri-o/server/useragent"
	"github.com/cri-o/cri-o/utils"
	"github.com/cri-o/cri-o/utils/cmdrunner"
)

// Defaults if none are specified.
const (
	defaultGRPCMaxMsgSize = 80 * 1024 * 1024
	// default minimum memory for all other runtimes.
	defaultContainerMinMemory = 12 * 1024 * 1024 // 12 MiB
	// defaultContainerCreateTimeout is the default timeout for container creation operations in seconds.
	defaultContainerCreateTimeout = 240
	// minimumContainerCreateTimeout is the minimum allowed timeout for container creation operations in seconds.
	minimumContainerCreateTimeout = 30
	// minimum memory for crun, the default runtime.
	defaultContainerMinMemoryCrun = 500 * 1024 // 500 KiB
	OCIBufSize                    = 8192
	RuntimeTypeVM                 = "vm"
	RuntimeTypePod                = "pod"
	defaultCtrStopTimeout         = 30 // seconds
	defaultNamespacesDir          = "/var/run"
	RuntimeTypeVMBinaryPattern    = "containerd-shim-([a-zA-Z0-9\\-\\+])+-v2"
	tasksetBinary                 = "taskset"
	MonitorExecCgroupDefault      = ""
	MonitorExecCgroupContainer    = "container"
)

// Config represents the entire set of configuration values that can be set for
// the server. This is intended to be loaded from a toml-encoded config file.
type Config struct {
	RootConfig
	APIConfig
	RuntimeConfig
	ImageConfig
	NetworkConfig
	MetricsConfig
	TracingConfig
	StatsConfig

	Comment          string
	singleConfigPath string // Path to the single config file
	dropInConfigDir  string // Path to the drop-in config files

	NRI           *nri.Config
	SystemContext *types.SystemContext
}

// Iface provides a config interface for data encapsulation.
type Iface interface {
	GetStore() (storage.Store, error)
	GetData() *Config
}

// GetData returns the Config of a Iface.
func (c *Config) GetData() *Config {
	return c
}

// ImageVolumesType describes image volume handling strategies.
type ImageVolumesType string

const (
	// ImageVolumesMkdir option is for using mkdir to handle image volumes.
	ImageVolumesMkdir ImageVolumesType = "mkdir"
	// ImageVolumesIgnore option is for ignoring image volumes altogether.
	ImageVolumesIgnore ImageVolumesType = "ignore"
	// ImageVolumesBind option is for using bind mounted volumes.
)

const (
	// DefaultPidsLimit is the default value for maximum number of processes
	// allowed inside a container.
	DefaultPidsLimit = -1

	// DefaultLogSizeMax is the default value for the maximum log size
	// allowed for a container. Negative values mean that no limit is imposed.
	DefaultLogSizeMax = -1
)

const (
	// DefaultBlockIOConfigFile is the default value for blockio controller configuration file.
	DefaultBlockIOConfigFile = ""
	// DefaultBlockIOReload is the default value for reloading blockio with changed config file and block devices.
	DefaultBlockIOReload = false
)

const (
	// DefaultIrqBalanceConfigFile default irqbalance service configuration file path.
	DefaultIrqBalanceConfigFile = "/etc/sysconfig/irqbalance"
	// DefaultIrqBalanceConfigRestoreFile contains the banned cpu mask configuration to restore. Name due to backward compatibility.
	DefaultIrqBalanceConfigRestoreFile = "/etc/sysconfig/orig_irq_banned_cpus"
)

// This structure is necessary to fake the TOML tables when parsing,
// while also not requiring a bunch of layered structs for no good
// reason.

// RootConfig represents the root of the "crio" TOML config table.
type RootConfig struct {
	// Root is a path to the "root directory" where data not
	// explicitly handled by other options will be stored.
	Root string `toml:"root"`

	// RunRoot is a path to the "run directory" where state information not
	// explicitly handled by other options will be stored.
	RunRoot string `toml:"runroot"`

	// ImageStore if set it will allow end-users to store newly pulled image
	// in path provided by `ImageStore` instead of path provided in `Root`.
	ImageStore string `toml:"imagestore"`

	// Storage is the name of the storage driver which handles actually
	// storing the contents of containers.
	Storage string `toml:"storage_driver"`

	// StorageOption is a list of storage driver specific options.
	StorageOptions []string `toml:"storage_option"`

	// PullOptions is a map of pull options that are passed to the storage driver.
	pullOptions map[string]string

	// LogDir is the default log directory where all logs will go unless kubelet
	// tells us to put them somewhere else.
	LogDir string `toml:"log_dir"`

	// VersionFile is the location CRI-O will lay down the version file
	// that checks whether we've rebooted
	VersionFile string `toml:"version_file"`

	// VersionFilePersist is the location CRI-O will lay down the version file
	// that checks whether we've upgraded
	VersionFilePersist string `toml:"version_file_persist"`

	// CleanShutdownFile is the location CRI-O will lay down the clean shutdown file
	// that checks whether we've had time to sync before shutting down
	CleanShutdownFile string `toml:"clean_shutdown_file"`

	// InternalWipe is whether CRI-O should wipe containers and images after a reboot when the server starts.
	// If set to false, one must use the external command `crio wipe` to wipe the containers and images in these situations.
	// The option InternalWipe is deprecated, and will be removed in a future release.
	InternalWipe bool `toml:"internal_wipe"`

	// InternalRepair is used to repair the affected images.
	InternalRepair bool `toml:"internal_repair"`
}

// GetStore returns the container storage for a given configuration.
func (c *RootConfig) GetStore() (storage.Store, error) {
	return storage.GetStore(storage.StoreOptions{
		RunRoot:            c.RunRoot,
		GraphRoot:          c.Root,
		ImageStore:         c.ImageStore,
		GraphDriverName:    c.Storage,
		GraphDriverOptions: c.StorageOptions,
		PullOptions:        c.pullOptions,
	})
}

// runtimeHandlerFeatures represents the supported features of the runtime.
type runtimeHandlerFeatures struct {
	features.Features

	RecursiveReadOnlyMounts bool `json:"-"` // Internal use only.
}

// RuntimeHandler represents each item of the "crio.runtime.runtimes" TOML
// config table.
type RuntimeHandler struct {
	RuntimeConfigPath string `toml:"runtime_config_path"`
	RuntimePath       string `toml:"runtime_path"`
	RuntimeType       string `toml:"runtime_type"`
	RuntimeRoot       string `toml:"runtime_root"`

	// PrivilegedWithoutHostDevices can be used to restrict passing host devices
	// to a container running as privileged.
	PrivilegedWithoutHostDevices bool `toml:"privileged_without_host_devices,omitempty"`
	// AllowedAnnotations is a slice of experimental annotations that this runtime handler is allowed to process.
	// The currently recognized values are:
	// "io.kubernetes.cri-o.userns-mode" for configuring a user namespace for the pod.
	// "io.kubernetes.cri-o.Devices" for configuring devices for the pod.
	// "io.kubernetes.cri-o.ShmSize" for configuring the size of /dev/shm.
	// "io.kubernetes.cri-o.UnifiedCgroup.$CTR_NAME" for configuring the cgroup v2 unified block for a container.
	// "io.containers.trace-syscall" for tracing syscalls via the OCI seccomp BPF hook.
	// "io.kubernetes.cri-o.LinkLogs" for linking logs into the pod.
	// "seccomp-profile.kubernetes.cri-o.io" for setting the seccomp profile for:
	//   - a specific container by using: `seccomp-profile.kubernetes.cri-o.io/<CONTAINER_NAME>`
	//   - a whole pod by using: `seccomp-profile.kubernetes.cri-o.io/POD`
	//   Note that the annotation works on containers as well as on images.
	//   For images, the plain annotation `seccomp-profile.kubernetes.cri-o.io`
	//   can be used without the required `/POD` suffix or a container name.
	// "io.kubernetes.cri-o.DisableFIPS" for disabling FIPS mode for a pod within a FIPS-enabled Kubernetes cluster.
	AllowedAnnotations []string `toml:"allowed_annotations,omitempty"`

	// DisallowedAnnotations is the slice of experimental annotations that are not allowed for this handler.
	DisallowedAnnotations []string `toml:"-"`

	// Fields prefixed by Monitor hold the configuration for the monitor for this runtime. At present, the following monitors are supported:
	// oci supports conmon
	// vm does not support any runtime monitor
	MonitorPath   string   `toml:"monitor_path,omitempty"`
	MonitorCgroup string   `toml:"monitor_cgroup,omitempty"`
	MonitorEnv    []string `toml:"monitor_env,omitempty"`

	// MonitorExecCgroup indicates whether to move exec probes to the container's cgroup.
	MonitorExecCgroup string `toml:"monitor_exec_cgroup,omitempty"`

	// PlatformRuntimePaths defines a configuration option that specifies
	// the runtime paths for different platforms.
	PlatformRuntimePaths map[string]string `toml:"platform_runtime_paths,omitempty"`

	// Marks the runtime as performing image pulling on its own, and doesn't
	// require crio to do it.
	RuntimePullImage bool `toml:"runtime_pull_image,omitempty"`

	// ContainerMinMemory is the minimum memory that must be set for a container.
	ContainerMinMemory string `toml:"container_min_memory,omitempty"`

	// NoSyncLog if enabled will disable fsync on log rotation and container exit.
	// This can improve performance but may result in data loss on hard system crashes.
	NoSyncLog bool `toml:"no_sync_log"`

	// Output of the "features" subcommand.
	// This is populated dynamically and not read from config.
	features runtimeHandlerFeatures

	// Inheritance request
	// Fill in the Runtime information (paths and type) from the default runtime
	InheritDefaultRuntime bool `toml:"inherit_default_runtime,omitempty"`

	// Default annotations specified for runtime handler if they're not overridden by
	// the pod spec.
	DefaultAnnotations map[string]string `toml:"default_annotations,omitempty"`

	// StreamWebsockets can be used to enable the WebSocket protocol for
	// container exec, attach and port forward.
	//
	// conmon-rs (runtime_type = "pod") supports this configuration for exec
	// and attach. Forwarding ports will be supported in future releases.
	StreamWebsockets bool `toml:"stream_websockets,omitempty"`

	// ExecCPUAffinity specifies which CPU is used when exec-ing the container.
	// The valid values are:
	// "":
	//   Use runtime default.
	// "first":
	//   When it has only exclusive cpuset, use the first CPU in the exclusive cpuset.
	//   When it has both shared and exclusive cpusets, use first CPU in the shared cpuset.
	ExecCPUAffinity ExecCPUAffinityType `toml:"exec_cpu_affinity,omitempty"`

	// SeccompProfile is the absolute path of the seccomp.json profile which is used as the
	// default for the runtime. This configuration takes precedence over runtime config seccomp_profile.
	// If set to "", the runtime config seccomp_profile will be used.
	// If that is also set to "", the internal default seccomp profile will be applied.
	SeccompProfile string `toml:"seccomp_profile,omitempty"`

	// ContainerCreateTimeout is the timeout for container creation operations in seconds.
	// If not set, defaults to 240 seconds.
	ContainerCreateTimeout int64 `toml:"container_create_timeout,omitempty"`

	// seccompConfig is the seccomp configuration for the handler.
	seccompConfig *seccomp.Config
}

type ExecCPUAffinityType string

const (
	ExecCPUAffinityTypeDefault   ExecCPUAffinityType = ""
	ExecCPUAffinityTypeFirst     ExecCPUAffinityType = "first"
	runtimeSeccompProfileDefault string              = ""
)

// Multiple runtime Handlers in a map.
type Runtimes map[string]*RuntimeHandler

// RuntimeConfig represents the "crio.runtime" TOML config table.
type RuntimeConfig struct {
	// NoPivot instructs the runtime to not use `pivot_root`, but instead use `MS_MOVE`
	NoPivot bool `toml:"no_pivot"`

	// SELinux determines whether or not SELinux is used for pod separation.
	SELinux bool `toml:"selinux"`

	// Whether container output should be logged to journald in addition
	// to the kubernetes log file
	LogToJournald bool `toml:"log_to_journald"`

	// DropInfraCtr determines whether the infra container is dropped when appropriate.
	DropInfraCtr bool `toml:"drop_infra_ctr"`

	// ReadOnly run all pods/containers in read-only mode.
	// This mode will mount tmpfs on /run, /tmp and /var/tmp, if those are not mountpoints
	// Will also set the readonly flag in the OCI Runtime Spec.  In this mode containers
	// will only be able to write to volumes mounted into them
	ReadOnly bool `toml:"read_only"`

	// ConmonEnv is the environment variable list for conmon process.
	// This option is currently deprecated, and will be replaced with RuntimeHandler.MonitorEnv.
	ConmonEnv []string `toml:"conmon_env"`

	// HooksDir holds paths to the directories containing hooks
	// configuration files.  When the same filename is present in
	// multiple directories, the file in the directory listed last in
	// this slice takes precedence.
	HooksDir []string `toml:"hooks_dir"`

	// Capabilities to add to all containers.
	DefaultCapabilities capabilities.Capabilities `toml:"default_capabilities"`

	// AddInheritableCapabilities can be set to add inheritable capabilities. They were pre-1.23 by default, and were dropped in 1.24.
	// This can cause a regression with non-root users not getting capabilities as they previously did.
	AddInheritableCapabilities bool `toml:"add_inheritable_capabilities"`

	// Additional environment variables to set for all the
	// containers. These are overridden if set in the
	// container image spec or in the container runtime configuration.
	DefaultEnv []string `toml:"default_env"`

	// Sysctls to add to all containers.
	DefaultSysctls []string `toml:"default_sysctls"`

	// DefaultUlimits specifies the default ulimits to apply to containers
	DefaultUlimits []string `toml:"default_ulimits"`

	// Devices that are allowed to be configured.
	AllowedDevices []string `toml:"allowed_devices"`

	// Devices to add to containers
	AdditionalDevices []string `toml:"additional_devices"`

	// CDISpecDirs specifies the directories CRI-O/CDI will scan for CDI Spec files.
	CDISpecDirs []string `toml:"cdi_spec_dirs"`

	// DeviceOwnershipFromSecurityContext changes the default behavior of setting container devices uid/gid
	// from CRI's SecurityContext (RunAsUser/RunAsGroup) instead of taking host's uid/gid. Defaults to false.
	DeviceOwnershipFromSecurityContext bool `toml:"device_ownership_from_security_context"`

	// DefaultRuntime is the _name_ of the OCI runtime to be used as the default.
	// The name is matched against the Runtimes map below.
	DefaultRuntime string `toml:"default_runtime"`

	// DecryptionKeysPath is the path where keys for image decryption are stored.
	DecryptionKeysPath string `toml:"decryption_keys_path"`

	// Conmon is the path to conmon binary, used for managing the runtime.
	// This option is currently deprecated, and will be replaced with RuntimeHandler.MonitorConfig.Path.
	Conmon string `toml:"conmon"`

	// ConmonCgroup is the cgroup setting used for conmon.
	// This option is currently deprecated, and will be replaced with RuntimeHandler.MonitorConfig.Cgroup.
	ConmonCgroup string `toml:"conmon_cgroup"`

	// SeccompProfile is the seccomp.json profile path which is used as the
	// default for the runtime.
	// If set to "" or not found, the internal default seccomp profile will be used.
	SeccompProfile string `toml:"seccomp_profile"`

	// PrivilegedSeccompProfile can be set to enable a seccomp profile for
	// privileged containers from the local path.
	PrivilegedSeccompProfile string `toml:"privileged_seccomp_profile"`

	// ApparmorProfile is the apparmor profile name which is used as the
	// default for the runtime.
	ApparmorProfile string `toml:"apparmor_profile"`

	// BlockIOConfigFile is the path to the blockio class configuration
	// file for configuring the cgroup blockio controller.
	BlockIOConfigFile string `toml:"blockio_config_file"`

	// BlockIOReload instructs the runtime to reload blockio configuration
	// rescan block devices in the system before assigning blockio parameters.
	BlockIOReload bool `toml:"blockio_reload"`

	// IrqBalanceConfigFile is the irqbalance service config file which is used
	// for configuring irqbalance daemon.
	IrqBalanceConfigFile string `toml:"irqbalance_config_file"`

	// RdtConfigFile is the RDT config file used for configuring resctrl fs
	RdtConfigFile string `toml:"rdt_config_file"`

	// CgroupManagerName is the manager implementation name which is used to
	// handle cgroups for containers.
	CgroupManagerName string `toml:"cgroup_manager"`

	// DefaultMountsFile is the file path for the default mounts to be mounted for the container
	// Note, for testing purposes mainly
	DefaultMountsFile string `toml:"default_mounts_file"`

	// ContainerExitsDir is the directory in which container exit files are
	// written to by conmon.
	ContainerExitsDir string `toml:"container_exits_dir"`

	// ContainerAttachSocketDir is the location for container attach sockets.
	ContainerAttachSocketDir string `toml:"container_attach_socket_dir"`

	// BindMountPrefix is the prefix to use for the source of the bind mounts.
	BindMountPrefix string `toml:"bind_mount_prefix"`

	// UIDMappings specifies the UID mappings to have in the user namespace.
	// A range is specified in the form containerUID:HostUID:Size.  Multiple
	// ranges are separated by comma.
	UIDMappings string `toml:"uid_mappings"`

	// MinimumMappableUID specifies the minimum UID value which can be
	// specified in a uid_mappings value, whether configured here or sent
	// to us via CRI, for a pod that isn't to be run as UID 0.
	MinimumMappableUID int64 `toml:"minimum_mappable_uid"`

	// GIDMappings specifies the GID mappings to have in the user namespace.
	// A range is specified in the form containerUID:HostUID:Size.  Multiple
	// ranges are separated by comma.
	GIDMappings string `toml:"gid_mappings"`

	// MinimumMappableGID specifies the minimum GID value which can be
	// specified in a gid_mappings value, whether configured here or sent
	// to us via CRI, for a pod that isn't to be run as UID 0.
	MinimumMappableGID int64 `toml:"minimum_mappable_gid"`

	// LogLevel determines the verbosity of the logs based on the level it is set to.
	// Options are fatal, panic, error (default), warn, info, debug, and trace.
	LogLevel string `toml:"log_level"`

	// LogFilter specifies a regular expression to filter the log messages
	LogFilter string `toml:"log_filter"`

	// NamespacesDir is the directory where the state of the managed namespaces
	// gets tracked
	NamespacesDir string `toml:"namespaces_dir"`

	// PinNSPath is the path to find the pinns binary, which is needed
	// to manage namespace lifecycle
	PinnsPath string `toml:"pinns_path"`

	// CriuPath is the path to find the criu binary, which is needed
	// to checkpoint and restore containers
	EnableCriuSupport bool `toml:"enable_criu_support"`

	// Runtimes defines a list of OCI compatible runtimes. The runtime to
	// use is picked based on the runtime_handler provided by the CRI. If
	// no runtime_handler is provided, the runtime will be picked based on
	// the level of trust of the workload.
	Runtimes Runtimes `toml:"runtimes"`

	// Workloads defines a list of workloads types that are have grouped settings
	// that will be applied to containers.
	Workloads Workloads `toml:"workloads"`

	// PidsLimit is the number of processes each container is restricted to
	// by the cgroup process number controller.
	PidsLimit int64 `toml:"pids_limit"`

	// LogSizeMax is the maximum number of bytes after which the log file
	// will be truncated. It can be expressed as a human-friendly string
	// that is parsed to bytes.
	// Negative values indicate that the log file won't be truncated.
	LogSizeMax int64 `toml:"log_size_max"`

	// CtrStopTimeout specifies the time to wait before to generate an
	// error because the container state is still tagged as "running".
	CtrStopTimeout int64 `toml:"ctr_stop_timeout"`

	// SeparatePullCgroup specifies whether an image pull must be performed in a separate cgroup
	SeparatePullCgroup string `toml:"separate_pull_cgroup"`

	// InfraCtrCPUSet is the CPUs set that will be used to run infra containers
	InfraCtrCPUSet string `toml:"infra_ctr_cpuset"`

	// SharedCPUSet is the CPUs set that will be used for guaranteed containers that
	// want access to shared cpus.
	SharedCPUSet string `toml:"shared_cpuset"`

	// AbsentMountSourcesToReject is a list of paths that, when absent from the host,
	// will cause a container creation to fail (as opposed to the current behavior of creating a directory).
	AbsentMountSourcesToReject []string `toml:"absent_mount_sources_to_reject"`

	// EnablePodEvents specifies if the container pod-level events should be generated to optimize the PLEG at Kubelet.
	EnablePodEvents bool `toml:"enable_pod_events"`

	// IrqBalanceConfigRestoreFile is the irqbalance service banned CPU list to restore.
	// If empty, no restoration attempt will be done.
	IrqBalanceConfigRestoreFile string `toml:"irqbalance_config_restore_file"`

	// seccompConfig is the internal seccomp configuration
	seccompConfig *seccomp.Config

	// apparmorConfig is the internal AppArmor configuration
	apparmorConfig *apparmor.Config

	// blockioConfig is the internal blockio configuration
	blockioConfig *blockio.Config

	// rdtConfig is the internal Rdt configuration
	rdtConfig *rdt.Config

	// ulimitConfig is the internal ulimit configuration
	ulimitsConfig *ulimits.Config

	// deviceConfig is the internal additional devices configuration
	deviceConfig *device.Config

	// cgroupManager is the internal CgroupManager configuration
	cgroupManager cgmgr.CgroupManager

	// conmonManager is the internal ConmonManager configuration
	conmonManager *conmonmgr.ConmonManager

	// namespaceManager is the internal NamespaceManager configuration
	namespaceManager *nsmgr.NamespaceManager

	// Whether SELinux should be disabled within a pod,
	// when it is running in the host network namespace
	// https://github.com/cri-o/cri-o/issues/5501
	HostNetworkDisableSELinux bool `toml:"hostnetwork_disable_selinux"`

	// Option to disable hostport mapping in CRI-O
	// Default value is 'false'
	DisableHostPortMapping bool `toml:"disable_hostport_mapping"`

	// Option to set the timezone inside the container.
	// Use 'Local' to match the timezone of the host machine.
	Timezone string `toml:"timezone"`
}

// ImageConfig represents the "crio.image" TOML config table.
type ImageConfig struct {
	// DefaultTransport is a value we prefix to image names that fail to
	// validate source references.
	DefaultTransport string `toml:"default_transport"`
	// GlobalAuthFile is a path to a file like /var/lib/kubelet/config.json
	// containing credentials necessary for pulling images from secure
	// registries.
	GlobalAuthFile string `toml:"global_auth_file"`
	// NamespacedAuthDir is the root path for pod namespace-separated
	// auth files, which is intended to be used together with CRI-O's credential provider:
	// https://github.com/cri-o/crio-credential-provider
	// The namespaced auth file will be <NAMESPACED_AUTH_DIR>/<NAMESPACE>-<IMAGE_NAME_SHA256>.json,
	// where CRI-O moves them into a dedicated location to mark them as "used" during image pull:
	// <NAMESPACED_AUTH_DIR>/in-use/<NAMESPACE>-<IMAGE_NAME_SHA256>-<UUID>.json
	// Note that image name provided to the credential provider does not
	// contain any specific tag or digest, only the normalized repository
	// as well as the image name, which can cause races if the same image
	// prefix get's pulled on a single node.
	// This temporary auth file will be used instead of any configured GlobalAuthFile.
	// If no pod namespace is being provided on image pull (via the sandbox
	// config), or the concatenated path is non existent, then the system wide
	// auth file will be used as fallback.
	// Must be an absolute path.
	NamespacedAuthDir string `toml:"namespaced_auth_dir"`
	// PauseImage is the name of an image on a registry which we use to instantiate infra
	// containers. It should start with a registry host name.
	// Format is enforced by validation.
	PauseImage string `toml:"pause_image"`
	// PauseImageAuthFile, if not empty, is a path to a file like
	// /var/lib/kubelet/config.json containing credentials necessary
	// for pulling PauseImage
	PauseImageAuthFile string `toml:"pause_image_auth_file"`
	// PauseCommand is the path of the binary we run in an infra
	// container that's been instantiated using PauseImage.
	PauseCommand string `toml:"pause_command"`
	// PinnedImages is a list of container images that should be pinned
	// and not subject to garbage collection by kubelet.
	// Pinned images will remain in the container runtime's storage until
	// they are manually removed. Default value: empty list (no images pinned)
	PinnedImages []string `toml:"pinned_images"`
	// SignaturePolicyPath is the name of the file which decides what sort
	// of policy we use when deciding whether or not to trust an image that
	// we've pulled.  Outside of testing situations, it is strongly advised
	// that this be left unspecified so that the default system-wide policy
	// will be used.
	SignaturePolicyPath string `toml:"signature_policy"`
	// SignaturePolicyDir is the root path for pod namespace-separated
	// signature policies. The final policy to be used on image pull will be
	// <SIGNATURE_POLICY_DIR>/<NAMESPACE>.json.
	// If no pod namespace is being provided on image pull (via the sandbox
	// config), or the concatenated path is non existent, then the
	// SignaturePolicyPath or system wide policy will be used as fallback.
	// Must be an absolute path.
	SignaturePolicyDir string `toml:"signature_policy_dir"`
	// InsecureRegistries is a list of registries that must be contacted w/o
	// TLS verification.
	//
	// Deprecated: it's no longer effective. Please use `insecure` in `registries.conf` instead.
	InsecureRegistries []string `toml:"insecure_registries"`
	// ImageVolumes controls how volumes specified in image config are handled
	ImageVolumes ImageVolumesType `toml:"image_volumes"`
	// Temporary directory for big files
	BigFilesTemporaryDir string `toml:"big_files_temporary_dir"`
	// AutoReloadRegistries if set to true, will automatically
	// reload the mirror registry when there is an update to the
	// 'registries.conf.d' directory.
	AutoReloadRegistries bool `toml:"auto_reload_registries"`
	// PullProgressTimeout is the timeout for an image pull to make progress
	// until the pull operation gets canceled. This value will be also used for
	// calculating the pull progress interval to pullProgressTimeout / 10.
	// Can be set to 0 to disable the timeout as well as the progress output.
	PullProgressTimeout time.Duration `toml:"pull_progress_timeout"`
	// OCIArtifactMountSupport is used to determine if CRI-O should support OCI Artifacts.
	OCIArtifactMountSupport bool `toml:"oci_artifact_mount_support"`
	// ShortNameMode describes the mode of short name resolution.
	// The valid values are "enforcing" and "disabled".
	// If "enforcing", an image pull will fail if a short name is used, but the results are ambiguous.
	// If "disabled", the first result will be chosen.
	ShortNameMode string `toml:"short_name_mode"`
}

// NetworkConfig represents the "crio.network" TOML config table.
type NetworkConfig struct {
	// CNIDefaultNetwork is the default CNI network name to be selected
	CNIDefaultNetwork string `toml:"cni_default_network"`

	// NetworkDir is where CNI network configuration files are stored.
	NetworkDir string `toml:"network_dir"`

	// PluginDir is where CNI plugin binaries are stored.
	PluginDir string `toml:"plugin_dir,omitempty"`

	// PluginDirs is where CNI plugin binaries are stored.
	PluginDirs []string `toml:"plugin_dirs"`

	// cniManager manages the internal ocicni plugin
	cniManager *cnimgr.CNIManager
}

// APIConfig represents the "crio.api" TOML config table.
type APIConfig struct {
	// GRPCMaxSendMsgSize is the maximum grpc send message size in bytes.
	GRPCMaxSendMsgSize int `toml:"grpc_max_send_msg_size"`

	// GRPCMaxRecvMsgSize is the maximum grpc receive message size in bytes.
	GRPCMaxRecvMsgSize int `toml:"grpc_max_recv_msg_size"`

	// Listen is the path to the AF_LOCAL socket on which cri-o will listen.
	// This may support proto://addr formats later, but currently this is just
	// a path.
	Listen string `toml:"listen"`

	// StreamAddress is the IP address on which the stream server will listen.
	StreamAddress string `toml:"stream_address"`

	// StreamPort is the port on which the stream server will listen.
	StreamPort string `toml:"stream_port"`

	// StreamEnableTLS enables encrypted tls transport of the stream server
	StreamEnableTLS bool `toml:"stream_enable_tls"`

	// StreamTLSCert is the x509 certificate file path used to serve the encrypted stream
	StreamTLSCert string `toml:"stream_tls_cert"`

	// StreamTLSKey is the key file path used to serve the encrypted stream
	StreamTLSKey string `toml:"stream_tls_key"`

	// StreamTLSCA is the x509 CA(s) file used to verify and authenticate client
	// communication with the tls encrypted stream
	StreamTLSCA string `toml:"stream_tls_ca"`

	// StreamIdleTimeout is how long to leave idle connections open for
	StreamIdleTimeout string `toml:"stream_idle_timeout"`
}

// MetricsConfig specifies all necessary configuration for Prometheus based
// metrics retrieval.
type MetricsConfig struct {
	// EnableMetrics can be used to globally enable or disable metrics support
	EnableMetrics bool `toml:"enable_metrics"`

	// MetricsCollectors specifies enabled metrics collectors.
	MetricsCollectors collectors.Collectors `toml:"metrics_collectors"`

	// MetricsHost is the IP address or hostname on which the metrics server will listen.
	MetricsHost string `toml:"metrics_host"`

	// MetricsPort is the port on which the metrics server will listen.
	MetricsPort int `toml:"metrics_port"`

	// Local socket path to bind the metrics server to
	MetricsSocket string `toml:"metrics_socket"`

	// MetricsCert is the certificate for the secure metrics server.
	MetricsCert string `toml:"metrics_cert"`

	// MetricsKey is the certificate key for the secure metrics server.
	MetricsKey string `toml:"metrics_key"`
}

// TracingConfig specifies all necessary configuration for opentelemetry trace exports.
type TracingConfig struct {
	// EnableTracing can be used to globally enable or disable tracing support
	EnableTracing bool `toml:"enable_tracing"`

	// TracingEndpoint is the address on which the grpc tracing collector server will listen.
	TracingEndpoint string `toml:"tracing_endpoint"`

	// TracingSamplingRatePerMillion is the number of samples to collect per million spans. Set to 1000000 to always sample.
	// Defaults to 0.
	TracingSamplingRatePerMillion int `toml:"tracing_sampling_rate_per_million"`
}

// StatsConfig specifies all necessary configuration for reporting container/pod stats
// and pod sandbox metrics.
type StatsConfig struct {
	// StatsCollectionPeriod is the number of seconds between collecting pod and container stats.
	// If set to 0, the stats are collected on-demand instead.
	StatsCollectionPeriod int `toml:"stats_collection_period"`

	// CollectionPeriod is the number of seconds between collecting pod/container stats
	// and pod sandbox metrics. If set to 0, the metrics/stats are collected on-demand instead.
	CollectionPeriod int `toml:"collection_period"`

	// IncludedPodMetrics specifies the list of metrics to include when collecting pod metrics.
	// If empty, all available metrics will be collected.
	IncludedPodMetrics []string `toml:"included_pod_metrics"`
}

// tomlConfig is another way of looking at a Config, which is
// TOML-friendly (it has all of the explicit tables). It's just used for
// conversions.
type tomlConfig struct {
	Crio struct {
		RootConfig

		API     struct{ APIConfig }     `toml:"api"`
		Runtime struct{ RuntimeConfig } `toml:"runtime"`
		Image   struct{ ImageConfig }   `toml:"image"`
		Network struct{ NetworkConfig } `toml:"network"`
		Metrics struct{ MetricsConfig } `toml:"metrics"`
		Tracing struct{ TracingConfig } `toml:"tracing"`
		Stats   struct{ StatsConfig }   `toml:"stats"`
		NRI     struct{ *nri.Config }   `toml:"nri"`
	} `toml:"crio"`
}

// SetSystemContext configures the SystemContext used by containers/image library.
func (t *tomlConfig) SetSystemContext(c *Config) {
	c.SystemContext.BigFilesTemporaryDir = c.BigFilesTemporaryDir
	c.SystemContext.ShortNameMode = ptr.To(types.ShortNameModeEnforcing)

	if c.ShortNameMode == "disabled" {
		c.SystemContext.ShortNameMode = ptr.To(types.ShortNameModeDisabled)
	}
}

func (t *tomlConfig) toConfig(c *Config) {
	c.Comment = "# "
	c.RootConfig = t.Crio.RootConfig
	c.APIConfig = t.Crio.API.APIConfig
	c.RuntimeConfig = t.Crio.Runtime.RuntimeConfig
	c.ImageConfig = t.Crio.Image.ImageConfig
	c.NetworkConfig = t.Crio.Network.NetworkConfig
	c.MetricsConfig = t.Crio.Metrics.MetricsConfig
	c.TracingConfig = t.Crio.Tracing.TracingConfig
	c.StatsConfig = t.Crio.Stats.StatsConfig
	c.NRI = t.Crio.NRI.Config
	t.SetSystemContext(c)
}

func (t *tomlConfig) fromConfig(c *Config) {
	t.Crio.RootConfig = c.RootConfig
	t.Crio.API.APIConfig = c.APIConfig
	t.Crio.Runtime.RuntimeConfig = c.RuntimeConfig
	t.Crio.Image.ImageConfig = c.ImageConfig
	t.Crio.Network.NetworkConfig = c.NetworkConfig
	t.Crio.Metrics.MetricsConfig = c.MetricsConfig
	t.Crio.Tracing.TracingConfig = c.TracingConfig
	t.Crio.Stats.StatsConfig = c.StatsConfig
	t.Crio.NRI.Config = c.NRI
}

const configLogPrefix = "Updating config from "

// UpdateFromFile populates the Config from the TOML-encoded file at the given
// path and "remembers" that we should reload this file's contents when we
// receive a SIGHUP.
// Returns errors encountered when reading or parsing the files, or nil
// otherwise.
func (c *Config) UpdateFromFile(ctx context.Context, path string) error {
	log.Infof(ctx, configLogPrefix+"single file: %s", path)

	if err := c.UpdateFromDropInFile(ctx, path); err != nil {
		return fmt.Errorf("update config from drop-in file: %w", err)
	}

	c.singleConfigPath = path

	return nil
}

// UpdateFromDropInFile populates the Config from the TOML-encoded file at the
// given path.  The file may be the main configuration file, or it can be one
// of the drop-in files which are used to supplement it.
// Returns errors encountered when reading or parsing the files, or nil
// otherwise.
func (c *Config) UpdateFromDropInFile(ctx context.Context, path string) error {
	log.Infof(ctx, configLogPrefix+"drop-in file: %s", path)
	// keeps the storage options from storage.conf and merge it to crio config
	var storageOpts []string

	storageOpts = append(storageOpts, c.StorageOptions...)
	// storage configurations from storage.conf, if crio config has no values for these, they will be merged to crio config
	graphRoot := c.Root
	runRoot := c.RunRoot
	storageDriver := c.Storage

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	t := new(tomlConfig)
	t.fromConfig(c)

	_, err = toml.Decode(string(data), t)
	if err != nil {
		return fmt.Errorf("unable to decode configuration %v: %w", path, err)
	}

	storageOpts = append(storageOpts, t.Crio.StorageOptions...)
	storageOpts = removeDupStorageOpts(storageOpts)
	t.Crio.StorageOptions = storageOpts
	// inherits storage configurations from storage.conf
	if t.Crio.Root == "" {
		t.Crio.Root = graphRoot
	}

	if t.Crio.RunRoot == "" {
		t.Crio.RunRoot = runRoot
	}

	if t.Crio.Storage == "" {
		t.Crio.Storage = storageDriver
	}

	t.toConfig(c)

	return nil
}

// removeDupStorageOpts removes duplicated storage option from the list
// keeps the last appearance.
func removeDupStorageOpts(storageOpts []string) []string {
	var resOpts []string

	opts := make(map[string]bool)
	for i := len(storageOpts) - 1; i >= 0; i-- {
		if ok := opts[storageOpts[i]]; ok {
			continue
		}

		opts[storageOpts[i]] = true

		resOpts = append(resOpts, storageOpts[i])
	}

	for i, j := 0, len(resOpts)-1; i < j; i, j = i+1, j-1 {
		resOpts[i], resOpts[j] = resOpts[j], resOpts[i]
	}

	return resOpts
}

// UpdateFromPath recursively iterates the provided path and updates the
// configuration for it.
func (c *Config) UpdateFromPath(ctx context.Context, path string) error {
	log.Infof(ctx, configLogPrefix+"path: %s", path)

	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		return nil
	}

	if err := filepath.Walk(path,
		func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			return c.UpdateFromDropInFile(ctx, p)
		}); err != nil {
		return fmt.Errorf("walk path: %w", err)
	}

	c.dropInConfigDir = path

	return nil
}

// ToFile outputs the given Config as a TOML-encoded file at the given path.
// Returns errors encountered when generating or writing the file, or nil
// otherwise.
func (c *Config) ToFile(path string) error {
	b, err := c.ToBytes()
	if err != nil {
		return err
	}

	return os.WriteFile(path, b, 0o644)
}

// ToString encodes the config into a string value.
func (c *Config) ToString() (string, error) {
	configBytes, err := c.ToBytes()
	if err != nil {
		return "", err
	}

	return string(configBytes), nil
}

// ToBytes encodes the config into a byte slice. It errors if the encoding
// fails, which should never happen at all because of general type safeness.
func (c *Config) ToBytes() ([]byte, error) {
	var buffer bytes.Buffer

	e := toml.NewEncoder(&buffer)

	tc := tomlConfig{}
	tc.fromConfig(c)

	if err := e.Encode(tc); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// DefaultConfig returns the default configuration for crio.
func DefaultConfig() (*Config, error) {
	storeOpts, err := storage.DefaultStoreOptions()
	if err != nil {
		return nil, err
	}

	cgroupManager := cgmgr.New()

	ua, err := useragent.Get()
	if err != nil {
		return nil, fmt.Errorf("get user agent: %w", err)
	}

	return &Config{
		Comment: "# ",
		SystemContext: &types.SystemContext{
			DockerRegistryUserAgent: ua,
		},
		RootConfig: RootConfig{
			Root:              storeOpts.GraphRoot,
			RunRoot:           storeOpts.RunRoot,
			ImageStore:        storeOpts.ImageStore,
			Storage:           storeOpts.GraphDriverName,
			StorageOptions:    storeOpts.GraphDriverOptions,
			pullOptions:       storeOpts.PullOptions,
			LogDir:            "/var/log/crio/pods",
			VersionFile:       CrioVersionPathTmp,
			CleanShutdownFile: CrioCleanShutdownFile,
			InternalWipe:      true,
			InternalRepair:    true,
		},
		APIConfig: APIConfig{
			Listen:             CrioSocketPath,
			StreamAddress:      "127.0.0.1",
			StreamPort:         "0",
			GRPCMaxSendMsgSize: defaultGRPCMaxMsgSize,
			GRPCMaxRecvMsgSize: defaultGRPCMaxMsgSize,
		},
		RuntimeConfig: *DefaultRuntimeConfig(cgroupManager),
		ImageConfig: ImageConfig{
			DefaultTransport:        "docker://",
			PauseImage:              DefaultPauseImage,
			PauseCommand:            "/pause",
			ImageVolumes:            ImageVolumesMkdir,
			SignaturePolicyDir:      "/etc/crio/policies",
			PullProgressTimeout:     0,
			OCIArtifactMountSupport: true,
			ShortNameMode:           "enforcing",
			NamespacedAuthDir:       cpConfig.AuthDir,
		},
		NetworkConfig: NetworkConfig{
			NetworkDir: cniConfigDir,
			PluginDirs: []string{cniBinDir},
		},
		MetricsConfig: MetricsConfig{
			MetricsHost:       "127.0.0.1",
			MetricsPort:       9090,
			MetricsCollectors: collectors.All(),
		},
		TracingConfig: TracingConfig{
			TracingEndpoint:               "127.0.0.1:4317",
			TracingSamplingRatePerMillion: 0,
			EnableTracing:                 false,
		},
		NRI: nri.New(),
	}, nil
}

// DefaultRuntimeConfig returns the default Runtime configs.
func DefaultRuntimeConfig(cgroupManager cgmgr.CgroupManager) *RuntimeConfig {
	return &RuntimeConfig{
		AllowedDevices:     []string{"/dev/fuse", "/dev/net/tun"},
		DecryptionKeysPath: "/etc/crio/keys/",
		DefaultRuntime:     DefaultRuntime,
		Runtimes: Runtimes{
			DefaultRuntime: defaultRuntimeHandler(cgroupManager.IsSystemd()),
		},
		SELinux:                     selinuxEnabled(),
		ApparmorProfile:             apparmor.DefaultProfile,
		BlockIOConfigFile:           DefaultBlockIOConfigFile,
		BlockIOReload:               DefaultBlockIOReload,
		IrqBalanceConfigFile:        DefaultIrqBalanceConfigFile,
		RdtConfigFile:               rdt.DefaultRdtConfigFile,
		CgroupManagerName:           cgroupManager.Name(),
		PidsLimit:                   DefaultPidsLimit,
		ContainerExitsDir:           containerExitsDir,
		ContainerAttachSocketDir:    ContainerAttachSocketDir,
		MinimumMappableUID:          -1,
		MinimumMappableGID:          -1,
		LogSizeMax:                  DefaultLogSizeMax,
		CtrStopTimeout:              defaultCtrStopTimeout,
		DefaultCapabilities:         capabilities.Default(),
		LogLevel:                    "info",
		HooksDir:                    []string{hooks.DefaultDir},
		CDISpecDirs:                 cdi.DefaultSpecDirs,
		NamespacesDir:               defaultNamespacesDir,
		DropInfraCtr:                true,
		IrqBalanceConfigRestoreFile: DefaultIrqBalanceConfigRestoreFile,
		seccompConfig:               seccomp.New(),
		apparmorConfig:              apparmor.New(),
		blockioConfig:               blockio.New(),
		cgroupManager:               cgroupManager,
		deviceConfig:                device.New(),
		namespaceManager:            nsmgr.New(defaultNamespacesDir, ""),
		rdtConfig:                   rdt.New(),
		ulimitsConfig:               ulimits.New(),
		HostNetworkDisableSELinux:   true,
		DisableHostPortMapping:      false,
		EnableCriuSupport:           true,
	}
}

// Validate is the main entry point for library configuration validation.
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *Config) Validate(onExecution bool) error {
	switch c.ImageVolumes {
	case ImageVolumesMkdir:
	case ImageVolumesIgnore:
	case ImageVolumesBind:
	default:
		return errors.New("unrecognized image volume type specified")
	}

	if onExecution {
		if err := node.ValidateConfig(); err != nil {
			return err
		}
	}

	if err := c.RootConfig.Validate(onExecution); err != nil {
		return fmt.Errorf("validating root config: %w", err)
	}

	if err := c.RuntimeConfig.Validate(c.SystemContext, onExecution); err != nil {
		return fmt.Errorf("validating runtime config: %w", err)
	}

	c.seccompConfig.SetNotifierPath(
		filepath.Join(filepath.Dir(c.Listen), "seccomp"),
	)

	for name := range c.Runtimes {
		if c.Runtimes[name].seccompConfig != nil {
			c.Runtimes[name].seccompConfig.SetNotifierPath(
				filepath.Join(filepath.Dir(c.Listen), "seccomp"),
			)
		}
	}

	if err := c.ImageConfig.Validate(onExecution); err != nil {
		return fmt.Errorf("validating image config: %w", err)
	}

	if err := c.NetworkConfig.Validate(onExecution); err != nil {
		return fmt.Errorf("validating network config: %w", err)
	}

	if err := c.APIConfig.Validate(onExecution); err != nil {
		return fmt.Errorf("validating api config: %w", err)
	}

	if !c.SELinux {
		selinux.SetDisabled()
	}

	if err := c.NRI.Validate(onExecution); err != nil {
		return fmt.Errorf("validating NRI config: %w", err)
	}

	return nil
}

// Validate is the main entry point for API configuration validation.
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *APIConfig) Validate(onExecution bool) error {
	if c.GRPCMaxSendMsgSize <= 0 {
		c.GRPCMaxSendMsgSize = defaultGRPCMaxMsgSize
	}

	if c.GRPCMaxRecvMsgSize <= 0 {
		c.GRPCMaxRecvMsgSize = defaultGRPCMaxMsgSize
	}

	if c.StreamEnableTLS {
		if c.StreamTLSCert == "" {
			return errors.New("stream TLS cert path is empty")
		}

		if c.StreamTLSKey == "" {
			return errors.New("stream TLS key path is empty")
		}
	}

	if onExecution {
		return RemoveUnusedSocket(c.Listen)
	}

	return nil
}

// RemoveUnusedSocket first ensures that the path to the socket exists and
// removes unused socket connections if available.
func RemoveUnusedSocket(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating socket directories: %w", err)
	}

	// Remove the socket if it already exists
	if _, err := os.Stat(path); err == nil {
		if _, err := net.DialTimeout("unix", path, 0); err == nil {
			return fmt.Errorf("already existing connection on %s", path)
		}

		if err := os.Remove(path); err != nil {
			return fmt.Errorf("removing %s: %w", path, err)
		}
	}

	return nil
}

// Validate is the main entry point for root configuration validation.
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *RootConfig) Validate(onExecution bool) error {
	if onExecution {
		if !filepath.IsAbs(c.LogDir) {
			return errors.New("log_dir is not an absolute path")
		}

		if err := os.MkdirAll(c.LogDir, 0o700); err != nil {
			return fmt.Errorf("invalid log_dir: %w", err)
		}

		store, err := c.GetStore()
		if err != nil {
			return fmt.Errorf("failed to get store to set defaults: %w", err)
		}
		// This step merges the /etc/container/storage.conf with the
		// storage configuration in crio.conf
		// If we don't do this step, we risk returning the incorrect info
		// on Inspect (/info) requests
		c.RunRoot = store.RunRoot()
		c.Root = store.GraphRoot()
		c.Storage = store.GraphDriverName()
		c.StorageOptions = store.GraphOptions()
		c.pullOptions = store.PullOptions()
	}

	return nil
}

func (c *RootConfig) CleanShutdownSupportedFileName() string {
	return c.CleanShutdownFile + ".supported"
}

// Validate is the main entry point for runtime configuration validation
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *RuntimeConfig) Validate(systemContext *types.SystemContext, onExecution bool) error {
	if err := c.ulimitsConfig.LoadUlimits(c.DefaultUlimits); err != nil {
		return err
	}

	if err := c.deviceConfig.LoadDevices(c.AdditionalDevices); err != nil {
		return err
	}

	if err := c.ValidateDefaultRuntime(); err != nil {
		return err
	}

	if c.Timezone != "" && !strings.EqualFold(c.Timezone, "local") {
		_, err := time.LoadLocation(c.Timezone)
		if err != nil {
			return fmt.Errorf("invalid timezone: %s", c.Timezone)
		}
	}

	if c.LogSizeMax >= 0 && c.LogSizeMax < OCIBufSize {
		return fmt.Errorf("log size max should be negative or >= %d", OCIBufSize)
	}

	// We need to ensure the container termination will be properly waited
	// for by defining a minimal timeout value. This will prevent timeout
	// value defined in the configuration file to be too low.
	if c.CtrStopTimeout < defaultCtrStopTimeout {
		c.CtrStopTimeout = defaultCtrStopTimeout
		logrus.Warnf("Forcing ctr_stop_timeout to lowest possible value of %ds", c.CtrStopTimeout)
	}

	if _, err := c.Sysctls(); err != nil {
		return fmt.Errorf("invalid default_sysctls: %w", err)
	}

	if err := c.DefaultCapabilities.Validate(); err != nil {
		return fmt.Errorf("invalid capabilities: %w", err)
	}

	if c.InfraCtrCPUSet != "" {
		set, err := cpuset.Parse(c.InfraCtrCPUSet)
		if err != nil {
			return fmt.Errorf("invalid infra_ctr_cpuset: %w", err)
		}

		executable, err := exec.LookPath(tasksetBinary)
		if err != nil {
			return fmt.Errorf("%q not found in $PATH: %w", tasksetBinary, err)
		}

		cmdrunner.PrependCommandsWith(executable, "--cpu-list", set.String())
	}

	if err := c.Workloads.Validate(); err != nil {
		return fmt.Errorf("workloads validation: %w", err)
	}

	// check for validation on execution
	if onExecution {
		// First, configure cgroup manager so the values of the Runtime.MonitorCgroup can be validated
		cgroupManager, err := cgmgr.SetCgroupManager(c.CgroupManagerName)
		if err != nil {
			return fmt.Errorf("unable to update cgroup manager: %w", err)
		}

		c.cgroupManager = cgroupManager

		if err := c.ValidateRuntimes(); err != nil {
			return fmt.Errorf("runtime validation: %w", err)
		}

		// Validate the system registries configuration
		if _, err := sysregistriesv2.GetRegistries(systemContext); err != nil {
			return fmt.Errorf("invalid registries: %w", err)
		}

		// we should use a hooks directory if
		// it exists and is a directory
		// it does not exist but can be created
		// otherwise, we skip
		hooksDirs := []string{}

		for _, hooksDir := range c.HooksDir {
			if err := utils.IsDirectory(hooksDir); err != nil {
				if !os.IsNotExist(err) {
					logrus.Warnf("Skipping invalid hooks directory: %s exists but is not a directory", hooksDir)

					continue
				}

				if err := os.MkdirAll(hooksDir, 0o755); err != nil {
					logrus.Debugf("Failed to create requested hooks dir: %v", err)

					continue
				}
			}

			logrus.Debugf("Using hooks directory: %s", hooksDir)
			hooksDirs = append(hooksDirs, hooksDir)

			continue
		}

		c.HooksDir = hooksDirs

		if err := cdi.Configure(cdi.WithSpecDirs(c.CDISpecDirs...)); err != nil {
			return err
		}

		// Validate the pinns path
		if err := c.ValidatePinnsPath("pinns"); err != nil {
			return fmt.Errorf("pinns validation: %w", err)
		}

		c.namespaceManager = nsmgr.New(c.NamespacesDir, c.PinnsPath)
		if err := c.namespaceManager.Initialize(); err != nil {
			return fmt.Errorf("initialize nsmgr: %w", err)
		}

		if c.EnableCriuSupport {
			if err := validateCriuInPath(); err != nil {
				c.EnableCriuSupport = false

				logrus.Infof("Checkpoint/restore support disabled: CRIU binary not found int $PATH")
			} else {
				logrus.Infof("Checkpoint/restore support enabled")
			}
		} else {
			logrus.Infof("Checkpoint/restore support disabled via configuration")
		}

		if c.SeccompProfile == "" {
			if err := c.seccompConfig.LoadDefaultProfile(); err != nil {
				return fmt.Errorf("unable to load default seccomp profile: %w", err)
			}
		} else if err := c.seccompConfig.LoadProfile(c.SeccompProfile); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("unable to load seccomp profile: %w", err)
			}

			// Fallback to the internal default in order not to break upgrade paths.
			logrus.Info("Seccomp profile does not exist on disk, fallback to internal default profile")

			if err := c.seccompConfig.LoadDefaultProfile(); err != nil {
				return fmt.Errorf("unable to load default seccomp profile: %w", err)
			}
		}

		if err := c.apparmorConfig.LoadProfile(c.ApparmorProfile); err != nil {
			return fmt.Errorf("unable to load AppArmor profile: %w", err)
		}

		if err := c.blockioConfig.Load(c.BlockIOConfigFile); err != nil {
			return fmt.Errorf("blockio configuration: %w", err)
		}

		c.blockioConfig.SetReload(c.BlockIOReload)

		if err := c.rdtConfig.Load(c.RdtConfigFile); err != nil {
			return fmt.Errorf("rdt configuration: %w", err)
		}
	}

	if err := c.TranslateMonitorFields(onExecution); err != nil {
		return fmt.Errorf("monitor fields translation: %w", err)
	}

	return nil
}

// ValidateDefaultRuntime ensures that the default runtime is set and valid.
func (c *RuntimeConfig) ValidateDefaultRuntime() error {
	// If the default runtime is defined in the runtime entry table, then it is valid
	if _, ok := c.Runtimes[c.DefaultRuntime]; ok {
		return nil
	}

	// If a non-empty runtime does not exist in the runtime entry table, this is an error.
	if c.DefaultRuntime != "" {
		return fmt.Errorf("default_runtime set to %q, but no runtime entry table [crio.runtime.runtimes.%s] was found", c.DefaultRuntime, c.DefaultRuntime)
	}

	// Set the default runtime to "crun" if default_runtime is not set
	logrus.Debugf("Defaulting to %q as the runtime since default_runtime is not set", DefaultRuntime)
	// The default config sets crun and its path in the runtimes map, so check for that
	// first. If it does not exist then we add runc + its path to the runtimes map.
	if _, ok := c.Runtimes[DefaultRuntime]; !ok {
		c.Runtimes[DefaultRuntime] = defaultRuntimeHandler(c.cgroupManager.IsSystemd())
	}
	// Set the DefaultRuntime to runc so we don't fail further along in the code
	c.DefaultRuntime = DefaultRuntime

	return nil
}

// getDefaultMonitorGroup checks which defaultmonitor group to use
// for cgroupfs it is empty.
func getDefaultMonitorGroup(isSystemd bool) string {
	monitorGroup := ""
	if isSystemd {
		monitorGroup = defaultMonitorCgroup
	}

	return monitorGroup
}

func defaultRuntimeHandler(isSystemd bool) *RuntimeHandler {
	return &RuntimeHandler{
		RuntimeType:            DefaultRuntimeType,
		RuntimeRoot:            DefaultRuntimeRoot,
		ContainerCreateTimeout: defaultContainerCreateTimeout,
		AllowedAnnotations: []string{
			annotations.OCISeccompBPFHookAnnotation,
			annotations.DevicesAnnotation,
		},
		MonitorEnv: []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		},
		ContainerMinMemory: units.BytesSize(defaultContainerMinMemoryCrun),
		MonitorCgroup:      getDefaultMonitorGroup(isSystemd),
		ExecCPUAffinity:    ExecCPUAffinityTypeDefault,
		SeccompProfile:     runtimeSeccompProfileDefault,
	}
}

// ValidateRuntimes checks every runtime if its members are valid.
func (c *RuntimeConfig) ValidateRuntimes() error {
	var failedValidation []string

	// Update the default runtime paths in all runtimes that are asking for inheritance
	for name := range c.Runtimes {
		if !c.Runtimes[name].InheritDefaultRuntime {
			continue
		}

		logrus.Infof("Inheriting runtime configuration %q from %q", name, c.DefaultRuntime)

		c.Runtimes[name].RuntimePath = c.Runtimes[c.DefaultRuntime].RuntimePath
		// An empty RuntimePath causes cri-o to look for a binary named `name`,
		// but we inherit from the default - look for binary called c.DefaultRuntime
		// The validator will check the binary is valid below.
		if c.Runtimes[name].RuntimePath == "" {
			executable, err := exec.LookPath(c.DefaultRuntime)
			if err == nil {
				c.Runtimes[name].RuntimePath = executable
			}
		}

		c.Runtimes[name].RuntimeType = c.Runtimes[c.DefaultRuntime].RuntimeType
		c.Runtimes[name].RuntimeConfigPath = c.Runtimes[c.DefaultRuntime].RuntimeConfigPath
		c.Runtimes[name].RuntimeRoot = c.Runtimes[c.DefaultRuntime].RuntimeRoot
	}

	// Validate if runtime_path does exist for each runtime
	for name, handler := range c.Runtimes {
		if err := handler.Validate(name); err != nil {
			if c.DefaultRuntime == name {
				return err
			}

			logrus.Warnf("Runtime handler %q is being ignored due to: %v", name, err)
			failedValidation = append(failedValidation, name)
		}
	}

	for _, invalidHandlerName := range failedValidation {
		delete(c.Runtimes, invalidHandlerName)
	}

	c.initializeRuntimeFeatures()

	return nil
}

func (c *RuntimeConfig) initializeRuntimeFeatures() {
	for name, handler := range c.Runtimes {
		versionOutput, err := cmdrunner.CombinedOutput(handler.RuntimePath, "--version")
		if err != nil {
			logrus.Errorf("Unable to determine version of runtime handler %q: %v", name, err)

			continue
		}

		versionString := strings.ReplaceAll(strings.TrimSpace(string(versionOutput)), "\n", ", ")
		logrus.Infof("Using runtime handler %s", versionString)

		// If this returns an error, we just ignore it and assume the features sub-command is
		// not supported by the runtime.
		output, err := cmdrunner.CombinedOutput(handler.RuntimePath, "features")
		if err != nil {
			logrus.Errorf("Getting %s OCI runtime features failed: %s: %v", handler.RuntimePath, output, err)

			continue
		}

		// Ignore error if we can't load runtime features.
		if err := handler.LoadRuntimeFeatures(output); err != nil {
			logrus.Errorf("Unable to load OCI features for runtime handler %q: %v", name, err)

			continue
		}

		if handler.RuntimeSupportsIDMap() {
			logrus.Debugf("Runtime handler %q supports User and Group ID-mappings", name)
		}

		// Recursive Read-only (RRO) mounts require runtime handler support,
		// such as runc v1.1 or crun v1.4. For Linux, the minimum kernel
		// version 5.12 or a kernel with the necessary changes backported
		// is required.
		rro := handler.RuntimeSupportsMountFlag("rro")
		if rro {
			logrus.Debugf("Runtime handler %q supports Recursive Read-only (RRO) mounts", name)

			// A given runtime might support Recursive Read-only (RRO) mounts,
			// but the current kernel might not.
			if err := checkKernelRROMountSupport(); err != nil {
				logrus.Warnf("Runtime handler %q supports Recursive Read-only (RRO) mounts, but kernel does not: %v", name, err)

				rro = false
			}
		}

		handler.features.RecursiveReadOnlyMounts = rro
	}
}

func (c *RuntimeConfig) TranslateMonitorFields(onExecution bool) error {
	for name, handler := range c.Runtimes {
		if handler.RuntimeType == DefaultRuntimeType || handler.RuntimeType == "" {
			if err := c.TranslateMonitorFieldsForHandler(handler, onExecution); err != nil {
				return fmt.Errorf("failed to translate monitor fields for runtime %s: %w", name, err)
			}
		}
	}

	return nil
}

// TranslateMonitorFields is a transitional function that takes the configuration fields
// previously held by the RuntimeConfig that are being moved inside of the runtime handler structure.
func (c *RuntimeConfig) TranslateMonitorFieldsForHandler(handler *RuntimeHandler, onExecution bool) error {
	if c.ConmonCgroup != "" {
		logrus.Debugf("Monitor cgroup %s is becoming %s", handler.MonitorCgroup, c.ConmonCgroup)
		handler.MonitorCgroup = c.ConmonCgroup
	}

	if c.Conmon != "" {
		logrus.Debugf("Monitor path %s is becoming %s", handler.MonitorPath, c.Conmon)
		handler.MonitorPath = c.Conmon
	}

	if len(c.ConmonEnv) != 0 {
		handler.MonitorEnv = c.ConmonEnv
	}

	// If systemd and empty, assume default
	if c.cgroupManager.IsSystemd() && handler.MonitorCgroup == "" {
		handler.MonitorCgroup = defaultMonitorCgroup
	}

	if onExecution {
		if err := c.ValidateConmonPath("conmon", handler); err != nil {
			return err
		}
		// if cgroupManager is cgroupfs
		if !c.cgroupManager.IsSystemd() {
			// handler.MonitorCgroup having value "" is valid
			// but the default value system.slice is not
			if handler.MonitorCgroup == defaultMonitorCgroup {
				handler.MonitorCgroup = ""
			}

			if handler.MonitorCgroup != utils.PodCgroupName && handler.MonitorCgroup != "" {
				return fmt.Errorf("cgroupfs manager conmon cgroup should be 'pod' or empty, but got: '%s'", handler.MonitorCgroup)
			}

			return nil
		}

		if handler.MonitorCgroup != utils.PodCgroupName && !strings.HasSuffix(handler.MonitorCgroup, ".slice") {
			return errors.New("conmon cgroup should be 'pod' or a systemd slice")
		}
	}

	return nil
}

// ValidateConmonPath checks if `Conmon` is set within the `RuntimeConfig`.
// If this is not the case, it tries to find it within the $PATH variable.
// In any other case, it simply checks if `Conmon` is a valid file.
func (c *RuntimeConfig) ValidateConmonPath(executable string, handler *RuntimeHandler) error {
	var err error

	handler.MonitorPath, err = validateExecutablePath(executable, handler.MonitorPath)
	if err != nil {
		return err
	}

	c.conmonManager, err = conmonmgr.New(handler.MonitorPath)

	return err
}

func (c *RuntimeConfig) ConmonSupportsSync() bool {
	return c.conmonManager.SupportsSync()
}

func (c *RuntimeConfig) ConmonSupportsLogGlobalSizeMax() bool {
	return c.conmonManager.SupportsLogGlobalSizeMax()
}

func validateCriuInPath() error {
	_, err := validateExecutablePath("criu", "")

	return err
}

// Seccomp returns the seccomp configuration.
func (c *RuntimeConfig) Seccomp() *seccomp.Config {
	return c.seccompConfig
}

// AppArmor returns the AppArmor configuration.
func (c *RuntimeConfig) AppArmor() *apparmor.Config {
	return c.apparmorConfig
}

// BlockIO returns the blockio configuration.
func (c *RuntimeConfig) BlockIO() *blockio.Config {
	return c.blockioConfig
}

// Rdt returns the RDT configuration.
func (c *RuntimeConfig) Rdt() *rdt.Config {
	return c.rdtConfig
}

// CgroupManager returns the CgroupManager configuration.
func (c *RuntimeConfig) CgroupManager() cgmgr.CgroupManager {
	return c.cgroupManager
}

// NamespaceManager returns the NamespaceManager configuration.
func (c *RuntimeConfig) NamespaceManager() *nsmgr.NamespaceManager {
	return c.namespaceManager
}

// Ulimits returns the Ulimits configuration.
func (c *RuntimeConfig) Ulimits() []ulimits.Ulimit {
	return c.ulimitsConfig.Ulimits()
}

func (c *RuntimeConfig) Devices() []device.Device {
	return c.deviceConfig.Devices()
}

func (c *RuntimeConfig) CheckpointRestore() bool {
	return c.EnableCriuSupport
}

func validateExecutablePath(executable, currentPath string) (string, error) {
	if currentPath == "" {
		path, err := exec.LookPath(executable)
		if err != nil {
			return "", err
		}

		logrus.Debugf("Using %s from $PATH: %s", executable, path)

		return path, nil
	}

	if _, err := os.Stat(currentPath); err != nil {
		return "", fmt.Errorf("invalid %s path: %w", executable, err)
	}

	logrus.Infof("Using %s executable: %s", executable, currentPath)

	return currentPath, nil
}

// Validate is the main entry point for image configuration validation.
// It returns an error on validation failure, otherwise nil.
func (c *ImageConfig) Validate(onExecution bool) error {
	for key, value := range map[string]string{
		"signature policy": c.SignaturePolicyDir,
		"namespaced auth":  c.NamespacedAuthDir,
	} {
		if !filepath.IsAbs(value) {
			return fmt.Errorf("%s dir %q is not absolute", key, value)
		}

		if onExecution {
			if err := os.MkdirAll(value, 0o755); err != nil {
				return fmt.Errorf("cannot create %s dir: %w", key, err)
			}
		}
	}

	if _, err := c.ParsePauseImage(); err != nil {
		return fmt.Errorf("invalid pause image %q: %w", c.PauseImage, err)
	}

	switch c.ShortNameMode {
	case "enforcing", "disabled", "":
	default:
		return fmt.Errorf("invalid short name mode %q", c.ShortNameMode)
	}

	return nil
}

// ParsePauseImage parses the .PauseImage value as into a validated, well-typed value.
func (c *ImageConfig) ParsePauseImage() (references.RegistryImageReference, error) {
	return references.ParseRegistryImageReferenceFromOutOfProcessData(c.PauseImage)
}

// Validate is the main entry point for network configuration validation.
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *NetworkConfig) Validate(onExecution bool) error {
	if onExecution {
		err := utils.IsDirectory(c.NetworkDir)
		if err != nil {
			if os.IsNotExist(err) {
				if err = os.MkdirAll(c.NetworkDir, 0o755); err != nil {
					return fmt.Errorf("cannot create network_dir: %s: %w", c.NetworkDir, err)
				}
			} else {
				return fmt.Errorf("invalid network_dir: %s: %w", c.NetworkDir, err)
			}
		}

		for _, pluginDir := range c.PluginDirs {
			if err := os.MkdirAll(pluginDir, 0o755); err != nil {
				return fmt.Errorf("invalid plugin_dirs entry: %w", err)
			}
		}
		// While the plugin_dir option is being deprecated, we need this check
		if c.PluginDir != "" {
			logrus.Warnf("The config field plugin_dir is being deprecated. Please use plugin_dirs instead")

			if err := os.MkdirAll(c.PluginDir, 0o755); err != nil {
				return fmt.Errorf("invalid plugin_dir entry: %w", err)
			}
			// Append PluginDir to PluginDirs, so from now on we can operate in terms of PluginDirs and not worry
			// about missing cases.
			c.PluginDirs = append(c.PluginDirs, c.PluginDir)

			// Empty the pluginDir so on future config calls we don't print it out
			// thus seamlessly transitioning and depreciating the option
			c.PluginDir = ""
		}

		// Init CNI plugin
		cniManager, err := cnimgr.New(
			c.CNIDefaultNetwork, c.NetworkDir, c.PluginDirs...,
		)
		if err != nil {
			return fmt.Errorf("initialize CNI plugin: %w", err)
		}

		c.cniManager = cniManager
	}

	return nil
}

// Validate checks if the whole runtime is valid.
func (r *RuntimeHandler) Validate(name string) error {
	if err := r.ValidateRuntimeType(name); err != nil {
		return err
	}

	if err := r.ValidateRuntimePath(name); err != nil {
		return err
	}

	if err := r.ValidateRuntimeConfigPath(name); err != nil {
		return err
	}

	if err := r.ValidateRuntimeAllowedAnnotations(); err != nil {
		return err
	}

	if err := r.ValidateContainerMinMemory(name); err != nil {
		logrus.Errorf("Unable to set minimum container memory for runtime handler %q: %v", name, err)
	}

	r.ValidateContainerCreateTimeout(name)

	if err := r.ValidateNoSyncLog(); err != nil {
		return fmt.Errorf("no sync log: %w", err)
	}

	if err := r.ValidateWebsocketStreaming(name); err != nil {
		return fmt.Errorf("websocket streaming: %w", err)
	}

	if err := r.validateRuntimeExecCPUAffinity(); err != nil {
		return err
	}

	if err := r.validateRuntimeSeccompProfile(); err != nil {
		return err
	}

	return nil
}

func (r *RuntimeHandler) ValidateRuntimeVMBinaryPattern() bool {
	if r.RuntimeType != RuntimeTypeVM {
		return true
	}

	binaryName := filepath.Base(r.RuntimePath)

	matched, err := regexp.MatchString(RuntimeTypeVMBinaryPattern, binaryName)
	if err != nil {
		return false
	}

	return matched
}

// ValidateRuntimePath checks if the `RuntimePath` is either set or available
// within the $PATH environment. The method fails on any `RuntimePath` lookup
// error.
func (r *RuntimeHandler) ValidateRuntimePath(name string) error {
	if r.RuntimePath == "" {
		executable, err := exec.LookPath(name)
		if err != nil {
			return fmt.Errorf("%q not found in $PATH: %w", name, err)
		}

		r.RuntimePath = executable
		logrus.Debugf("Using runtime executable from $PATH %q", executable)
	} else if _, err := os.Stat(r.RuntimePath); err != nil && os.IsNotExist(err) {
		return fmt.Errorf("invalid runtime_path for runtime '%s': %w", name, err)
	}

	ok := r.ValidateRuntimeVMBinaryPattern()
	if !ok {
		return fmt.Errorf("invalid runtime_path for runtime '%s': containerd binary naming pattern is not followed",
			name)
	}

	logrus.Debugf(
		"Found valid runtime %q for runtime_path %q", name, r.RuntimePath,
	)

	return nil
}

// ValidateRuntimeType checks if the `RuntimeType` is valid.
func (r *RuntimeHandler) ValidateRuntimeType(name string) error {
	if r.RuntimeType != "" && r.RuntimeType != DefaultRuntimeType && r.RuntimeType != RuntimeTypeVM && r.RuntimeType != RuntimeTypePod {
		return fmt.Errorf("invalid `runtime_type` %q for runtime %q",
			r.RuntimeType, name)
	}

	return nil
}

// ValidateRuntimeConfigPath checks if the `RuntimeConfigPath` exists.
func (r *RuntimeHandler) ValidateRuntimeConfigPath(name string) error {
	if r.RuntimeConfigPath == "" {
		return nil
	}

	if r.RuntimeType != RuntimeTypeVM {
		return errors.New("runtime_config_path can only be used with the 'vm' runtime type")
	}

	if _, err := os.Stat(r.RuntimeConfigPath); err != nil && os.IsNotExist(err) {
		return fmt.Errorf("invalid runtime_config_path for runtime '%s': %w", name, err)
	}

	return nil
}

func (r *RuntimeHandler) ValidateRuntimeAllowedAnnotations() error {
	disallowed, err := validateAllowedAndGenerateDisallowedAnnotations(r.AllowedAnnotations)
	if err != nil {
		return err
	}

	logrus.Debugf(
		"Allowed annotations for runtime: %v", r.AllowedAnnotations,
	)

	r.DisallowedAnnotations = disallowed

	return nil
}

// ValidateNoSyncLog checks if the `NoSyncLog` is used with the correct `RuntimeType` ('oci').
func (r *RuntimeHandler) ValidateNoSyncLog() error {
	if !r.NoSyncLog {
		return nil
	}
	// no_sync_log can only be used with the 'oci' runtime type.
	// This means that the runtime type must be set to 'oci' or left empty
	if r.RuntimeType == DefaultRuntimeType || r.RuntimeType == "" {
		logrus.Warn("NoSyncLog is enabled. This can lead to lost log data")

		return nil
	}

	return fmt.Errorf("no_sync_log is only allowed with runtime type 'oci', runtime type is '%s'", r.RuntimeType)
}

// ValidateContainerMinMemory sets the minimum container memory for a given runtime.
// assigns defaultContainerMinMemory if no container_min_memory provided.
func (r *RuntimeHandler) ValidateContainerMinMemory(name string) error {
	if r.ContainerMinMemory == "" {
		r.ContainerMinMemory = units.BytesSize(defaultContainerMinMemory)
	}

	memorySize, err := units.RAMInBytes(r.ContainerMinMemory)
	if err != nil {
		err = fmt.Errorf("unable to set runtime memory to %q: %w. Setting to %q instead", r.ContainerMinMemory, err, defaultContainerMinMemory)
		// Fallback to default value if something is wrong with the configured value.
		r.ContainerMinMemory = units.BytesSize(defaultContainerMinMemory)

		return err
	}

	logrus.Debugf("Runtime handler %q container minimum memory set to %d bytes", name, memorySize)

	return nil
}

// ValidateContainerCreateTimeout sets the default container create timeout if not configured.
func (r *RuntimeHandler) ValidateContainerCreateTimeout(name string) {
	switch {
	case r.ContainerCreateTimeout == 0:
		r.ContainerCreateTimeout = defaultContainerCreateTimeout
		logrus.Infof("Runtime handler %q container create timeout not set, using default: %d seconds", name, r.ContainerCreateTimeout)
	case r.ContainerCreateTimeout < minimumContainerCreateTimeout:
		logrus.Warnf("Runtime handler %q container create timeout (%d seconds) is less than minimum (%d seconds), setting to minimum: %d seconds", name, r.ContainerCreateTimeout, minimumContainerCreateTimeout, minimumContainerCreateTimeout)
		r.ContainerCreateTimeout = minimumContainerCreateTimeout
	default:
		logrus.Infof("Runtime handler %q container create timeout set to: %d seconds", name, r.ContainerCreateTimeout)
	}
}

// ValidateWebsocketStreaming can be used to verify if the runtime supports WebSocket streaming.
func (r *RuntimeHandler) ValidateWebsocketStreaming(name string) error {
	if r.RuntimeType != RuntimeTypePod {
		if r.StreamWebsockets {
			return fmt.Errorf(`only the 'runtime_type = "pod"' supports websocket streaming, not %q (runtime %q)`, r.RuntimeType, name)
		}

		return nil
	}

	// Requires at least conmon-rs v0.7.0
	v, err := conmonrsClient.Version(r.MonitorPath)
	if err != nil {
		if errors.Is(err, conmonrsClient.ErrUnsupported) {
			logrus.Debugf("Unable to verify pod runtime version: %v", err)

			// Streaming server support got introduced in v0.7.0
			if r.StreamWebsockets {
				logrus.Warnf("Disabling streaming over websockets, it requires conmon-rs >= v0.7.0")

				r.StreamWebsockets = false
			}

			return nil
		}

		return fmt.Errorf("get conmon-rs version: %w", err)
	}

	if v.Tag == "" {
		v.Tag = "none"
	}

	logrus.Infof(
		"Runtime handler %q is using conmon-rs version: %s, tag: %s, commit: %s, build: %s, target: %s, %s, %s",
		name, v.Version, v.Tag, v.Commit, v.BuildDate, v.Target, v.RustVersion, v.CargoVersion,
	)

	return nil
}

// LoadRuntimeFeatures loads features for a given runtime handler using the "features"
// sub-command output, where said output contains a JSON document called "Features
// Structure" that describes the runtime handler's supported features.
func (r *RuntimeHandler) LoadRuntimeFeatures(input []byte) error {
	if err := json.Unmarshal(input, &r.features); err != nil {
		return fmt.Errorf("unable to unmarshal features structure: %w", err)
	}

	// All other properties of the Features Structure are optional and might be
	// either absent, empty, or set to the null value, with the exception of
	// OCIVersionMin and OCIVersionMax, which are required. Thus, the lack of
	// them should indicate that the Features Structure document is potentially
	// not valid.
	//
	// See the following for more details about the Features Structure:
	//   https://github.com/opencontainers/runtime-spec/blob/main/features.md
	if r.features.OCIVersionMin == "" || r.features.OCIVersionMax == "" {
		return errors.New("runtime features structure is not valid")
	}

	return nil
}

// RuntimeSupportsIDMap returns whether this runtime supports the "runtime features"
// command, and that the output of that command advertises IDMap mounts as an option.
func (r *RuntimeHandler) RuntimeSupportsIDMap() bool {
	if r.features.Linux == nil || r.features.Linux.MountExtensions == nil || r.features.Linux.MountExtensions.IDMap == nil {
		return false
	}

	if enabled := r.features.Linux.MountExtensions.IDMap.Enabled; enabled == nil || !*enabled {
		return false
	}

	return true
}

// RuntimeSupportsRROMounts returns whether this runtime supports the Recursive Read-only mount as an option.
func (r *RuntimeHandler) RuntimeSupportsRROMounts() bool {
	return r.features.RecursiveReadOnlyMounts
}

// RuntimeSupportsMountFlag returns whether this runtime supports the specified mount option.
func (r *RuntimeHandler) RuntimeSupportsMountFlag(flag string) bool {
	return slices.Contains(r.features.MountOptions, flag)
}

// RuntimeDefaultAnnotations returns the default annotations for this handler.
func (r *RuntimeHandler) RuntimeDefaultAnnotations() map[string]string {
	return r.DefaultAnnotations
}

// RuntimeStreamWebsockets returns the configured websocket streaming option for this handler.
func (r *RuntimeHandler) RuntimeStreamWebsockets() bool {
	return r.StreamWebsockets
}

// RuntimeSeccomp returns the configuration of the loaded seccomp profile for this handler.
func (r *RuntimeHandler) RuntimeSeccomp() *seccomp.Config {
	return r.seccompConfig
}

// validateRuntimeExecCPUAffinity checks if the RuntimeHandler enforces proper CPU affinity settings.
func (r *RuntimeHandler) validateRuntimeExecCPUAffinity() error {
	switch r.ExecCPUAffinity {
	case ExecCPUAffinityTypeDefault, ExecCPUAffinityTypeFirst:
		return nil
	}

	return fmt.Errorf("invalid exec_cpu_affinity %q", r.ExecCPUAffinity)
}

// validateRuntimeSeccompProfile tries to load the RuntimeHandler seccomp profile.
func (r *RuntimeHandler) validateRuntimeSeccompProfile() error {
	if r.SeccompProfile == "" {
		r.seccompConfig = nil

		return nil
	}

	r.seccompConfig = seccomp.New()
	if err := r.seccompConfig.LoadProfile(r.SeccompProfile); err != nil {
		return fmt.Errorf("unable to load runtime handler seccomp profile: %w", err)
	}

	return nil
}

func validateAllowedAndGenerateDisallowedAnnotations(allowed []string) (disallowed []string, _ error) {
	disallowedMap := make(map[string]bool)
	for _, ann := range annotations.AllAllowedAnnotations {
		disallowedMap[ann] = false
	}

	for _, ann := range allowed {
		if _, ok := disallowedMap[ann]; !ok {
			return nil, fmt.Errorf("invalid allowed_annotation: %s", ann)
		}

		disallowedMap[ann] = true
	}

	disallowed = make([]string, 0, len(disallowedMap))

	for ann, allowed := range disallowedMap {
		if !allowed {
			disallowed = append(disallowed, ann)
		}
	}

	return disallowed, nil
}

// CNIPlugin returns the network configuration CNI plugin.
func (c *NetworkConfig) CNIPlugin() ocicni.CNIPlugin {
	return c.cniManager.Plugin()
}

// CNIPluginReadyOrError returns whether the cni plugin is ready.
func (c *NetworkConfig) CNIPluginReadyOrError() error {
	return c.cniManager.ReadyOrError()
}

// CNIPluginAddWatcher returns the network configuration CNI plugin.
func (c *NetworkConfig) CNIPluginAddWatcher() chan bool {
	return c.cniManager.AddWatcher()
}

// CNIPluginGC calls the plugin's GC to clean up any resources concerned with
// stale pods (pod other than the ones provided by validPodList). The call to
// the plugin will be deferred until it is ready logging any errors then and
// returning nil error here.
func (c *Config) CNIPluginGC(ctx context.Context, validPodList cnimgr.PodNetworkLister) error {
	return c.cniManager.GC(ctx, validPodList)
}

// CNIManagerShutdown shuts down the CNI Manager.
func (c *NetworkConfig) CNIManagerShutdown() {
	c.cniManager.Shutdown()
}

// SetSingleConfigPath set single config path for config.
func (c *Config) SetSingleConfigPath(singleConfigPath string) {
	c.singleConfigPath = singleConfigPath
}
