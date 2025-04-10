package criocli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v2"

	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/server/metrics/collectors"
)

// DefaultCommands are the flags commands can be added to every binary.
var DefaultCommands = []*cli.Command{
	completion(),
	man(),
	markdown(),
}

func GetConfigFromContext(c *cli.Context) (*libconfig.Config, error) {
	config, ok := c.App.Metadata["config"].(*libconfig.Config)
	if !ok {
		return nil, errors.New("type assertion error when accessing server config")
	}

	return config, nil
}

func GetAndMergeConfigFromContext(c *cli.Context) (*libconfig.Config, error) {
	config, err := GetConfigFromContext(c)
	if err != nil {
		return nil, err
	}

	if err := libconfig.MergeConfig(config, c); err != nil {
		return nil, err
	}

	return config, nil
}

func GetFlagsAndMetadata() ([]cli.Flag, map[string]interface{}, error) {
	config, err := libconfig.DefaultConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("error loading server config: %w", err)
	}

	// TODO FIXME should be crio wipe flags
	flags := getCrioFlags(config)

	metadata := map[string]any{
		"config": config,
	}

	return flags, metadata, nil
}

func getCrioFlags(defConf *libconfig.Config) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:      "config",
			Aliases:   []string{"c"},
			Value:     libconfig.CrioConfigPath,
			Usage:     "Path to configuration file",
			EnvVars:   []string{"CONTAINER_CONFIG"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "config-dir",
			Aliases: []string{"d"},
			Value:   libconfig.CrioConfigDropInPath,
			Usage: fmt.Sprintf("Path to the configuration drop-in directory."+`
    This directory will be recursively iterated and each file gets applied
    to the configuration in their processing order. This means that a
    configuration file named '00-default' has a lower priority than a file
    named '01-my-overwrite'.
    The global config file, provided via '--config,-c' or per default in
    %s, always has a lower priority than the files in the directory specified
    by '--config-dir,-d'.
    Besides that, provided command line parameters have a higher priority
    than any configuration file.`, libconfig.CrioConfigPath),
			EnvVars:   []string{"CONTAINER_CONFIG_DIR"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "conmon",
			Usage:     "Path to the conmon binary, used for monitoring the OCI runtime. Will be searched for using $PATH if empty. This option is deprecated, and will be removed in the future.",
			EnvVars:   []string{"CONTAINER_CONMON"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "conmon-cgroup",
			Usage:   "cgroup to be used for conmon process. This option is deprecated and will be removed in the future.",
			Value:   defConf.ConmonCgroup,
			EnvVars: []string{"CONTAINER_CONMON_CGROUP"},
		},
		&cli.StringFlag{
			Name:      "listen",
			Usage:     "Path to the CRI-O socket.",
			Value:     defConf.Listen,
			EnvVars:   []string{"CONTAINER_LISTEN"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "stream-address",
			Usage:   "Bind address for streaming socket.",
			Value:   defConf.StreamAddress,
			EnvVars: []string{"CONTAINER_STREAM_ADDRESS"},
		},
		&cli.StringFlag{
			Name:    "stream-port",
			Usage:   "Bind port for streaming socket. If the port is set to '0', then CRI-O will allocate a random free port number.",
			Value:   defConf.StreamPort,
			EnvVars: []string{"CONTAINER_STREAM_PORT"},
		},
		&cli.StringFlag{
			Name:      "log",
			Usage:     "Set the log file path where internal debug information is written.",
			EnvVars:   []string{"CONTAINER_LOG"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "log-format",
			Value:   "text",
			Usage:   "Set the format used by logs: 'text' or 'json'.",
			EnvVars: []string{"CONTAINER_LOG_FORMAT"},
		},
		&cli.StringFlag{
			Name:    "log-level",
			Aliases: []string{"l"},
			Value:   "info",
			Usage:   "Log messages above specified level: trace, debug, info, warn, error, fatal or panic.",
			EnvVars: []string{"CONTAINER_LOG_LEVEL"},
		},
		&cli.StringFlag{
			Name:    "log-filter",
			Usage:   `Filter the log messages by the provided regular expression. For example 'request.\*' filters all gRPC requests.`,
			EnvVars: []string{"CONTAINER_LOG_FILTER"},
		},
		&cli.StringFlag{
			Name:      "log-dir",
			Usage:     "Default log directory where all logs will go unless directly specified by the kubelet.",
			Value:     defConf.LogDir,
			EnvVars:   []string{"CONTAINER_LOG_DIR"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "pause-command",
			Usage:   "Path to the pause executable in the pause image.",
			Value:   defConf.PauseCommand,
			EnvVars: []string{"CONTAINER_PAUSE_COMMAND"},
		},
		&cli.StringFlag{
			Name:    "pause-image",
			Usage:   "Image which contains the pause executable.",
			Value:   defConf.PauseImage,
			EnvVars: []string{"CONTAINER_PAUSE_IMAGE"},
		},
		&cli.StringFlag{
			Name:      "pause-image-auth-file",
			Usage:     "Path to a config file containing credentials for --pause-image.",
			EnvVars:   []string{"CONTAINER_PAUSE_IMAGE_AUTH_FILE"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "separate-pull-cgroup",
			Usage:   "[EXPERIMENTAL] Pull in new cgroup.",
			EnvVars: []string{"PULL_IN_A_CGROUP"},
		},
		&cli.StringFlag{
			Name:      "global-auth-file",
			Usage:     "Path to a file like /var/lib/kubelet/config.json holding credentials necessary for pulling images from secure registries.",
			EnvVars:   []string{"CONTAINER_GLOBAL_AUTH_FILE"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "signature-policy",
			Usage:     "Path to signature policy JSON file.",
			EnvVars:   []string{"CONTAINER_SIGNATURE_POLICY"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "signature-policy-dir",
			Usage:     "Path to the root directory for namespaced signature policies. Must be an absolute path.",
			Value:     defConf.SignaturePolicyDir,
			EnvVars:   []string{"CONTAINER_SIGNATURE_POLICY_DIR"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "root",
			Aliases:   []string{"r"},
			Usage:     "The CRI-O root directory.",
			Value:     defConf.Root,
			EnvVars:   []string{"CONTAINER_ROOT"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "runroot",
			Usage:     "The CRI-O state directory.",
			Value:     defConf.RunRoot,
			EnvVars:   []string{"CONTAINER_RUNROOT"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "imagestore",
			Usage:     "Store newly pulled images in the specified path, rather than the path provided by --root.",
			Value:     defConf.ImageStore,
			EnvVars:   []string{"CONTAINER_IMAGESTORE"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "storage-driver",
			Aliases: []string{"s"},
			Usage:   "OCI storage driver.",
			EnvVars: []string{"CONTAINER_STORAGE_DRIVER"},
		},
		&cli.StringSliceFlag{
			Name:    "storage-opt",
			Value:   cli.NewStringSlice(defConf.StorageOptions...),
			Usage:   "OCI storage driver option.",
			EnvVars: []string{"CONTAINER_STORAGE_OPT"},
		},
		&cli.StringSliceFlag{
			Name:  "insecure-registry",
			Value: cli.NewStringSlice(defConf.InsecureRegistries...),
			Usage: "Enable insecure registry communication, i.e., enable un-encrypted and/or untrusted communication." + `
    1. List of insecure registries can contain an element with CIDR notation to
       specify a whole subnet.
    2. Insecure registries accept HTTP or accept HTTPS with certificates from
       unknown CAs.
    3. Enabling '--insecure-registry' is useful when running a local registry.
       However, because its use creates security vulnerabilities, **it should ONLY
       be enabled for testing purposes**. For increased security, users should add
       their CA to their system's list of trusted CAs instead of using
       '--insecure-registry'.`,
			EnvVars: []string{"CONTAINER_INSECURE_REGISTRY"},
		},
		&cli.StringFlag{
			Name:    "default-transport",
			Usage:   "A prefix to prepend to image names that cannot be pulled as-is.",
			Value:   defConf.DefaultTransport,
			EnvVars: []string{"CONTAINER_DEFAULT_TRANSPORT"},
		},
		&cli.StringFlag{
			Name:  "decryption-keys-path",
			Usage: "Path to load keys for image decryption.",
			Value: defConf.DecryptionKeysPath,
		},
		&cli.StringFlag{
			Name:    "default-runtime",
			Usage:   "Default OCI runtime from the runtimes config.",
			Value:   defConf.DefaultRuntime,
			EnvVars: []string{"CONTAINER_DEFAULT_RUNTIME"},
		},
		&cli.StringSliceFlag{
			Name:    "runtimes",
			Usage:   "OCI runtimes, format is 'runtime_name:runtime_path:runtime_root:runtime_type:privileged_without_host_devices:runtime_config_path:container_min_memory'.",
			EnvVars: []string{"CONTAINER_RUNTIMES"},
		},
		&cli.StringFlag{
			Name:      "seccomp-profile",
			Usage:     "Path to the seccomp.json profile to be used as the runtime's default. If not specified, then the internal default seccomp profile will be used.",
			EnvVars:   []string{"CONTAINER_SECCOMP_PROFILE"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "apparmor-profile",
			Usage:   "Name of the apparmor profile to be used as the runtime's default. This only takes effect if the user does not specify a profile via the Kubernetes Pod's metadata annotation.",
			Value:   defConf.ApparmorProfile,
			EnvVars: []string{"CONTAINER_APPARMOR_PROFILE"},
		},
		&cli.StringFlag{
			Name:  "blockio-config-file",
			Usage: "Path to the blockio class configuration file for configuring the cgroup blockio controller.",
			Value: defConf.BlockIOConfigFile,
		},
		&cli.BoolFlag{
			Name:  "blockio-reload",
			Usage: "Reload blockio-config-file and rescan blockio devices in the system before applying blockio parameters.",
			Value: defConf.BlockIOReload,
		},
		&cli.StringFlag{
			Name:  "irqbalance-config-file",
			Usage: "The irqbalance service config file which is used by CRI-O.",
			Value: defConf.IrqBalanceConfigFile,
		},
		&cli.StringFlag{
			Name:  "rdt-config-file",
			Usage: "Path to the RDT configuration file for configuring the resctrl pseudo-filesystem.",
			Value: defConf.RdtConfigFile,
		},
		&cli.BoolFlag{
			Name:    "selinux",
			Usage:   "Enable selinux support. This option is deprecated, and be interpreted from whether SELinux is enabled on the host in the future.",
			EnvVars: []string{"CONTAINER_SELINUX"},
			Value:   defConf.SELinux,
		},
		&cli.StringFlag{
			Name:    "cgroup-manager",
			Usage:   "cgroup manager (cgroupfs or systemd).",
			Value:   defConf.CgroupManagerName,
			EnvVars: []string{"CONTAINER_CGROUP_MANAGER"},
		},
		&cli.Int64Flag{
			Name:    "pids-limit",
			Value:   libconfig.DefaultPidsLimit,
			Usage:   "Maximum number of processes allowed in a container. This option is deprecated. The Kubelet flag '--pod-pids-limit' should be used instead.",
			EnvVars: []string{"CONTAINER_PIDS_LIMIT"},
		},
		&cli.Int64Flag{
			Name:    "log-size-max",
			Value:   libconfig.DefaultLogSizeMax,
			Usage:   "Maximum log size in bytes for a container. If it is positive, it must be >= 8192 to match/exceed conmon read buffer. This option is deprecated. The Kubelet flag '--container-log-max-size' should be used instead.",
			EnvVars: []string{"CONTAINER_LOG_SIZE_MAX"},
		},
		&cli.BoolFlag{
			Name:    "log-journald",
			Usage:   "Log to systemd journal (journald) in addition to kubernetes log file.",
			EnvVars: []string{"CONTAINER_LOG_JOURNALD"},
			Value:   defConf.LogToJournald,
		},
		&cli.StringFlag{
			Name:    "cni-default-network",
			Usage:   `Name of the default CNI network to select. If not set or "", then CRI-O will pick-up the first one found in --cni-config-dir.`,
			Value:   defConf.CNIDefaultNetwork,
			EnvVars: []string{"CONTAINER_CNI_DEFAULT_NETWORK"},
		},
		&cli.StringFlag{
			Name:      "cni-config-dir",
			Usage:     "CNI configuration files directory.",
			Value:     defConf.NetworkDir,
			EnvVars:   []string{"CONTAINER_CNI_CONFIG_DIR"},
			TakesFile: true,
		},
		&cli.StringSliceFlag{
			Name:    "cni-plugin-dir",
			Value:   cli.NewStringSlice(defConf.PluginDir),
			Usage:   "CNI plugin binaries directory.",
			EnvVars: []string{"CONTAINER_CNI_PLUGIN_DIR"},
		},
		&cli.StringFlag{
			Name:  "image-volumes",
			Value: string(libconfig.ImageVolumesMkdir),
			Usage: "Image volume handling ('mkdir', 'bind', or 'ignore')" + `
    1. mkdir: A directory is created inside the container root filesystem for
       the volumes.
    2. bind: A directory is created inside container state directory and bind
       mounted into the container for the volumes.
	3. ignore: All volumes are just ignored and no action is taken.`,
			EnvVars: []string{"CONTAINER_IMAGE_VOLUMES"},
		},
		&cli.StringSliceFlag{
			Name: "hooks-dir",
			Usage: `Set the OCI hooks directory path (may be set multiple times)
    If one of the directories does not exist, then CRI-O will automatically
    skip them.
    Each '\*.json' file in the path configures a hook for CRI-O
    containers. For more details on the syntax of the JSON files and
    the semantics of hook injection, see 'oci-hooks(5)'. CRI-O
    currently support both the 1.0.0 and 0.1.0 hook schemas, although
    the 0.1.0 schema is deprecated.
    This option may be set multiple times; paths from later options
    have higher precedence ('oci-hooks(5)' discusses directory
    precedence).
    For the annotation conditions, CRI-O uses the Kubernetes
    annotations, which are a subset of the annotations passed to the
    OCI runtime. For example, 'io.kubernetes.cri-o.Volumes' is part of
    the OCI runtime configuration annotations, but it is not part of
    the Kubernetes annotations being matched for hooks.
    For the bind-mount conditions, only mounts explicitly requested by
    Kubernetes configuration are considered. Bind mounts that CRI-O
    inserts by default (e.g. '/dev/shm') are not considered.`,
			Value:   cli.NewStringSlice(defConf.HooksDir...),
			EnvVars: []string{"CONTAINER_HOOKS_DIR"},
		},
		&cli.StringFlag{
			Name:      "default-mounts-file",
			Usage:     "Path to default mounts file.",
			EnvVars:   []string{"CONTAINER_DEFAULT_MOUNTS_FILE"},
			TakesFile: true,
		},
		&cli.StringSliceFlag{
			Name:    "default-capabilities",
			Usage:   "Capabilities to add to the containers.",
			EnvVars: []string{"CONTAINER_DEFAULT_CAPABILITIES"},
			Value:   cli.NewStringSlice(defConf.DefaultCapabilities...),
		},
		&cli.BoolFlag{
			Name:    "add-inheritable-capabilities",
			Usage:   "Add capabilities to the inheritable set, as well as the default group of permitted, bounding and effective.",
			EnvVars: []string{"CONTAINER_ADD_INHERITABLE_CAPABILITIES"},
			Value:   defConf.AddInheritableCapabilities,
		},
		&cli.StringSliceFlag{
			Name:    "default-sysctls",
			Usage:   "Sysctls to add to the containers.",
			EnvVars: []string{"CONTAINER_DEFAULT_SYSCTLS"},
			Value:   cli.NewStringSlice(defConf.DefaultSysctls...),
		},
		&cli.StringSliceFlag{
			Name:    "default-ulimits",
			Usage:   "Ulimits to apply to containers by default (name=soft:hard).",
			EnvVars: []string{"CONTAINER_DEFAULT_ULIMITS"},
			Value:   cli.NewStringSlice(defConf.DefaultUlimits...),
		},
		&cli.BoolFlag{
			Name:    "profile",
			Usage:   "Enable pprof remote profiler on 127.0.0.1:6060.",
			EnvVars: []string{"CONTAINER_PROFILE"},
		},
		&cli.StringFlag{
			Name:    "profile-cpu",
			Usage:   "Write a pprof CPU profile to the provided path.",
			EnvVars: []string{"CONTAINER_PROFILE_CPU"},
		},
		&cli.StringFlag{
			Name:    "profile-mem",
			Usage:   "Write a pprof memory profile to the provided path.",
			EnvVars: []string{"CONTAINER_PROFILE_MEM"},
		},
		&cli.IntFlag{
			Name:    "profile-port",
			Value:   6060,
			Usage:   "Port for the pprof profiler.",
			EnvVars: []string{"CONTAINER_PROFILE_PORT"},
		},
		&cli.BoolFlag{
			Name:    "enable-profile-unix-socket",
			Usage:   "Enable pprof profiler on crio unix domain socket.",
			EnvVars: []string{"ENABLE_PROFILE_UNIX_SOCKET"},
		},
		&cli.BoolFlag{
			Name:    "enable-metrics",
			Usage:   "Enable metrics endpoint for the server.",
			EnvVars: []string{"CONTAINER_ENABLE_METRICS"},
			Value:   defConf.EnableMetrics,
		},
		&cli.StringFlag{
			Name:    "metrics-host",
			Usage:   "Host for the metrics endpoint.",
			EnvVars: []string{"CONTAINER_METRICS_HOST"},
			Value:   defConf.MetricsHost,
		},
		&cli.IntFlag{
			Name:    "metrics-port",
			Usage:   "Port for the metrics endpoint.",
			EnvVars: []string{"CONTAINER_METRICS_PORT"},
			Value:   defConf.MetricsPort,
		},
		&cli.StringFlag{
			Name:    "metrics-socket",
			Usage:   "Socket for the metrics endpoint.",
			EnvVars: []string{"CONTAINER_METRICS_SOCKET"},
			Value:   defConf.MetricsSocket,
		},
		&cli.StringFlag{
			Name:    "metrics-cert",
			Usage:   "Certificate for the secure metrics endpoint.",
			EnvVars: []string{"CONTAINER_METRICS_CERT"},
			Value:   defConf.MetricsCert,
		},
		&cli.StringFlag{
			Name:    "metrics-key",
			Usage:   "Certificate key for the secure metrics endpoint.",
			EnvVars: []string{"CONTAINER_METRICS_KEY"},
			Value:   defConf.MetricsKey,
		},
		&cli.StringSliceFlag{
			Name:    "metrics-collectors",
			Usage:   "Enabled metrics collectors.",
			Value:   cli.NewStringSlice(collectors.All().ToSlice()...),
			EnvVars: []string{"CONTAINER_METRICS_COLLECTORS"},
		},
		&cli.BoolFlag{
			Name:    "enable-tracing",
			Usage:   "Enable OpenTelemetry trace data exporting.",
			EnvVars: []string{"CONTAINER_ENABLE_TRACING"},
			Value:   defConf.EnableTracing,
		},
		&cli.IntFlag{
			Name:    "tracing-sampling-rate-per-million",
			Value:   defConf.TracingSamplingRatePerMillion,
			Usage:   "Number of samples to collect per million OpenTelemetry spans. Set to 1000000 to always sample.",
			EnvVars: []string{"CONTAINER_TRACING_SAMPLING_RATE_PER_MILLION"},
		},
		&cli.StringFlag{
			Name:    "tracing-endpoint",
			Value:   defConf.TracingEndpoint,
			Usage:   "Address on which the gRPC tracing collector will listen.",
			EnvVars: []string{"CONTAINER_TRACING_ENDPOINT"},
		},
		&cli.BoolFlag{
			Name:  "enable-nri",
			Usage: "Enable NRI (Node Resource Interface) support.",
			Value: defConf.NRI.Enabled,
		},
		&cli.StringFlag{
			Name:  "nri-listen",
			Usage: "Socket to listen on for externally started NRI plugins to connect to.",
			Value: defConf.NRI.SocketPath,
		},
		&cli.StringFlag{
			Name:  "nri-plugin-dir",
			Usage: "Directory to scan for pre-installed NRI plugins to start automatically.",
			Value: defConf.NRI.PluginPath,
		},
		&cli.StringFlag{
			Name:  "nri-plugin-config-dir",
			Usage: "Directory to scan for configuration of pre-installed NRI plugins.",
			Value: defConf.NRI.PluginConfigPath,
		},
		&cli.BoolFlag{
			Name:  "nri-disable-connections",
			Usage: "Disable connections from externally started NRI plugins.",
			Value: defConf.NRI.DisableConnections,
		},
		&cli.DurationFlag{
			Name:  "nri-plugin-registration-timeout",
			Usage: `Timeout for a plugin to register itself with NRI.`,
			Value: defConf.NRI.PluginRegistrationTimeout,
		},
		&cli.DurationFlag{
			Name:  "nri-plugin-request-timeout",
			Usage: `Timeout for a plugin to handle an NRI request.`,
			Value: defConf.NRI.PluginRequestTimeout,
		},
		&cli.StringFlag{
			Name:    "big-files-temporary-dir",
			Usage:   `Path to the temporary directory to use for storing big files, used to store image blobs and data streams related to containers image management.`,
			EnvVars: []string{"CONTAINER_BIG_FILES_TEMPORARY_DIR"},
			Value:   defConf.BigFilesTemporaryDir,
		},
		&cli.BoolFlag{
			Name:    "auto-reload-registries",
			Usage:   "If true, CRI-O will automatically reload the mirror registry when there is an update to the 'registries.conf.d' directory. Default value is set to 'false'.",
			EnvVars: []string{"AUTO_RELOAD_REGISTRIES"},
			Value:   defConf.AutoReloadRegistries,
		},
		&cli.DurationFlag{
			Name:    "pull-progress-timeout",
			Usage:   "The timeout for an image pull to make progress until the pull operation gets canceled. This value will be also used for calculating the pull progress interval to --pull-progress-timeout / 10. Can be set to 0 to disable the timeout as well as the progress output.",
			EnvVars: []string{"CONTAINER_PULL_PROGRESS_TIMEOUT"},
			Value:   defConf.PullProgressTimeout,
		},
		&cli.BoolFlag{
			Name:    "read-only",
			Usage:   "Setup all unprivileged containers to run as read-only. Automatically mounts the containers' tmpfs on '/run', '/tmp' and '/var/tmp'.",
			EnvVars: []string{"CONTAINER_READ_ONLY"},
		},
		&cli.StringFlag{
			Name:    "bind-mount-prefix",
			Usage:   "A prefix to use for the source of the bind mounts. This option would be useful if you were running CRI-O in a container. And had '/' mounted on '/host' in your container. Then if you ran CRI-O with the '--bind-mount-prefix=/host' option, CRI-O would add /host to any bind mounts it is handed over CRI. If Kubernetes asked to have '/var/lib/foobar' bind mounted into the container, then CRI-O would bind mount '/host/var/lib/foobar'. Since CRI-O itself is running in a container with '/' or the host mounted on '/host', the container would end up with '/var/lib/foobar' from the host mounted in the container rather then '/var/lib/foobar' from the CRI-O container.",
			EnvVars: []string{"CONTAINER_BIND_MOUNT_PREFIX"},
		},
		&cli.StringFlag{
			Name:    "uid-mappings",
			Usage:   "Specify the UID mappings to use for the user namespace. This option is deprecated, and will be replaced with Kubernetes user namespace support (KEP-127) in the future.",
			Value:   "",
			EnvVars: []string{"CONTAINER_UID_MAPPINGS"},
		},
		&cli.StringFlag{
			Name:    "gid-mappings",
			Usage:   "Specify the GID mappings to use for the user namespace. This option is deprecated, and will be replaced with Kubernetes user namespace (KEP-127) support in the future.",
			Value:   "",
			EnvVars: []string{"CONTAINER_GID_MAPPINGS"},
		},
		&cli.Int64Flag{
			Name:    "minimum-mappable-uid",
			Usage:   "Specify the lowest host UID which can be specified in mappings for a pod that will be run as a UID other than 0. This option is deprecated, and will be replaced with Kubernetes user namespace support (KEP-127) in the future.",
			Value:   defConf.MinimumMappableUID,
			EnvVars: []string{"CONTAINER_MINIMUM_MAPPABLE_UID"},
		},
		&cli.Int64Flag{
			Name:    "minimum-mappable-gid",
			Usage:   "Specify the lowest host GID which can be specified in mappings for a pod that will be run as a UID other than 0. This option is deprecated, and will be replaced with Kubernetes user namespace support (KEP-127) in the future.",
			Value:   defConf.MinimumMappableGID,
			EnvVars: []string{"CONTAINER_MINIMUM_MAPPABLE_GID"},
		},
		&cli.StringSliceFlag{
			Name:    "allowed-devices",
			Usage:   "Devices a user is allowed to specify with the \"io.kubernetes.cri-o.Devices\" allowed annotation.",
			Value:   cli.NewStringSlice(defConf.AllowedDevices...),
			EnvVars: []string{"CONTAINER_ALLOWED_DEVICES"},
		},
		&cli.StringSliceFlag{
			Name:    "additional-devices",
			Usage:   "Devices to add to the containers.",
			Value:   cli.NewStringSlice(defConf.AdditionalDevices...),
			EnvVars: []string{"CONTAINER_ADDITIONAL_DEVICES"},
		},
		&cli.StringSliceFlag{
			Name:    "cdi-spec-dirs",
			Usage:   "Directories to scan for CDI Spec files.",
			Value:   cli.NewStringSlice(defConf.CDISpecDirs...),
			EnvVars: []string{"CONTAINER_CDI_SPEC_DIRS"},
		},
		&cli.BoolFlag{
			Name:  "device-ownership-from-security-context",
			Usage: "Set devices' uid/gid ownership from runAsUser/runAsGroup.",
			Value: defConf.DeviceOwnershipFromSecurityContext,
		},
		&cli.StringSliceFlag{
			Name:    "conmon-env",
			Value:   cli.NewStringSlice(defConf.ConmonEnv...),
			Usage:   "Environment variable list for the conmon process, used for passing necessary environment variables to conmon or the runtime. This option is deprecated and will be removed in the future.",
			EnvVars: []string{"CONTAINER_CONMON_ENV"},
		},
		&cli.StringSliceFlag{
			Name:    "default-env",
			Value:   cli.NewStringSlice(defConf.DefaultEnv...),
			Usage:   "Additional environment variables to set for all containers.",
			EnvVars: []string{"CONTAINER_DEFAULT_ENV"},
		},
		&cli.StringFlag{
			Name:      "container-attach-socket-dir",
			Usage:     "Path to directory for container attach sockets.",
			Value:     defConf.ContainerAttachSocketDir,
			EnvVars:   []string{"CONTAINER_ATTACH_SOCKET_DIR"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "container-exits-dir",
			Usage:     "Path to directory in which container exit files are written to by conmon.",
			Value:     defConf.ContainerExitsDir,
			EnvVars:   []string{"CONTAINER_EXITS_DIR"},
			TakesFile: true,
		},
		&cli.Int64Flag{
			Name:    "ctr-stop-timeout",
			Usage:   "The minimal amount of time in seconds to wait before issuing a timeout regarding the proper termination of the container. The lowest possible value is 30s, whereas lower values are not considered by CRI-O.",
			Value:   defConf.CtrStopTimeout,
			EnvVars: []string{"CONTAINER_STOP_TIMEOUT"},
		},
		&cli.IntFlag{
			Name:    "grpc-max-recv-msg-size",
			Usage:   "Maximum grpc receive message size in bytes.",
			Value:   defConf.GRPCMaxRecvMsgSize,
			EnvVars: []string{"CONTAINER_GRPC_MAX_RECV_MSG_SIZE"},
		},
		&cli.IntFlag{
			Name:    "grpc-max-send-msg-size",
			Usage:   "Maximum grpc receive message size.",
			Value:   defConf.GRPCMaxSendMsgSize,
			EnvVars: []string{"CONTAINER_GRPC_MAX_SEND_MSG_SIZE"},
		},
		&cli.BoolFlag{
			Name:    "drop-infra-ctr",
			Usage:   "Determines whether pods are created without an infra container, when the pod is not using a pod level PID namespace.",
			EnvVars: []string{"CONTAINER_DROP_INFRA_CTR"},
			Value:   defConf.DropInfraCtr,
		},
		&cli.StringFlag{
			Name:      "pinns-path",
			Usage:     "The path to find the pinns binary, which is needed to manage namespace lifecycle. Will be searched for in $PATH if empty.",
			EnvVars:   []string{"CONTAINER_PINNS_PATH"},
			Value:     defConf.PinnsPath,
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "namespaces-dir",
			Usage:   "The directory where the state of the managed namespaces gets tracked. Only used when manage-ns-lifecycle is true.",
			Value:   defConf.NamespacesDir,
			EnvVars: []string{"CONTAINER_NAMESPACES_DIR"},
		},
		&cli.BoolFlag{
			Name:    "no-pivot",
			Usage:   "If true, the runtime will not use 'pivot_root', but instead use 'MS_MOVE'.",
			EnvVars: []string{"CONTAINER_NO_PIVOT"},
			Value:   defConf.NoPivot,
		},
		&cli.BoolFlag{
			Name:    "stream-enable-tls",
			Usage:   "Enable encrypted TLS transport of the stream server.",
			EnvVars: []string{"CONTAINER_ENABLE_TLS"},
			Value:   defConf.StreamEnableTLS,
		},
		&cli.StringFlag{
			Name:      "stream-tls-ca",
			Usage:     "Path to the x509 CA(s) file used to verify and authenticate client communication with the encrypted stream. This file can change and CRI-O will automatically pick up the changes.",
			EnvVars:   []string{"CONTAINER_TLS_CA"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "stream-tls-cert",
			Usage:     "Path to the x509 certificate file used to serve the encrypted stream. This file can change and CRI-O will automatically pick up the changes.",
			EnvVars:   []string{"CONTAINER_TLS_CERT"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "stream-tls-key",
			Usage:     "Path to the key file used to serve the encrypted stream. This file can change and CRI-O will automatically pick up the changes.",
			EnvVars:   []string{"CONTAINER_TLS_KEY"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "stream-idle-timeout",
			Usage:   "Length of time until open streams terminate due to lack of activity.",
			EnvVars: []string{"STREAM_IDLE_TIMEOUT"},
			Value:   defConf.StreamIdleTimeout,
		},
		&cli.StringFlag{
			Name:        "registries-conf",
			Usage:       "path to the registries.conf file.",
			Destination: &defConf.SystemContext.SystemRegistriesConfPath,
			Hidden:      true,
			EnvVars:     []string{"CONTAINER_REGISTRIES_CONF"},
			TakesFile:   true,
		},
		&cli.StringFlag{
			Name:        "registries-conf-dir",
			Usage:       "path to the registries.conf.d directory.",
			Destination: &defConf.SystemContext.SystemRegistriesConfDirPath,
			Hidden:      true,
			EnvVars:     []string{"CONTAINER_REGISTRIES_CONF_DIR"},
			TakesFile:   true,
		},
		&cli.StringFlag{
			Name:   "address",
			Usage:  "address used for the publish command.",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:      "version-file",
			Usage:     "Location for CRI-O to lay down the temporary version file. It is used to check if crio wipe should wipe containers, which should always happen on a node reboot.",
			Value:     defConf.VersionFile,
			EnvVars:   []string{"CONTAINER_VERSION_FILE"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "version-file-persist",
			Usage:     "Location for CRI-O to lay down the persistent version file. It is used to check if crio wipe should wipe images, which should only happen when CRI-O has been upgraded.",
			Value:     defConf.VersionFilePersist,
			EnvVars:   []string{"CONTAINER_VERSION_FILE_PERSIST"},
			TakesFile: true,
		},
		&cli.BoolFlag{
			Name:    "internal-wipe",
			Usage:   "Whether CRI-O should wipe containers after a reboot and images after an upgrade when the server starts. If set to false, one must run 'crio wipe' to wipe the containers and images in these situations. This option is deprecated, and will be removed in the future.",
			Value:   defConf.InternalWipe,
			EnvVars: []string{"CONTAINER_INTERNAL_WIPE"},
		},
		&cli.BoolFlag{
			Name:    "internal-repair",
			Usage:   "If true, CRI-O will check if the container and image storage was corrupted after a sudden restart, and attempt to repair the storage if it was.",
			EnvVars: []string{"CONTAINER_INTERNAL_REPAIR"},
			Value:   defConf.InternalRepair,
		},
		&cli.StringFlag{
			Name:    "infra-ctr-cpuset",
			Usage:   "CPU set to run infra containers, if not specified CRI-O will use all online CPUs to run infra containers.",
			EnvVars: []string{"CONTAINER_INFRA_CTR_CPUSET"},
			Value:   defConf.InfraCtrCPUSet,
		},
		&cli.StringFlag{
			Name:    "shared-cpuset",
			Usage:   "CPUs set that will be used for guaranteed containers that want access to shared cpus",
			EnvVars: []string{"CONTAINER_SHARED_CPUSET"},
			Value:   defConf.SharedCPUSet,
		},
		&cli.StringFlag{
			Name:      "clean-shutdown-file",
			Usage:     "Location for CRI-O to lay down the clean shutdown file. It indicates whether we've had time to sync changes to disk before shutting down. If not found, crio wipe will clear the storage directory.",
			Value:     defConf.CleanShutdownFile,
			EnvVars:   []string{"CONTAINER_CLEAN_SHUTDOWN_FILE"},
			TakesFile: true,
		},
		&cli.StringSliceFlag{
			Name:    "absent-mount-sources-to-reject",
			Value:   cli.NewStringSlice(defConf.AbsentMountSourcesToReject...),
			Usage:   "A list of paths that, when absent from the host, will cause a container creation to fail (as opposed to the current behavior of creating a directory).",
			EnvVars: []string{"CONTAINER_ABSENT_MOUNT_SOURCES_TO_REJECT"},
		},
		&cli.IntFlag{
			Name:    "stats-collection-period",
			Value:   defConf.StatsCollectionPeriod,
			Usage:   "The number of seconds between collecting pod and container stats. If set to 0, the stats are collected on-demand instead. DEPRECATED: This option will be removed in the future.",
			EnvVars: []string{"CONTAINER_STATS_COLLECTION_PERIOD"},
		},
		&cli.IntFlag{
			Name:    "collection-period",
			Value:   defConf.CollectionPeriod,
			Usage:   "The number of seconds between collecting pod/container stats and pod sandbox metrics. If set to 0, the metrics/stats are collected on-demand instead.",
			EnvVars: []string{"COLLECTION_PERIOD"},
		},
		&cli.StringSliceFlag{
			Name:    "included-pod-metrics",
			Usage:   "A list of pod metrics to include. Specify the names of the metrics to include in this list.",
			EnvVars: []string{"CONTAINER_INCLUDED_POD_METRCIS"},
			Value:   cli.NewStringSlice(defConf.IncludedPodMetrics...),
		},
		&cli.BoolFlag{
			Name:    "enable-criu-support",
			Usage:   "Enable CRIU integration, requires that the criu binary is available in $PATH.",
			EnvVars: []string{"CONTAINER_ENABLE_CRIU_SUPPORT"},
			Value:   false,
		},
		&cli.BoolFlag{
			Name:    "enable-pod-events",
			Usage:   "If true, CRI-O starts sending the container events to the kubelet",
			EnvVars: []string{"ENABLE_POD_EVENTS"},
		},
		&cli.StringFlag{
			Name:  "irqbalance-config-restore-file",
			Value: defConf.IrqBalanceConfigRestoreFile,
			Usage: "Determines if CRI-O should attempt to restore the irqbalance config at startup with the mask in this file. Use the 'disable' value to disable the restore flow entirely.",
		},
		&cli.BoolFlag{
			Name:    "hostnetwork-disable-selinux",
			Usage:   "Determines whether SELinux should be disabled within a pod when it is running in the host network namespace.",
			EnvVars: []string{"CONTAINER_HOSTNETWORK_DISABLE_SELINUX"},
			Value:   defConf.HostNetworkDisableSELinux,
		},
		&cli.StringSliceFlag{
			Name:    "pinned-images",
			Usage:   "A list of images that will be excluded from the kubelet's garbage collection.",
			EnvVars: []string{"CONTAINER_PINNED_IMAGES"},
			Value:   cli.NewStringSlice(defConf.PinnedImages...),
		},
		&cli.BoolFlag{
			Name:    "disable-hostport-mapping",
			Usage:   "If true, CRI-O would disable the hostport mapping.",
			EnvVars: []string{"DISABLE_HOSTPORT_MAPPING"},
			Value:   defConf.DisableHostPortMapping,
		},
		&cli.StringFlag{
			Name:    "timezone",
			Aliases: []string{"tz"},
			Usage:   "To set the timezone for a container in CRI-O. If an empty string is provided, CRI-O retains its default behavior. Use 'Local' to match the timezone of the host machine.",
			EnvVars: []string{"CONTAINER_TIME_ZONE"},
			Value:   defConf.Timezone,
		},
	}
}

// Timestamp returns a string timestamp representation.
func Timestamp() string {
	return strings.ReplaceAll(time.Now().Format(time.RFC3339), ":", "")
}
