package criocli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/types"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// DefaultsPath is the path to default configuration files set at build time
var DefaultsPath string

// DefaultCommands are the flags commands can be added to every binary
var DefaultCommands = []cli.Command{
	completion(),
	man(),
	markdown(),
}

func GetConfigFromContext(c *cli.Context) (string, *libconfig.Config, error) {
	config, ok := c.App.Metadata["config"].(*libconfig.Config)
	if !ok {
		return "", nil, fmt.Errorf("type assertion error when accessing server config")
	}
	configPath, err := mergeConfig(config, c)
	if err != nil {
		return "", nil, err
	}
	return configPath, config, nil
}

func mergeConfig(config *libconfig.Config, ctx *cli.Context) (string, error) {
	// Don't parse the config if the user explicitly set it to "".
	path := ctx.GlobalString("config")
	if path != "" {
		if err := config.UpdateFromFile(path); err != nil {
			if ctx.GlobalIsSet("config") || !os.IsNotExist(err) {
				return path, err
			}

			// Use the build-time-defined defaults path
			if DefaultsPath != "" && os.IsNotExist(err) {
				path = filepath.Join(DefaultsPath, "/crio.conf")
				if err := config.UpdateFromFile(path); err != nil {
					if ctx.GlobalIsSet("config") || !os.IsNotExist(err) {
						return path, err
					}
				}
			}

			// We don't error out if --config wasn't explicitly set and the
			// default doesn't exist. But we will log a warning about it, so
			// the user doesn't miss it.
			logrus.Warnf("default configuration file does not exist: %s", path)
		}
	}

	// Override options set with the CLI.
	if ctx.GlobalIsSet("conmon") {
		config.Conmon = ctx.GlobalString("conmon")
	}
	if ctx.GlobalIsSet("pause-command") {
		config.PauseCommand = ctx.GlobalString("pause-command")
	}
	if ctx.GlobalIsSet("pause-image") {
		config.PauseImage = ctx.GlobalString("pause-image")
	}
	if ctx.GlobalIsSet("pause-image-auth-file") {
		config.PauseImageAuthFile = ctx.GlobalString("pause-image-auth-file")
	}
	if ctx.GlobalIsSet("global-auth-file") {
		config.GlobalAuthFile = ctx.GlobalString("global-auth-file")
	}
	if ctx.GlobalIsSet("signature-policy") {
		config.SignaturePolicyPath = ctx.GlobalString("signature-policy")
	}
	if ctx.GlobalIsSet("root") {
		config.Root = ctx.GlobalString("root")
	}
	if ctx.GlobalIsSet("runroot") {
		config.RunRoot = ctx.GlobalString("runroot")
	}
	if ctx.GlobalIsSet("storage-driver") {
		config.Storage = ctx.GlobalString("storage-driver")
	}
	if ctx.GlobalIsSet("storage-opt") {
		config.StorageOptions = ctx.GlobalStringSlice("storage-opt")
	}
	if ctx.GlobalIsSet("insecure-registry") {
		config.InsecureRegistries = ctx.GlobalStringSlice("insecure-registry")
	}
	if ctx.GlobalIsSet("registry") {
		config.Registries = ctx.GlobalStringSlice("registry")
	}
	if ctx.GlobalIsSet("default-transport") {
		config.DefaultTransport = ctx.GlobalString("default-transport")
	}
	if ctx.GlobalIsSet("listen") {
		config.Listen = ctx.GlobalString("listen")
	}
	if ctx.GlobalIsSet("stream-address") {
		config.StreamAddress = ctx.GlobalString("stream-address")
	}
	if ctx.GlobalIsSet("host-ip") {
		config.HostIP = ctx.GlobalStringSlice("host-ip")
	}
	if ctx.GlobalIsSet("stream-port") {
		config.StreamPort = ctx.GlobalString("stream-port")
	}
	if ctx.GlobalIsSet("default-runtime") {
		config.DefaultRuntime = ctx.GlobalString("default-runtime")
	}
	if ctx.GlobalIsSet("runtimes") {
		runtimes := ctx.GlobalStringSlice("runtimes")
		for _, r := range runtimes {
			fields := strings.Split(r, ":")
			if len(fields) != 3 {
				return path, fmt.Errorf("wrong format for --runtimes: %q", r)
			}
			config.Runtimes[fields[0]] = &libconfig.RuntimeHandler{
				RuntimePath: fields[1],
				RuntimeRoot: fields[2],
			}
		}
	}
	if ctx.GlobalIsSet("selinux") {
		config.SELinux = ctx.GlobalBool("selinux")
	}
	if ctx.GlobalIsSet("seccomp-profile") {
		config.SeccompProfile = ctx.GlobalString("seccomp-profile")
	}
	if ctx.GlobalIsSet("apparmor-profile") {
		config.ApparmorProfile = ctx.GlobalString("apparmor-profile")
	}
	if ctx.GlobalIsSet("cgroup-manager") {
		config.CgroupManager = ctx.GlobalString("cgroup-manager")
	}
	if ctx.GlobalIsSet("conmon-cgroup") {
		config.ConmonCgroup = ctx.GlobalString("conmon-cgroup")
	}
	if ctx.GlobalIsSet("hooks-dir") {
		config.HooksDir = ctx.GlobalStringSlice("hooks-dir")
	}
	if ctx.GlobalIsSet("default-mounts") {
		config.DefaultMounts = ctx.GlobalStringSlice("default-mounts")
	}
	if ctx.GlobalIsSet("default-mounts-file") {
		config.DefaultMountsFile = ctx.GlobalString("default-mounts-file")
	}
	if ctx.GlobalIsSet("default-capabilities") {
		config.DefaultCapabilities = strings.Split(ctx.GlobalString("default-capabilities"), ",")
	}
	if ctx.GlobalIsSet("default-sysctls") {
		config.DefaultSysctls = strings.Split(ctx.GlobalString("default-sysctls"), ",")
	}
	if ctx.GlobalIsSet("default-ulimits") {
		config.DefaultUlimits = ctx.GlobalStringSlice("default-ulimits")
	}
	if ctx.GlobalIsSet("pids-limit") {
		config.PidsLimit = ctx.GlobalInt64("pids-limit")
	}
	if ctx.GlobalIsSet("log-size-max") {
		config.LogSizeMax = ctx.GlobalInt64("log-size-max")
	}
	if ctx.GlobalIsSet("log-journald") {
		config.LogToJournald = ctx.GlobalBool("log-journald")
	}
	if ctx.GlobalIsSet("cni-config-dir") {
		config.NetworkDir = ctx.GlobalString("cni-config-dir")
	}
	if ctx.GlobalIsSet("cni-plugin-dir") {
		config.PluginDirs = ctx.GlobalStringSlice("cni-plugin-dir")
	}
	if ctx.GlobalIsSet("image-volumes") {
		config.ImageVolumes = libconfig.ImageVolumesType(ctx.GlobalString("image-volumes"))
	}
	if ctx.GlobalIsSet("read-only") {
		config.ReadOnly = ctx.GlobalBool("read-only")
	}
	if ctx.GlobalIsSet("bind-mount-prefix") {
		config.BindMountPrefix = ctx.GlobalString("bind-mount-prefix")
	}
	if ctx.GlobalIsSet("uid-mappings") {
		config.UIDMappings = ctx.GlobalString("uid-mappings")
	}
	if ctx.GlobalIsSet("gid-mappings") {
		config.GIDMappings = ctx.GlobalString("gid-mappings")
	}
	if ctx.GlobalIsSet("log-level") {
		config.LogLevel = ctx.GlobalString("log-level")
	}
	if ctx.GlobalIsSet("log-filter") {
		config.LogFilter = ctx.GlobalString("log-filter")
	}
	if ctx.GlobalIsSet("log-dir") {
		config.LogDir = ctx.GlobalString("log-dir")
	}
	if ctx.GlobalIsSet("additional-devices") {
		config.AdditionalDevices = ctx.GlobalStringSlice("additional-devices")
	}
	if ctx.GlobalIsSet("conmon-env") {
		config.ConmonEnv = ctx.GlobalStringSlice("conmon-env")
	}
	if ctx.GlobalIsSet("container-attach-socket-dir") {
		config.ContainerAttachSocketDir = ctx.GlobalString("container-attach-socket-dir")
	}
	if ctx.GlobalIsSet("container-exits-dir") {
		config.ContainerExitsDir = ctx.GlobalString("container-exits-dir")
	}
	if ctx.GlobalIsSet("ctr-stop-timeout") {
		config.CtrStopTimeout = ctx.GlobalInt64("ctr-stop-timeout")
	}
	if ctx.GlobalIsSet("grpc-max-recv-msg-size") {
		config.GRPCMaxRecvMsgSize = ctx.GlobalInt("grpc-max-recv-msg-size")
	}
	if ctx.GlobalIsSet("grpc-max-send-msg-size") {
		config.GRPCMaxSendMsgSize = ctx.GlobalInt("grpc-max-send-msg-size")
	}
	if ctx.GlobalIsSet("manage-network-ns-lifecycle") {
		config.ManageNetworkNSLifecycle = ctx.GlobalBool("manage-network-ns-lifecycle")
	}
	if ctx.GlobalIsSet("no-pivot") {
		config.NoPivot = ctx.GlobalBool("no-pivot")
	}
	if ctx.GlobalIsSet("stream-enable-tls") {
		config.StreamEnableTLS = ctx.GlobalBool("stream-enable-tls")
	}
	if ctx.GlobalIsSet("stream-tls-ca") {
		config.StreamTLSCA = ctx.GlobalString("stream-tls-ca")
	}
	if ctx.GlobalIsSet("stream-tls-cert") {
		config.StreamTLSCert = ctx.GlobalString("stream-tls-cert")
	}
	if ctx.GlobalIsSet("stream-tls-key") {
		config.StreamTLSKey = ctx.GlobalString("stream-tls-key")
	}
	if ctx.GlobalIsSet("version-file") {
		config.VersionFile = ctx.GlobalString("version-file")
	}
	if ctx.GlobalIsSet("enable-metrics") {
		config.EnableMetrics = ctx.GlobalBool("enable-metrics")
	}
	if ctx.GlobalIsSet("metrics-port") {
		config.MetricsPort = ctx.GlobalInt("metrics-port")
	}

	return path, nil
}

