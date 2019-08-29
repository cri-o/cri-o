package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cri-o/cri-o/lib"
	"github.com/cri-o/cri-o/oci"
	"github.com/cri-o/cri-o/server"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// DefaultsPath is the path to default configuration files set at build time
var DefaultsPath string

func GetConfigFromContext(c *cli.Context) (*server.Config, error) {
	config, ok := c.App.Metadata["config"].(*server.Config)
	if !ok {
		return nil, fmt.Errorf("type assertion error when accessing server config")
	}
	err := mergeConfig(config, c)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func mergeConfig(config *server.Config, ctx *cli.Context) error {
	// Don't parse the config if the user explicitly set it to "".
	if path := ctx.GlobalString("config"); path != "" {
		if err := config.UpdateFromFile(path); err != nil {
			if ctx.GlobalIsSet("config") || !os.IsNotExist(err) {
				return err
			}

			// Use the build-time-defined defaults path
			if DefaultsPath != "" && os.IsNotExist(err) {
				path = filepath.Join(DefaultsPath, "/crio.conf")
				if err := config.UpdateFromFile(path); err != nil {
					if ctx.GlobalIsSet("config") || !os.IsNotExist(err) {
						return err
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
	if ctx.GlobalIsSet("file-locking") {
		config.FileLocking = ctx.GlobalBool("file-locking")
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
		config.HostIP = ctx.GlobalString("host-ip")
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
			if fields[0] == "" {
				return fmt.Errorf("wrong format for --runtimes: %q", r)
			}
			config.Runtimes[fields[0]] = oci.RuntimeHandler{RuntimePath: fields[1]}
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
		config.ImageVolumes = lib.ImageVolumesType(ctx.GlobalString("image-volumes"))
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
	if ctx.GlobalIsSet("additional-devices") {
		config.AdditionalDevices = ctx.GlobalStringSlice("additional-devices")
	}
	if ctx.GlobalIsSet("version-file-location") {
		config.VersionFileLocation = ctx.GlobalString("version-file-location")
	}
	return nil
}

func GetFlagsAndMetadata() ([]cli.Flag, map[string]interface{}, error) {
	config, err := server.DefaultConfig()
	if err != nil {
		return nil, nil, errors.Errorf("error loading server config: %v", err)
	}

	// TODO FIXME should be crio wipe flags
	flags := getCrioFlags(config)

	metadata := map[string]interface{}{
		"config": config,
	}

	return flags, metadata, nil
}

func getCrioFlags(defConf *server.Config) []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:   "config, c",
			Value:  server.CrioConfigPath,
			Usage:  "path to configuration file",
			EnvVar: "CONTAINER_CONFIG",
		},
		cli.StringFlag{
			Name:   "conmon",
			Usage:  fmt.Sprintf("path to the conmon executable (default: %q)", defConf.Conmon),
			EnvVar: "CONTAINER_CONMON",
		},
		cli.StringFlag{
			Name:   "listen",
			Usage:  fmt.Sprintf("path to crio socket (default: %q)", defConf.Listen),
			EnvVar: "CONTAINER_LISTEN",
		},
		cli.StringFlag{
			Name:   "stream-address",
			Usage:  fmt.Sprintf("bind address for streaming socket (default: %q)", defConf.StreamAddress),
			EnvVar: "CONTAINER_STREAM_ADDRESS",
		},
		cli.StringFlag{
			Name:   "stream-port",
			Usage:  fmt.Sprintf("bind port for streaming socket (default: %q)", defConf.StreamPort),
			EnvVar: "CONTAINER_STREAM_PORT",
		},
		cli.StringFlag{
			Name:   "log",
			Value:  "",
			Usage:  "set the log file path where internal debug information is written",
			EnvVar: "CONTAINER_LOG",
		},
		cli.StringFlag{
			Name:   "log-format",
			Value:  "text",
			Usage:  "set the format used by logs ('text' (default), or 'json')",
			EnvVar: "CONTAINER_LOG_FORMAT",
		},
		cli.StringFlag{
			Name:   "log-level, l",
			Value:  "error",
			Usage:  "log messages above specified level: debug, info, warn, error (default), fatal or panic",
			EnvVar: "CONTAINER_LOG_LEVEL",
		},
		cli.StringFlag{
			Name:   "log-dir",
			Value:  "",
			Usage:  "default log directory where all logs will go unless directly specified by the kubelet",
			EnvVar: "CONTAINER_LOG_DIR",
		},
		cli.StringFlag{
			Name:   "pause-command",
			Usage:  fmt.Sprintf("name of the pause command in the pause image (default: %q)", defConf.PauseCommand),
			EnvVar: "CONTAINER_PAUSE_COMMAND",
		},
		cli.StringFlag{
			Name:   "pause-image",
			Usage:  fmt.Sprintf("name of the pause image (default: %q)", defConf.PauseImage),
			EnvVar: "CONTAINER_PAUSE_IMAGE",
		},
		cli.StringFlag{
			Name:   "pause-image-auth-file",
			Usage:  fmt.Sprintf("path to a config file containing credentials for --pause-image (default: %q)", defConf.PauseImageAuthFile),
			EnvVar: "CONTAINER_PAUSE_IMAGE_AUTH_FILE",
		},
		cli.StringFlag{
			Name:   "global-auth-file",
			Usage:  fmt.Sprintf("path to a file like /var/lib/kubelet/config.json holding credentials necessary for pulling images from secure registries (default: %q)", defConf.GlobalAuthFile),
			EnvVar: "CONTAINER_GLOBAL_AUTH_FILE",
		},
		cli.StringFlag{
			Name:   "signature-policy",
			Usage:  fmt.Sprintf("path to signature policy file (default: %q)", defConf.SignaturePolicyPath),
			EnvVar: "CONTAINER_SIGNATURE_POLICY",
		},
		cli.StringFlag{
			Name:   "root, r",
			Usage:  fmt.Sprintf("crio root dir (default: %q)", defConf.Root),
			EnvVar: "CONTAINER_ROOT",
		},
		cli.StringFlag{
			Name:   "runroot",
			Usage:  fmt.Sprintf("crio state dir (default: %q)", defConf.RunRoot),
			EnvVar: "CONTAINER_RUNROOT",
		},
		cli.StringFlag{
			Name:   "storage-driver, s",
			Usage:  fmt.Sprintf("storage driver (default: %q)", defConf.Storage),
			EnvVar: "CONTAINER_STORAGE_DRIVER",
		},
		cli.StringSliceFlag{
			Name:  "storage-opt",
			Usage: fmt.Sprintf("storage driver option (default: %q)", defConf.StorageOptions),
		},
		cli.StringSliceFlag{
			Name:  "insecure-registry",
			Usage: "whether to disable TLS verification for the given registry",
		},
		cli.StringSliceFlag{
			Name:  "registry",
			Usage: fmt.Sprintf("registry to be prepended when pulling unqualified images, can be specified multiple times (default: configured in /etc/containers/registries.conf)"),
		},
		cli.StringFlag{
			Name:   "default-transport",
			Usage:  fmt.Sprintf("default transport (default: %q)", defConf.DefaultTransport),
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
			Usage:  fmt.Sprintf("default OCI runtime from the runtimes config (default: %q)", defConf.DefaultRuntime),
			EnvVar: "CONTAINER_DEFAULT_RUNTIME",
		},
		cli.StringSliceFlag{
			Name:  "runtimes",
			Usage: "OCI runtimes, format is runtime_name:runtime_path:runtime_root",
		},
		cli.StringFlag{
			Name:   "seccomp-profile",
			Usage:  fmt.Sprintf("default seccomp profile path (default: %q)", defConf.SeccompProfile),
			EnvVar: "CONTAINER_SECCOMP_PROFILE",
		},
		cli.StringFlag{
			Name:   "apparmor-profile",
			Usage:  fmt.Sprintf("default apparmor profile name (default: %q)", defConf.ApparmorProfile),
			EnvVar: "CONTAINER_APPARMOR_PROFILE",
		},
		cli.BoolFlag{
			Name:   "selinux",
			Usage:  fmt.Sprintf("enable selinux support (default: %t)", defConf.SELinux),
			EnvVar: "CONTAINER_SELINUX",
		},
		cli.StringFlag{
			Name:   "cgroup-manager",
			Usage:  fmt.Sprintf("cgroup manager (cgroupfs or systemd) (default: %q)", defConf.CgroupManager),
			EnvVar: "CONTAINER_CGROUP_MANAGER",
		},
		cli.Int64Flag{
			Name:   "pids-limit",
			Value:  lib.DefaultPidsLimit,
			Usage:  "maximum number of processes allowed in a container",
			EnvVar: "CONTAINER_PIDS_LIMIT",
		},
		cli.Int64Flag{
			Name:   "log-size-max",
			Value:  lib.DefaultLogSizeMax,
			Usage:  "maximum log size in bytes for a container",
			EnvVar: "CONTAINER_LOG_SIZE_MAX",
		},
		cli.BoolFlag{
			Name:   "log-journald",
			Usage:  fmt.Sprintf("Log to journald in addition to kubernetes log file (default: %t)", defConf.LogToJournald),
			EnvVar: "CONTAINER_LOG_JOURNALD",
		},
		cli.StringFlag{
			Name:   "cni-config-dir",
			Usage:  fmt.Sprintf("CNI configuration files directory (default: %q)", defConf.NetworkDir),
			EnvVar: "CONTAINER_CNI_CONFIG_DIR",
		},
		cli.StringSliceFlag{
			Name:  "cni-plugin-dir",
			Usage: fmt.Sprintf("CNI plugin binaries directory (default: %q)", defConf.PluginDir),
		},
		cli.StringFlag{
			Name:   "image-volumes",
			Value:  string(lib.ImageVolumesMkdir),
			Usage:  "image volume handling ('mkdir', 'bind', or 'ignore')",
			EnvVar: "CONTAINER_IMAGE_VOLUMES",
		},
		cli.StringSliceFlag{
			Name:  "hooks-dir",
			Usage: fmt.Sprintf("set the OCI hooks directory path (may be set multiple times) (default: %q)", defConf.HooksDir),
		},
		cli.StringSliceFlag{
			Name:  "default-mounts",
			Usage: fmt.Sprintf("add one or more default mount paths in the form host:container (deprecated) (default: %q)", defConf.DefaultMounts),
		},
		cli.StringFlag{
			Name:   "default-mounts-file",
			Usage:  fmt.Sprintf("path to default mounts file (default: %q)", defConf.DefaultMountsFile),
			EnvVar: "CONTAINER_DEFAULT_MOUNTS_FILE",
		},
		cli.StringFlag{
			Name:   "default-capabilities",
			Usage:  fmt.Sprintf("capabilities to add to the containers (default: %q)", defConf.DefaultCapabilities),
			EnvVar: "CONTAINER_DEFAULT_CAPABILITIES",
		},
		cli.StringSliceFlag{
			Name:  "default-sysctls",
			Usage: fmt.Sprintf("sysctls to add to the containers (default: %q)", defConf.DefaultSysctls),
		},
		cli.StringSliceFlag{
			Name:  "default-ulimits",
			Usage: fmt.Sprintf("ulimits to apply to containers by default (name=soft:hard) (default: %q)", defConf.DefaultUlimits),
		},
		cli.BoolFlag{
			Name:   "profile",
			Usage:  "enable pprof remote profiler on localhost:6060",
			EnvVar: "CONTAINER_PROFILE",
		},
		cli.IntFlag{
			Name:   "profile-port",
			Value:  6060,
			Usage:  "port for the pprof profiler",
			EnvVar: "CONTAINER_PROFILE_PORT",
		},
		cli.BoolFlag{
			Name:   "enable-metrics",
			Usage:  "enable metrics endpoint for the server on localhost:9090",
			EnvVar: "CONTAINER_ENABLE_METRICS",
		},
		cli.IntFlag{
			Name:   "metrics-port",
			Value:  9090,
			Usage:  "port for the metrics endpoint",
			EnvVar: "CONTAINER_METRICS_PORT",
		},
		cli.BoolFlag{
			Name:   "read-only",
			Usage:  fmt.Sprintf("setup all unprivileged containers to run as read-only (default: %t)", defConf.ReadOnly),
			EnvVar: "CONTAINER_READ_ONLY",
		},
		cli.StringFlag{
			Name:   "bind-mount-prefix",
			Usage:  fmt.Sprintf("specify a prefix to prepend to the source of a bind mount (default: %q)", defConf.BindMountPrefix),
			EnvVar: "CONTAINER_BIND_MOUNT_PREFIX",
		},
		cli.StringFlag{
			Name:   "uid-mappings",
			Usage:  fmt.Sprintf("specify the UID mappings to use for the user namespace (default: %q)", defConf.UIDMappings),
			Value:  "",
			EnvVar: "CONTAINER_UID_MAPPINGS",
		},
		cli.StringFlag{
			Name:   "gid-mappings",
			Usage:  fmt.Sprintf("specify the GID mappings to use for the user namespace (default: %q)", defConf.GIDMappings),
			Value:  "",
			EnvVar: "CONTAINER_GID_MAPPINGS",
		},
		cli.StringSliceFlag{
			Name:  "additional-devices",
			Usage: fmt.Sprintf("devices to add to the containers (default: %q)", defConf.AdditionalDevices),
		},
		cli.StringSliceFlag{
			Name:  "conmon-env",
			Usage: fmt.Sprintf("environment variable list for the conmon process, used for passing necessary environment variables to conmon or the runtime (default: %q)", defConf.ConmonEnv),
		},
		cli.StringFlag{
			Name:  "container-attach-socket-dir",
			Usage: fmt.Sprintf("path to directory for container attach sockets (default: %q)", defConf.ContainerAttachSocketDir),
		},
		cli.StringFlag{
			Name:  "container-exits-dir",
			Usage: fmt.Sprintf("path to directory in which container exit files are written to by conmon (default: %q)", defConf.ContainerExitsDir),
		},
		cli.Int64Flag{
			Name:  "ctr-stop-timeout",
			Usage: fmt.Sprintf("the minimal amount of time in seconds to wait before issuing a timeout regarding the proper termination of the container (default: %q)", defConf.CtrStopTimeout),
		},
		cli.IntFlag{
			Name:  "grpc-max-recv-msg-size",
			Usage: fmt.Sprintf("maximum grpc receive message size in bytes (default: %q)", defConf.GRPCMaxRecvMsgSize),
		},
		cli.IntFlag{
			Name:  "grpc-max-send-msg-size",
			Usage: fmt.Sprintf("maximum grpc receive message size (default: %q)", defConf.GRPCMaxSendMsgSize),
		},
		cli.StringFlag{
			Name:  "host-ip",
			Usage: fmt.Sprintf("host IP considered as the primary IP to use by CRI-O for things such as host network IP (default: %q)", defConf.HostIP),
		},
		cli.BoolFlag{
			Name:  "manage-network-ns-lifecycle",
			Usage: fmt.Sprintf("determines whether we pin and remove network namespace and manage its lifecycle (default: %v)", defConf.ManageNetworkNSLifecycle),
		},
		cli.BoolFlag{
			Name:  "no-pivot",
			Usage: fmt.Sprintf("if true, the runtime will not use `pivot_root`, but instead use `MS_MOVE` (default: %v)", defConf.NoPivot),
		},
		cli.BoolFlag{
			Name:  "stream-enable-tls",
			Usage: fmt.Sprintf("enable encrypted TLS transport of the stream server (default: %v)", defConf.StreamEnableTLS),
		},
		cli.StringFlag{
			Name:  "stream-tls-ca",
			Usage: fmt.Sprintf("path to the x509 CA(s) file used to verify and authenticate client communication with the encrypted stream. This file can change and CRI-O will automatically pick up the changes within 5 minutes (default: %q)", defConf.StreamTLSCA),
		},
		cli.StringFlag{
			Name:  "stream-tls-cert",
			Usage: fmt.Sprintf("path to the x509 certificate file used to serve the encrypted stream. This file can change and CRI-O will automatically pick up the changes within 5 minutes (default: %q)", defConf.StreamTLSCert),
		},
		cli.StringFlag{
			Name:  "stream-tls-key",
			Usage: fmt.Sprintf("path to the key file used to serve the encrypted stream. This file can change and CRI-O will automatically pick up the changes within 5 minutes (default: %q)", defConf.StreamTLSKey),
		},
		cli.StringFlag{
			Name:  "version-file-location",
			Usage: fmt.Sprintf("path to where CRI-O should put the version file, as well as where crio-wipe will check for the version file. (default: %s)", defConf.VersionFileLocation),
		},
	}
}
