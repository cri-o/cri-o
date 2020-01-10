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
	"github.com/urfave/cli/v2"
)

// DefaultsPath is the path to default configuration files set at build time
var DefaultsPath string

// DefaultCommands are the flags commands can be added to every binary
var DefaultCommands = []*cli.Command{
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
	path := ctx.String("config")
	if path != "" {
		if err := config.UpdateFromFile(path); err != nil {
			if ctx.IsSet("config") || !os.IsNotExist(err) {
				return path, err
			}

			// Use the build-time-defined defaults path
			if DefaultsPath != "" && os.IsNotExist(err) {
				path = filepath.Join(DefaultsPath, "/crio.conf")
				if err := config.UpdateFromFile(path); err != nil {
					if ctx.IsSet("config") || !os.IsNotExist(err) {
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
	if ctx.IsSet("conmon") {
		config.Conmon = ctx.String("conmon")
	}
	if ctx.IsSet("pause-command") {
		config.PauseCommand = ctx.String("pause-command")
	}
	if ctx.IsSet("pause-image") {
		config.PauseImage = ctx.String("pause-image")
	}
	if ctx.IsSet("pause-image-auth-file") {
		config.PauseImageAuthFile = ctx.String("pause-image-auth-file")
	}
	if ctx.IsSet("global-auth-file") {
		config.GlobalAuthFile = ctx.String("global-auth-file")
	}
	if ctx.IsSet("signature-policy") {
		config.SignaturePolicyPath = ctx.String("signature-policy")
	}
	if ctx.IsSet("root") {
		config.Root = ctx.String("root")
	}
	if ctx.IsSet("runroot") {
		config.RunRoot = ctx.String("runroot")
	}
	if ctx.IsSet("storage-driver") {
		config.Storage = ctx.String("storage-driver")
	}
	if ctx.IsSet("storage-opt") {
		config.StorageOptions = ctx.StringSlice("storage-opt")
	}
	if ctx.IsSet("insecure-registry") {
		config.InsecureRegistries = ctx.StringSlice("insecure-registry")
	}
	if ctx.IsSet("registry") {
		config.Registries = ctx.StringSlice("registry")
	}
	if ctx.IsSet("default-transport") {
		config.DefaultTransport = ctx.String("default-transport")
	}
	if ctx.IsSet("listen") {
		config.Listen = ctx.String("listen")
	}
	if ctx.IsSet("stream-address") {
		config.StreamAddress = ctx.String("stream-address")
	}
	if ctx.IsSet("host-ip") {
		config.HostIP = ctx.StringSlice("host-ip")
	}
	if ctx.IsSet("stream-port") {
		config.StreamPort = ctx.String("stream-port")
	}
	if ctx.IsSet("default-runtime") {
		config.DefaultRuntime = ctx.String("default-runtime")
	}

	if ctx.IsSet("decryption-keys-path") {
		config.DecryptionKeysPath = ctx.String("decryption-keys-path")
	}

	if ctx.IsSet("runtimes") {
		runtimes := ctx.StringSlice("runtimes")
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
	if ctx.IsSet("selinux") {
		config.SELinux = ctx.Bool("selinux")
	}
	if ctx.IsSet("seccomp-profile") {
		config.SeccompProfile = ctx.String("seccomp-profile")
	}
	if ctx.IsSet("apparmor-profile") {
		config.ApparmorProfile = ctx.String("apparmor-profile")
	}
	if ctx.IsSet("cgroup-manager") {
		config.CgroupManager = ctx.String("cgroup-manager")
	}
	if ctx.IsSet("conmon-cgroup") {
		config.ConmonCgroup = ctx.String("conmon-cgroup")
	}
	if ctx.IsSet("hooks-dir") {
		config.HooksDir = ctx.StringSlice("hooks-dir")
	}
	if ctx.IsSet("default-mounts") {
		config.DefaultMounts = ctx.StringSlice("default-mounts")
	}
	if ctx.IsSet("default-mounts-file") {
		config.DefaultMountsFile = ctx.String("default-mounts-file")
	}
	if ctx.IsSet("default-capabilities") {
		config.DefaultCapabilities = strings.Split(ctx.String("default-capabilities"), ",")
	}
	if ctx.IsSet("default-sysctls") {
		config.DefaultSysctls = strings.Split(ctx.String("default-sysctls"), ",")
	}
	if ctx.IsSet("default-ulimits") {
		config.DefaultUlimits = ctx.StringSlice("default-ulimits")
	}
	if ctx.IsSet("pids-limit") {
		config.PidsLimit = ctx.Int64("pids-limit")
	}
	if ctx.IsSet("log-size-max") {
		config.LogSizeMax = ctx.Int64("log-size-max")
	}
	if ctx.IsSet("log-journald") {
		config.LogToJournald = ctx.Bool("log-journald")
	}
	if ctx.IsSet("cni-config-dir") {
		config.NetworkDir = ctx.String("cni-config-dir")
	}
	if ctx.IsSet("cni-plugin-dir") {
		config.PluginDirs = ctx.StringSlice("cni-plugin-dir")
	}
	if ctx.IsSet("image-volumes") {
		config.ImageVolumes = libconfig.ImageVolumesType(ctx.String("image-volumes"))
	}
	if ctx.IsSet("read-only") {
		config.ReadOnly = ctx.Bool("read-only")
	}
	if ctx.IsSet("bind-mount-prefix") {
		config.BindMountPrefix = ctx.String("bind-mount-prefix")
	}
	if ctx.IsSet("uid-mappings") {
		config.UIDMappings = ctx.String("uid-mappings")
	}
	if ctx.IsSet("gid-mappings") {
		config.GIDMappings = ctx.String("gid-mappings")
	}
	if ctx.IsSet("log-level") {
		config.LogLevel = ctx.String("log-level")
	}
	if ctx.IsSet("log-filter") {
		config.LogFilter = ctx.String("log-filter")
	}
	if ctx.IsSet("log-dir") {
		config.LogDir = ctx.String("log-dir")
	}
	if ctx.IsSet("additional-devices") {
		config.AdditionalDevices = ctx.StringSlice("additional-devices")
	}
	if ctx.IsSet("conmon-env") {
		config.ConmonEnv = ctx.StringSlice("conmon-env")
	}
	if ctx.IsSet("container-attach-socket-dir") {
		config.ContainerAttachSocketDir = ctx.String("container-attach-socket-dir")
	}
	if ctx.IsSet("container-exits-dir") {
		config.ContainerExitsDir = ctx.String("container-exits-dir")
	}
	if ctx.IsSet("ctr-stop-timeout") {
		config.CtrStopTimeout = ctx.Int64("ctr-stop-timeout")
	}
	if ctx.IsSet("grpc-max-recv-msg-size") {
		config.GRPCMaxRecvMsgSize = ctx.Int("grpc-max-recv-msg-size")
	}
	if ctx.IsSet("grpc-max-send-msg-size") {
		config.GRPCMaxSendMsgSize = ctx.Int("grpc-max-send-msg-size")
	}
	if ctx.IsSet("manage-ns-lifecycle") {
		config.ManageNSLifecycle = ctx.Bool("manage-ns-lifecycle")
	}
	if ctx.IsSet("namespaces-dir") {
		config.NamespacesDir = ctx.String("namespaces-dir")
	}
	if ctx.IsSet("pinns-path") {
		config.PinnsPath = ctx.String("pinns-path")
	}
	if ctx.IsSet("no-pivot") {
		config.NoPivot = ctx.Bool("no-pivot")
	}
	if ctx.IsSet("stream-enable-tls") {
		config.StreamEnableTLS = ctx.Bool("stream-enable-tls")
	}
	if ctx.IsSet("stream-tls-ca") {
		config.StreamTLSCA = ctx.String("stream-tls-ca")
	}
	if ctx.IsSet("stream-tls-cert") {
		config.StreamTLSCert = ctx.String("stream-tls-cert")
	}
	if ctx.IsSet("stream-tls-key") {
		config.StreamTLSKey = ctx.String("stream-tls-key")
	}
	if ctx.IsSet("version-file") {
		config.VersionFile = ctx.String("version-file")
	}
	if ctx.IsSet("enable-metrics") {
		config.EnableMetrics = ctx.Bool("enable-metrics")
	}
	if ctx.IsSet("metrics-port") {
		config.MetricsPort = ctx.Int("metrics-port")
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
		&cli.StringFlag{
			Name:      "config",
			Aliases:   []string{"c"},
			Value:     libconfig.CrioConfigPath,
			Usage:     "Path to configuration file",
			EnvVars:   []string{"CONTAINER_CONFIG"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "conmon",
			Usage:     fmt.Sprintf("Path to the conmon binary, used for monitoring the OCI runtime. Will be searched for using $PATH if empty. (default: %q)", defConf.Conmon),
			EnvVars:   []string{"CONTAINER_CONMON"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "conmon-cgroup",
			Usage:   fmt.Sprintf("cgroup to be used for conmon process (default: %q)", defConf.ConmonCgroup),
			EnvVars: []string{"CONTAINER_CONMON_CGROUP"},
		},
		&cli.StringFlag{
			Name:      "listen",
			Usage:     fmt.Sprintf("Path to the CRI-O socket (default: %q)", defConf.Listen),
			EnvVars:   []string{"CONTAINER_LISTEN"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "stream-address",
			Usage:   fmt.Sprintf("Bind address for streaming socket (default: %q)", defConf.StreamAddress),
			EnvVars: []string{"CONTAINER_STREAM_ADDRESS"},
		},
		&cli.StringFlag{
			Name:    "stream-port",
			Usage:   fmt.Sprintf("Bind port for streaming socket (default: %q)", defConf.StreamPort),
			EnvVars: []string{"CONTAINER_STREAM_PORT"},
		},
		&cli.StringFlag{
			Name:      "log",
			Value:     "",
			Usage:     "Set the log file path where internal debug information is written",
			EnvVars:   []string{"CONTAINER_LOG"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "log-format",
			Value:   "text",
			Usage:   "Set the format used by logs: 'text' or 'json'",
			EnvVars: []string{"CONTAINER_LOG_FORMAT"},
		},
		&cli.StringFlag{
			Name:    "log-level",
			Aliases: []string{"l"},
			Value:   "error",
			Usage:   "Log messages above specified level: trace, debug, info, warn, error, fatal or panic",
			EnvVars: []string{"CONTAINER_LOG_LEVEL"},
		},
		&cli.StringFlag{
			Name:    "log-filter",
			Usage:   `Filter the log messages by the provided regular expression. For example 'request.\*' filters all gRPC requests.`,
			EnvVars: []string{"CONTAINER_LOG_FILTER"},
		},
		&cli.StringFlag{
			Name:      "log-dir",
			Value:     "",
			Usage:     "Default log directory where all logs will go unless directly specified by the kubelet",
			EnvVars:   []string{"CONTAINER_LOG_DIR"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "pause-command",
			Usage:   fmt.Sprintf("Path to the pause executable in the pause image (default: %q)", defConf.PauseCommand),
			EnvVars: []string{"CONTAINER_PAUSE_COMMAND"},
		},
		&cli.StringFlag{
			Name:    "pause-image",
			Usage:   fmt.Sprintf("Image which contains the pause executable (default: %q)", defConf.PauseImage),
			EnvVars: []string{"CONTAINER_PAUSE_IMAGE"},
		},
		&cli.StringFlag{
			Name:      "pause-image-auth-file",
			Usage:     fmt.Sprintf("Path to a config file containing credentials for --pause-image (default: %q)", defConf.PauseImageAuthFile),
			EnvVars:   []string{"CONTAINER_PAUSE_IMAGE_AUTH_FILE"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "global-auth-file",
			Usage:     fmt.Sprintf("Path to a file like /var/lib/kubelet/config.json holding credentials necessary for pulling images from secure registries (default: %q)", defConf.GlobalAuthFile),
			EnvVars:   []string{"CONTAINER_GLOBAL_AUTH_FILE"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "signature-policy",
			Usage:     fmt.Sprintf("Path to signature policy JSON file. (default: %q, to use the system-wide default)", defConf.SignaturePolicyPath),
			EnvVars:   []string{"CONTAINER_SIGNATURE_POLICY"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "root",
			Aliases:   []string{"r"},
			Usage:     fmt.Sprintf("The CRI-O root directory (default: %q)", defConf.Root),
			EnvVars:   []string{"CONTAINER_ROOT"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "runroot",
			Usage:     fmt.Sprintf("The CRI-O state directory (default: %q)", defConf.RunRoot),
			EnvVars:   []string{"CONTAINER_RUNROOT"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "storage-driver",
			Aliases: []string{"s"},
			Usage:   fmt.Sprintf("OCI storage driver (default: %q)", defConf.Storage),
			EnvVars: []string{"CONTAINER_STORAGE_DRIVER"},
		},
		&cli.StringSliceFlag{
			Name:    "storage-opt",
			Value:   cli.NewStringSlice(defConf.StorageOptions...),
			Usage:   "OCI storage driver option",
			EnvVars: []string{"CONTAINER_STORAGE_OPT"},
		},
		&cli.StringSliceFlag{
			Name:  "insecure-registry",
			Value: cli.NewStringSlice(defConf.InsecureRegistries...),
			Usage: "Enable insecure registry communication, i.e., enable un-encrypted and/or untrusted communication." + `
    1. List of insecure registries can contain an element with CIDR notation to specify a whole subnet.
    2. Insecure registries accept HTTP or accept HTTPS with certificates from unknown CAs.
    3. Enabling '--insecure-registry' is useful when running a local registry. However, because its use creates security vulnerabilities, **it should ONLY be enabled for testing purposes**. For increased security, users should add their CA to their system's list of trusted CAs instead of using '--insecure-registry'.`,
			EnvVars: []string{"CONTAINER_INSECURE_REGISTRY"},
		},
		&cli.StringSliceFlag{
			Name:    "registry",
			Value:   cli.NewStringSlice(defConf.Registries...),
			Usage:   "Registry to be prepended when pulling unqualified images, can be specified multiple times",
			EnvVars: []string{"CONTAINER_REGISTRY"},
		},
		&cli.StringFlag{
			Name:    "default-transport",
			Usage:   fmt.Sprintf("A prefix to prepend to image names that cannot be pulled as-is (default: %q)", defConf.DefaultTransport),
			EnvVars: []string{"CONTAINER_DEFAULT_TRANSPORT"},
		},
		&cli.StringFlag{
			Name:  "decryption-keys-path",
			Usage: fmt.Sprintf("Path to load keys for image decryption. (default: %q)", defConf.DecryptionKeysPath),
		},
		// XXX: DEPRECATED
		&cli.StringFlag{
			Name:    "runtime",
			Usage:   "OCI runtime path",
			Hidden:  true,
			EnvVars: []string{"CONTAINER_RUNTIME"},
		},
		&cli.StringFlag{
			Name:    "default-runtime",
			Usage:   fmt.Sprintf("Default OCI runtime from the runtimes config (default: %q)", defConf.DefaultRuntime),
			EnvVars: []string{"CONTAINER_DEFAULT_RUNTIME"},
		},
		&cli.StringSliceFlag{
			Name:    "runtimes",
			Usage:   "OCI runtimes, format is runtime_name:runtime_path:runtime_root",
			EnvVars: []string{"CONTAINER_RUNTIMES"},
		},
		&cli.StringFlag{
			Name:      "seccomp-profile",
			Usage:     fmt.Sprintf("Path to the seccomp.json profile to be used as the runtime's default. If not specified, then the internal default seccomp profile will be used. (default: %q)", defConf.SeccompProfile),
			EnvVars:   []string{"CONTAINER_SECCOMP_PROFILE"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "apparmor-profile",
			Usage:   fmt.Sprintf("Name of the apparmor profile to be used as the runtime's default. The default profile name is 'crio-default-' followed by the version string of CRI-O. (default: %q)", defConf.ApparmorProfile),
			EnvVars: []string{"CONTAINER_APPARMOR_PROFILE"},
		},
		&cli.BoolFlag{
			Name:    "selinux",
			Usage:   fmt.Sprintf("Enable selinux support (default: %t)", defConf.SELinux),
			EnvVars: []string{"CONTAINER_SELINUX"},
		},
		&cli.StringFlag{
			Name:    "cgroup-manager",
			Usage:   fmt.Sprintf("cgroup manager (cgroupfs or systemd) (default: %q)", defConf.CgroupManager),
			EnvVars: []string{"CONTAINER_CGROUP_MANAGER"},
		},
		&cli.Int64Flag{
			Name:    "pids-limit",
			Value:   libconfig.DefaultPidsLimit,
			Usage:   "Maximum number of processes allowed in a container",
			EnvVars: []string{"CONTAINER_PIDS_LIMIT"},
		},
		&cli.Int64Flag{
			Name:    "log-size-max",
			Value:   libconfig.DefaultLogSizeMax,
			Usage:   "Maximum log size in bytes for a container. If it is positive, it must be >= 8192 to match/exceed conmon read buffer",
			EnvVars: []string{"CONTAINER_LOG_SIZE_MAX"},
		},
		&cli.BoolFlag{
			Name:    "log-journald",
			Usage:   fmt.Sprintf("Log to systemd journal (journald) in addition to kubernetes log file (default: %t)", defConf.LogToJournald),
			EnvVars: []string{"CONTAINER_LOG_JOURNALD"},
		},
		&cli.StringFlag{
			Name:      "cni-config-dir",
			Usage:     fmt.Sprintf("CNI configuration files directory (default: %q)", defConf.NetworkDir),
			EnvVars:   []string{"CONTAINER_CNI_CONFIG_DIR"},
			TakesFile: true,
		},
		&cli.StringSliceFlag{
			Name:    "cni-plugin-dir",
			Value:   cli.NewStringSlice(defConf.PluginDir),
			Usage:   "CNI plugin binaries directory",
			EnvVars: []string{"CONTAINER_CNI_PLUGIN_DIR"},
		},
		&cli.StringFlag{
			Name:  "image-volumes",
			Value: string(libconfig.ImageVolumesMkdir),
			Usage: "Image volume handling ('mkdir', 'bind', or 'ignore')" + `
    1. mkdir: A directory is created inside the container root filesystem for the volumes.
    2. bind: A directory is created inside container state directory and bind mounted into the container for the volumes.
    3. ignore: All volumes are just ignored and no action is taken.`,
			EnvVars: []string{"CONTAINER_IMAGE_VOLUMES"},
		},
		&cli.StringSliceFlag{
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
			EnvVars: []string{"CONTAINER_HOOKS_DIR"},
		},
		&cli.StringSliceFlag{
			Name:    "default-mounts",
			Usage:   fmt.Sprintf("Add one or more default mount paths in the form host:container (deprecated) (default: %q)", defConf.DefaultMounts),
			EnvVars: []string{"CONTAINER_DEFAULT_MOUNTS"},
		},
		&cli.StringFlag{
			Name:      "default-mounts-file",
			Usage:     fmt.Sprintf("Path to default mounts file (default: %q)", defConf.DefaultMountsFile),
			EnvVars:   []string{"CONTAINER_DEFAULT_MOUNTS_FILE"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:    "default-capabilities",
			Usage:   fmt.Sprintf("Capabilities to add to the containers (default: %q)", defConf.DefaultCapabilities),
			EnvVars: []string{"CONTAINER_DEFAULT_CAPABILITIES"},
		},
		&cli.StringSliceFlag{
			Name:    "default-sysctls",
			Usage:   fmt.Sprintf("Sysctls to add to the containers (default: %q)", defConf.DefaultSysctls),
			EnvVars: []string{"CONTAINER_DEFAULT_SYSCTLS"},
		},
		&cli.StringSliceFlag{
			Name:    "default-ulimits",
			Usage:   fmt.Sprintf("Ulimits to apply to containers by default (name=soft:hard) (default: %q)", defConf.DefaultUlimits),
			EnvVars: []string{"CONTAINER_DEFAULT_ULIMITS"},
		},
		&cli.BoolFlag{
			Name:    "profile",
			Usage:   "Enable pprof remote profiler on localhost:6060",
			EnvVars: []string{"CONTAINER_PROFILE"},
		},
		&cli.IntFlag{
			Name:    "profile-port",
			Value:   6060,
			Usage:   "Port for the pprof profiler",
			EnvVars: []string{"CONTAINER_PROFILE_PORT"},
		},
		&cli.BoolFlag{
			Name:    "enable-metrics",
			Usage:   "Enable metrics endpoint for the server on localhost:9090",
			EnvVars: []string{"CONTAINER_ENABLE_METRICS"},
		},
		&cli.IntFlag{
			Name:    "metrics-port",
			Value:   9090,
			Usage:   "Port for the metrics endpoint",
			EnvVars: []string{"CONTAINER_METRICS_PORT"},
		},
		&cli.BoolFlag{
			Name:    "read-only",
			Usage:   fmt.Sprintf("Setup all unprivileged containers to run as read-only. Automatically mounts tmpfs on `/run`, `/tmp` and `/var/tmp`. (default: %t)", defConf.ReadOnly),
			EnvVars: []string{"CONTAINER_READ_ONLY"},
		},
		&cli.StringFlag{
			Name:    "bind-mount-prefix",
			Usage:   fmt.Sprintf("A prefix to use for the source of the bind mounts. This option would be useful if you were running CRI-O in a container. And had `/` mounted on `/host` in your container. Then if you ran CRI-O with the `--bind-mount-prefix=/host` option, CRI-O would add /host to any bind mounts it is handed over CRI. If Kubernetes asked to have `/var/lib/foobar` bind mounted into the container, then CRI-O would bind mount `/host/var/lib/foobar`. Since CRI-O itself is running in a container with `/` or the host mounted on `/host`, the container would end up with `/var/lib/foobar` from the host mounted in the container rather then `/var/lib/foobar` from the CRI-O container. (default: %q)", defConf.BindMountPrefix),
			EnvVars: []string{"CONTAINER_BIND_MOUNT_PREFIX"},
		},
		&cli.StringFlag{
			Name:    "uid-mappings",
			Usage:   fmt.Sprintf("Specify the UID mappings to use for the user namespace (default: %q)", defConf.UIDMappings),
			Value:   "",
			EnvVars: []string{"CONTAINER_UID_MAPPINGS"},
		},
		&cli.StringFlag{
			Name:    "gid-mappings",
			Usage:   fmt.Sprintf("Specify the GID mappings to use for the user namespace (default: %q)", defConf.GIDMappings),
			Value:   "",
			EnvVars: []string{"CONTAINER_GID_MAPPINGS"},
		},
		&cli.StringSliceFlag{
			Name:    "additional-devices",
			Usage:   "Devices to add to the containers ",
			Value:   cli.NewStringSlice(defConf.AdditionalDevices...),
			EnvVars: []string{"CONTAINER_ADDITIONAL_DEVICES"},
		},
		&cli.StringSliceFlag{
			Name:    "conmon-env",
			Value:   cli.NewStringSlice(defConf.ConmonEnv...),
			Usage:   "Environment variable list for the conmon process, used for passing necessary environment variables to conmon or the runtime",
			EnvVars: []string{"CONTAINER_CONMON_ENV"},
		},
		&cli.StringFlag{
			Name:      "container-attach-socket-dir",
			Usage:     fmt.Sprintf("Path to directory for container attach sockets (default: %q)", defConf.ContainerAttachSocketDir),
			EnvVars:   []string{"CONTAINER_ATTACH_SOCKET_DIR"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "container-exits-dir",
			Usage:     fmt.Sprintf("Path to directory in which container exit files are written to by conmon (default: %q)", defConf.ContainerExitsDir),
			EnvVars:   []string{"CONTAINER_EXITS_DIR"},
			TakesFile: true,
		},
		&cli.Int64Flag{
			Name:    "ctr-stop-timeout",
			Usage:   "The minimal amount of time in seconds to wait before issuing a timeout regarding the proper termination of the container",
			Value:   defConf.CtrStopTimeout,
			EnvVars: []string{"CONTAINER_STOP_TIMEOUT"},
		},
		&cli.IntFlag{
			Name:    "grpc-max-recv-msg-size",
			Usage:   "Maximum grpc receive message size in bytes",
			Value:   defConf.GRPCMaxRecvMsgSize,
			EnvVars: []string{"CONTAINER_GRPC_MAX_RECV_MSG_SIZE"},
		},
		&cli.IntFlag{
			Name:    "grpc-max-send-msg-size",
			Usage:   "Maximum grpc receive message size",
			Value:   defConf.GRPCMaxSendMsgSize,
			EnvVars: []string{"CONTAINER_GRPC_MAX_SEND_MSG_SIZE"},
		},
		&cli.StringSliceFlag{
			Name:    "host-ip",
			Value:   cli.NewStringSlice(defConf.HostIP...),
			Usage:   "Host IPs are the addresses to be used for the host network and can be specified up to two times",
			EnvVars: []string{"CONTAINER_HOST_IP"},
		},
		&cli.BoolFlag{
			Name:    "manage-network-ns-lifecycle",
			Usage:   "Deprecated: this option is being replaced by `manage_ns_lifecycle`, which is described below",
			EnvVars: []string{"CONTAINER_MANAGE_NETWORK_NS_LIFECYCLE"},
		},
		&cli.BoolFlag{
			Name:    "manage-ns-lifecycle",
			Usage:   fmt.Sprintf("Determines whether we pin and remove IPC, network and UTS namespaces and manage their lifecycle (default: %v)", defConf.ManageNSLifecycle),
			EnvVars: []string{"CONTAINER_MANAGE_NS_LIFECYCLE"},
		},
		&cli.StringFlag{
			Name:    "pinns-path",
			Usage:   fmt.Sprintf("The path to find the pinns binary, which is needed to manage namespace lifecycle. Will be searched for in $PATH if empty (default: %q)", defConf.PinnsPath),
			EnvVars: []string{"CONTAINER_PINNS_PATH"},
		},
		&cli.StringFlag{
			Name:    "namespaces-dir",
			Usage:   fmt.Sprintf("The directory where the state of the managed namespaces gets tracked. Only used when manage-ns-lifecycle is true (default: %q)", defConf.NamespacesDir),
			EnvVars: []string{"CONTAINER_NAMESPACES_DIR"},
		},
		&cli.BoolFlag{
			Name:    "no-pivot",
			Usage:   fmt.Sprintf("If true, the runtime will not use `pivot_root`, but instead use `MS_MOVE` (default: %v)", defConf.NoPivot),
			EnvVars: []string{"CONTAINER_NO_PIVOT"},
		},
		&cli.BoolFlag{
			Name:    "stream-enable-tls",
			Usage:   fmt.Sprintf("Enable encrypted TLS transport of the stream server (default: %v)", defConf.StreamEnableTLS),
			EnvVars: []string{"CONTAINER_ENABLE_TLS"},
		},
		&cli.StringFlag{
			Name:      "stream-tls-ca",
			Usage:     fmt.Sprintf("Path to the x509 CA(s) file used to verify and authenticate client communication with the encrypted stream. This file can change and CRI-O will automatically pick up the changes within 5 minutes (default: %q)", defConf.StreamTLSCA),
			EnvVars:   []string{"CONTAINER_TLS_CA"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "stream-tls-cert",
			Usage:     fmt.Sprintf("Path to the x509 certificate file used to serve the encrypted stream. This file can change and CRI-O will automatically pick up the changes within 5 minutes (default: %q)", defConf.StreamTLSCert),
			EnvVars:   []string{"CONTAINER_TLS_CERT"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      "stream-tls-key",
			Usage:     fmt.Sprintf("Path to the key file used to serve the encrypted stream. This file can change and CRI-O will automatically pick up the changes within 5 minutes (default: %q)", defConf.StreamTLSKey),
			EnvVars:   []string{"CONTAINER_TLS_KEY"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:        "registries-conf",
			Usage:       "path to the registries.conf file",
			Destination: &systemContext.SystemRegistriesConfPath,
			Hidden:      true,
			EnvVars:     []string{"CONTAINER_REGISTRIES_CONF"},
			TakesFile:   true,
		},
		&cli.StringFlag{
			Name:      "version-file",
			Usage:     fmt.Sprintf("Location for CRI-O to lay down the version file (default: %s)", defConf.VersionFile),
			EnvVars:   []string{"CONTAINER_VERSION_FILE"},
			TakesFile: true,
		},
	}
}