func GetFlagsAndMetadata(systemContext *types.SystemContext) ([]cli.Flag, map[string]interface{}, error) {
	config, err := libconfig.DefaultConfig()
	if err != nil {
		return nil, nil, errors.Errorf("error loading server config: %v", err)
	}

	// TODO FIXME should be crio wipe flags
	flags := getCrioFlags(config, systemContext)

	metadata := map[string]interface{}{
		"config": config,
	}
	return flags, metadata, nil
}

func getCrioFlags(defConf *libconfig.Config, systemContext *types.SystemContext) []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:      "config, c",
			Value:     libconfig.CrioConfigPath,
			Usage:     "Path to configuration file",
			EnvVar:    "CONTAINER_CONFIG",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:      "conmon",
			Usage:     fmt.Sprintf("Path to the conmon binary, used for monitoring the OCI runtime. Will be searched for using $PATH if empty. (default: %q)", defConf.Conmon),
			EnvVar:    "CONTAINER_CONMON",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:   "conmon-cgroup",
			Usage:  fmt.Sprintf("cgroup to be used for conmon process (default: %q)", defConf.ConmonCgroup),
			EnvVar: "CONTAINER_CONMON_CGROUP",
		},
		cli.StringFlag{
			Name:      "listen",
			Usage:     fmt.Sprintf("Path to the CRI-O socket (default: %q)", defConf.Listen),
			EnvVar:    "CONTAINER_LISTEN",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:   "stream-address",
			Usage:  fmt.Sprintf("Bind address for streaming socket (default: %q)", defConf.StreamAddress),
			EnvVar: "CONTAINER_STREAM_ADDRESS",
		},
		cli.StringFlag{
			Name:   "stream-port",
			Usage:  fmt.Sprintf("Bind port for streaming socket (default: %q)", defConf.StreamPort),
			EnvVar: "CONTAINER_STREAM_PORT",
		},
		cli.StringFlag{
			Name:      "log",
			Value:     "",
			Usage:     "Set the log file path where internal debug information is written",
			EnvVar:    "CONTAINER_LOG",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:   "log-format",
			Value:  "text",
			Usage:  "Set the format used by logs ('text' (default), or 'json')",
			EnvVar: "CONTAINER_LOG_FORMAT",
		},
		cli.StringFlag{
			Name:   "log-level, l",
			Value:  "error",
			Usage:  "Log messages above specified level: trace, debug, info, warn, error (default), fatal or panic",
			EnvVar: "CONTAINER_LOG_LEVEL",
		},
		cli.StringFlag{
			Name:   "log-filter",
			Usage:  `Filter the log messages by the provided regular expression. For example 'request.\*' filters all gRPC requests.`,
			EnvVar: "CONTAINER_LOG_FILTER",
		},
		cli.StringFlag{
			Name:      "log-dir",
			Value:     "",
			Usage:     "Default log directory where all logs will go unless directly specified by the kubelet",
			EnvVar:    "CONTAINER_LOG_DIR",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:   "pause-command",
			Usage:  fmt.Sprintf("Path to the pause executable in the pause image (default: %q)", defConf.PauseCommand),
			EnvVar: "CONTAINER_PAUSE_COMMAND",
		},
		cli.StringFlag{
			Name:   "pause-image",
			Usage:  fmt.Sprintf("Image which contains the pause executable (default: %q)", defConf.PauseImage),
			EnvVar: "CONTAINER_PAUSE_IMAGE",
		},
		cli.StringFlag{
			Name:      "pause-image-auth-file",
			Usage:     fmt.Sprintf("Path to a config file containing credentials for --pause-image (default: %q)", defConf.PauseImageAuthFile),
			EnvVar:    "CONTAINER_PAUSE_IMAGE_AUTH_FILE",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:      "global-auth-file",
			Usage:     fmt.Sprintf("Path to a file like /var/lib/kubelet/config.json holding credentials necessary for pulling images from secure registries (default: %q)", defConf.GlobalAuthFile),
			EnvVar:    "CONTAINER_GLOBAL_AUTH_FILE",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:      "signature-policy",
			Usage:     fmt.Sprintf("Path to signature policy JSON file. (default: %q, to use the system-wide default)", defConf.SignaturePolicyPath),
			EnvVar:    "CONTAINER_SIGNATURE_POLICY",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:      "root, r",
			Usage:     fmt.Sprintf("The CRI-O root directory (default: %q)", defConf.Root),
			EnvVar:    "CONTAINER_ROOT",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:      "runroot",
			Usage:     fmt.Sprintf("The CRI-O state directory (default: %q)", defConf.RunRoot),
			EnvVar:    "CONTAINER_RUNROOT",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:   "storage-driver, s",
			Usage:  fmt.Sprintf("OCI storage driver (default: %q)", defConf.Storage),
			EnvVar: "CONTAINER_STORAGE_DRIVER",
		},
		cli.StringSliceFlag{
			Name:   "storage-opt",
			Usage:  fmt.Sprintf("OCI storage driver option (default: %q)", defConf.StorageOptions),
			EnvVar: "CONTAINER_STORAGE_OPT",
		},
		cli.StringSliceFlag{
			Name: "insecure-registry",
			Usage: "Enable insecure registry communication, i.e., enable un-encrypted and/or untrusted communication." + `
    1. List of insecure registries can contain an element with CIDR notation to specify a whole subnet.
    2. Insecure registries accept HTTP or accept HTTPS with certificates from unknown CAs.
    3. Enabling '--insecure-registry' is useful when running a local registry. However, because its use creates security vulnerabilities, **it should ONLY be enabled for testing purposes**. For increased security, users should add their CA to their system's list of trusted CAs instead of using '--insecure-registry'.`,
			EnvVar: "CONTAINER_INSECURE_REGISTRY",
		},
		cli.StringSliceFlag{
			Name:   "registry",
			Usage:  fmt.Sprintf("Registry to be prepended when pulling unqualified images, can be specified multiple times (default: configured in /etc/containers/registries.conf)"),
			EnvVar: "CONTAINER_REGISTRY",
		},
		cli.StringFlag{
			Name:   "default-transport",
			Usage:  fmt.Sprintf("A prefix to prepend to image names that cannot be pulled as-is (default: %q)", defConf.DefaultTransport),
			EnvVar: "CONTAINER_DEFAULT_TRANSPORT",
		},
		// XXX: DEPRECATED
		cli.StringFlag{
			Name:   "runtime",
			Usage:  "OCI runtime path",
			Hidden: true,
			EnvVar: "CONTAINER_RUNTIME",
		},
		cli.StringFlag{
			Name:   "default-runtime",
			Usage:  fmt.Sprintf("Default OCI runtime from the runtimes config (default: %q)", defConf.DefaultRuntime),
			EnvVar: "CONTAINER_DEFAULT_RUNTIME",
		},
		cli.StringSliceFlag{
			Name:   "runtimes",
			Usage:  "OCI runtimes, format is runtime_name:runtime_path:runtime_root",
			EnvVar: "CONTAINER_RUNTIMES",
		},
		cli.StringFlag{
			Name:      "seccomp-profile",
			Usage:     fmt.Sprintf("Path to the seccomp.json profile to be used as the runtime's default. If not specified, then the internal default seccomp profile will be used. (default: %q)", defConf.SeccompProfile),
			EnvVar:    "CONTAINER_SECCOMP_PROFILE",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:   "apparmor-profile",
			Usage:  fmt.Sprintf("Name of the apparmor profile to be used as the runtime's default. The default profile name is 'crio-default-' followed by the version string of CRI-O. (default: %q)", defConf.ApparmorProfile),
			EnvVar: "CONTAINER_APPARMOR_PROFILE",
		},
		cli.BoolFlag{
			Name:   "selinux",
			Usage:  fmt.Sprintf("Enable selinux support (default: %t)", defConf.SELinux),
			EnvVar: "CONTAINER_SELINUX",
		},
		cli.StringFlag{
			Name:   "cgroup-manager",
			Usage:  fmt.Sprintf("cgroup manager (cgroupfs or systemd) (default: %q)", defConf.CgroupManager),
			EnvVar: "CONTAINER_CGROUP_MANAGER",
		},
		cli.Int64Flag{
			Name:   "pids-limit",
			Value:  libconfig.DefaultPidsLimit,
			Usage:  fmt.Sprintf("Maximum number of processes allowed in a container (default: %d)", libconfig.DefaultPidsLimit),
			EnvVar: "CONTAINER_PIDS_LIMIT",
		},
		cli.Int64Flag{
			Name:   "log-size-max",
			Value:  libconfig.DefaultLogSizeMax,
			Usage:  fmt.Sprintf("Maximum log size in bytes for a container. If it is positive, it must be >= 8192 (to match/exceed conmon read buffer) (default: %d, no limit)", libconfig.DefaultLogSizeMax),
			EnvVar: "CONTAINER_LOG_SIZE_MAX",
		},
		cli.BoolFlag{
			Name:   "log-journald",
			Usage:  fmt.Sprintf("Log to systemd journal (journald) in addition to kubernetes log file (default: %t)", defConf.LogToJournald),
			EnvVar: "CONTAINER_LOG_JOURNALD",
		},
		cli.StringFlag{
			Name:      "cni-config-dir",
			Usage:     fmt.Sprintf("CNI configuration files directory (default: %q)", defConf.NetworkDir),
			EnvVar:    "CONTAINER_CNI_CONFIG_DIR",
			TakesFile: true,
		},
		cli.StringSliceFlag{
			Name:   "cni-plugin-dir",
			Usage:  fmt.Sprintf("CNI plugin binaries directory (default: %q)", defConf.PluginDir),
			EnvVar: "CONTAINER_CNI_PLUGIN_DIR",
		},
		cli.StringFlag{
			Name:  "image-volumes",
			Value: string(libconfig.ImageVolumesMkdir),
			Usage: "Image volume handling ('mkdir', 'bind', or 'ignore')" + `
    1. mkdir: A directory is created inside the container root filesystem for the volumes.
    2. bind: A directory is created inside container state directory and bind mounted into the container for the volumes.
    3. ignore: All volumes are just ignored and no action is taken.`,
			EnvVar: "CONTAINER_IMAGE_VOLUMES",
		},
		cli.StringSliceFlag{
			Name: "hooks-dir",
			Usage: fmt.Sprintf("Set the OCI hooks directory path (may be set multiple times) (default: %q)", defConf.HooksDir) + `
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
			EnvVar: "CONTAINER_HOOKS_DIR",
		},
		cli.StringSliceFlag{
			Name:   "default-mounts",
			Usage:  fmt.Sprintf("Add one or more default mount paths in the form host:container (deprecated) (default: %q)", defConf.DefaultMounts),
			EnvVar: "CONTAINER_DEFAULT_MOUNTS",
		},
		cli.StringFlag{
			Name:      "default-mounts-file",
			Usage:     fmt.Sprintf("Path to default mounts file (default: %q)", defConf.DefaultMountsFile),
			EnvVar:    "CONTAINER_DEFAULT_MOUNTS_FILE",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:   "default-capabilities",
			Usage:  fmt.Sprintf("Capabilities to add to the containers (default: %q)", defConf.DefaultCapabilities),
			EnvVar: "CONTAINER_DEFAULT_CAPABILITIES",
		},
		cli.StringSliceFlag{
			Name:   "default-sysctls",
			Usage:  fmt.Sprintf("Sysctls to add to the containers (default: %q)", defConf.DefaultSysctls),
			EnvVar: "CONTAINER_DEFAULT_SYSCTLS",
		},
		cli.StringSliceFlag{
			Name:   "default-ulimits",
			Usage:  fmt.Sprintf("Ulimits to apply to containers by default (name=soft:hard) (default: %q)", defConf.DefaultUlimits),
			EnvVar: "CONTAINER_DEFAULT_ULIMITS",
		},
		cli.BoolFlag{
			Name:   "profile",
			Usage:  "Enable pprof remote profiler on localhost:6060",
			EnvVar: "CONTAINER_PROFILE",
		},
		cli.IntFlag{
			Name:   "profile-port",
			Value:  6060,
			Usage:  "Port for the pprof profiler (default: 6060)",
			EnvVar: "CONTAINER_PROFILE_PORT",
		},
		cli.BoolFlag{
			Name:   "enable-metrics",
			Usage:  "Enable metrics endpoint for the server on localhost:9090",
			EnvVar: "CONTAINER_ENABLE_METRICS",
		},
		cli.IntFlag{
			Name:   "metrics-port",
			Value:  9090,
			Usage:  "Port for the metrics endpoint (default: 9090)",
			EnvVar: "CONTAINER_METRICS_PORT",
		},
		cli.BoolFlag{
			Name:   "read-only",
			Usage:  fmt.Sprintf("Setup all unprivileged containers to run as read-only. Automatically mounts tmpfs on `/run`, `/tmp` and `/var/tmp`. (default: %t)", defConf.ReadOnly),
			EnvVar: "CONTAINER_READ_ONLY",
		},
		cli.StringFlag{
			Name:   "bind-mount-prefix",
			Usage:  fmt.Sprintf("A prefix to use for the source of the bind mounts. This option would be useful if you were running CRI-O in a container. And had `/` mounted on `/host` in your container. Then if you ran CRI-O with the `--bind-mount-prefix=/host` option, CRI-O would add /host to any bind mounts it is handed over CRI. If Kubernetes asked to have `/var/lib/foobar` bind mounted into the container, then CRI-O would bind mount `/host/var/lib/foobar`. Since CRI-O itself is running in a container with `/` or the host mounted on `/host`, the container would end up with `/var/lib/foobar` from the host mounted in the container rather then `/var/lib/foobar` from the CRI-O container. (default: %q)", defConf.BindMountPrefix),
			EnvVar: "CONTAINER_BIND_MOUNT_PREFIX",
		},
		cli.StringFlag{
			Name:   "uid-mappings",
			Usage:  fmt.Sprintf("Specify the UID mappings to use for the user namespace (default: %q)", defConf.UIDMappings),
			Value:  "",
			EnvVar: "CONTAINER_UID_MAPPINGS",
		},
		cli.StringFlag{
			Name:   "gid-mappings",
			Usage:  fmt.Sprintf("Specify the GID mappings to use for the user namespace (default: %q)", defConf.GIDMappings),
			Value:  "",
			EnvVar: "CONTAINER_GID_MAPPINGS",
		},
		cli.StringSliceFlag{
			Name:   "additional-devices",
			Usage:  fmt.Sprintf("Devices to add to the containers (default: %q)", defConf.AdditionalDevices),
			EnvVar: "CONTAINER_ADDITIONAL_DEVICES",
		},
		cli.StringSliceFlag{
			Name:   "conmon-env",
			Usage:  fmt.Sprintf("Environment variable list for the conmon process, used for passing necessary environment variables to conmon or the runtime (default: %q)", defConf.ConmonEnv),
			EnvVar: "CONTAINER_CONMON_ENV",
		},
		cli.StringFlag{
			Name:      "container-attach-socket-dir",
			Usage:     fmt.Sprintf("Path to directory for container attach sockets (default: %q)", defConf.ContainerAttachSocketDir),
			EnvVar:    "CONTAINER_ATTACH_SOCKET_DIR",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:      "container-exits-dir",
			Usage:     fmt.Sprintf("Path to directory in which container exit files are written to by conmon (default: %q)", defConf.ContainerExitsDir),
			EnvVar:    "CONTAINER_EXITS_DIR",
			TakesFile: true,
		},
		cli.Int64Flag{
			Name:   "ctr-stop-timeout",
			Usage:  fmt.Sprintf("The minimal amount of time in seconds to wait before issuing a timeout regarding the proper termination of the container (default: %q)", defConf.CtrStopTimeout),
			EnvVar: "CONTAINER_STOP_TIMEOUT",
		},
		cli.IntFlag{
			Name:   "grpc-max-recv-msg-size",
			Usage:  fmt.Sprintf("Maximum grpc receive message size in bytes (default: %q)", defConf.GRPCMaxRecvMsgSize),
			EnvVar: "CONTAINER_GRPC_MAX_RECV_MSG_SIZE",
		},
		cli.IntFlag{
			Name:   "grpc-max-send-msg-size",
			Usage:  fmt.Sprintf("Maximum grpc receive message size (default: %q)", defConf.GRPCMaxSendMsgSize),
			EnvVar: "CONTAINER_GRPC_MAX_SEND_MSG_SIZE",
		},
		cli.StringSliceFlag{
			Name:   "host-ip",
			Usage:  fmt.Sprintf("Host IPs are the addresses to be used for the host network and can be specified up to two times (default: %q)", defConf.HostIP),
			EnvVar: "CONTAINER_HOST_IP",
		},
		cli.BoolFlag{
			Name:   "manage-network-ns-lifecycle",
			Usage:  fmt.Sprintf("Determines whether we pin and remove network namespace and manage its lifecycle (default: %v)", defConf.ManageNetworkNSLifecycle),
			EnvVar: "CONTAINER_MANAGE_NETWORK_NS_LIFECYCLE",
		},
		cli.BoolFlag{
			Name:   "no-pivot",
			Usage:  fmt.Sprintf("If true, the runtime will not use `pivot_root`, but instead use `MS_MOVE` (default: %v)", defConf.NoPivot),
			EnvVar: "CONTAINER_NO_PIVOT",
		},
		cli.BoolFlag{
			Name:   "stream-enable-tls",
			Usage:  fmt.Sprintf("Enable encrypted TLS transport of the stream server (default: %v)", defConf.StreamEnableTLS),
			EnvVar: "CONTAINER_ENABLE_TLS",
		},
		cli.StringFlag{
			Name:      "stream-tls-ca",
			Usage:     fmt.Sprintf("Path to the x509 CA(s) file used to verify and authenticate client communication with the encrypted stream. This file can change and CRI-O will automatically pick up the changes within 5 minutes (default: %q)", defConf.StreamTLSCA),
			EnvVar:    "CONTAINER_TLS_CA",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:      "stream-tls-cert",
			Usage:     fmt.Sprintf("Path to the x509 certificate file used to serve the encrypted stream. This file can change and CRI-O will automatically pick up the changes within 5 minutes (default: %q)", defConf.StreamTLSCert),
			EnvVar:    "CONTAINER_TLS_CERT",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:      "stream-tls-key",
			Usage:     fmt.Sprintf("Path to the key file used to serve the encrypted stream. This file can change and CRI-O will automatically pick up the changes within 5 minutes (default: %q)", defConf.StreamTLSKey),
			EnvVar:    "CONTAINER_TLS_KEY",
			TakesFile: true,
		},
		cli.StringFlag{
			Name:        "registries-conf",
			Usage:       "path to the registries.conf file",
			Destination: &systemContext.SystemRegistriesConfPath,
			Hidden:      true,
			EnvVar:      "CONTAINER_REGISTRIES_CONF",
			TakesFile:   true,
		},
		cli.StringFlag{
			Name:      "version-file",
			Usage:     fmt.Sprintf("Location for CRI-O to lay down the version file (default: %s)", defConf.VersionFile),
			EnvVar:    "CONTAINER_VERSION_FILE",
			TakesFile: true,
		},
	}
}
