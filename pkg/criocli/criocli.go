package criocli

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
	if ctx.GlobalIsSet("version-file") {
		config.VersionFile = ctx.GlobalString("version-file")
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
			Name:  "config",
			Value: server.CrioConfigPath,
			Usage: "path to configuration file",
		},
		cli.StringFlag{
			Name:  "conmon",
			Usage: "path to the conmon executable",
		},
		cli.StringFlag{
			Name:  "conmon-cgroup",
			Usage: fmt.Sprintf("cgroup used for conmon process (default: %q)", defConf.ConmonCgroup),
		},
		cli.StringFlag{
			Name:  "listen",
			Usage: "path to crio socket",
		},
		cli.StringFlag{
			Name:  "stream-address",
			Usage: "bind address for streaming socket",
		},
		cli.StringFlag{
			Name:  "stream-port",
			Usage: "bind port for streaming socket (default: \"0\")",
		},
		cli.StringFlag{
			Name:  "log",
			Value: "",
			Usage: "set the log file path where internal debug information is written",
		},
		cli.StringFlag{
			Name:  "log-format",
			Value: "text",
			Usage: "set the format used by logs ('text' (default), or 'json')",
		},
		cli.StringFlag{
			Name:  "log-level",
			Value: "error",
			Usage: "log messages above specified level: debug, info, warn, error (default), fatal or panic",
		},
		cli.StringFlag{
			Name:  "pause-command",
			Usage: "name of the pause command in the pause image",
		},
		cli.StringFlag{
			Name:  "pause-image",
			Usage: "name of the pause image",
		},
		cli.StringFlag{
			Name:  "pause-image-auth-file",
			Usage: "path to a config file containing credentials for --pause-image",
		},
		cli.StringFlag{
			Name:  "signature-policy",
			Usage: "path to signature policy file",
		},
		cli.StringFlag{
			Name:  "root",
			Usage: "crio root dir",
		},
		cli.StringFlag{
			Name:  "runroot",
			Usage: "crio state dir",
		},
		cli.StringFlag{
			Name:  "storage-driver",
			Usage: "storage driver",
		},
		cli.StringSliceFlag{
			Name:  "storage-opt",
			Usage: "storage driver option",
		},
		cli.BoolFlag{
			Name:  "file-locking",
			Usage: "enable or disable file-based locking",
		},
		cli.StringSliceFlag{
			Name:  "insecure-registry",
			Usage: "whether to disable TLS verification for the given registry",
		},
		cli.StringSliceFlag{
			Name:  "registry",
			Usage: "registry to be prepended when pulling unqualified images, can be specified multiple times",
		},
		cli.StringFlag{
			Name:  "default-transport",
			Usage: "default transport",
		},
		// XXX: DEPRECATED
		cli.StringFlag{
			Name:  "runtime",
			Usage: "OCI runtime path",
		},
		cli.StringFlag{
			Name:  "default-runtime",
			Usage: "default OCI runtime from the runtimes config",
		},
		cli.StringSliceFlag{
			Name:  "runtimes",
			Usage: "OCI runtimes, format is runtime_name:runtime_path",
		},
		cli.StringFlag{
			Name:  "seccomp-profile",
			Usage: "default seccomp profile path",
		},
		cli.StringFlag{
			Name:  "apparmor-profile",
			Usage: "default apparmor profile name (default: \"crio-default\")",
		},
		cli.BoolFlag{
			Name:  "selinux",
			Usage: "enable selinux support",
		},
		cli.StringFlag{
			Name:  "cgroup-manager",
			Usage: "cgroup manager (cgroupfs or systemd)",
		},
		cli.Int64Flag{
			Name:  "pids-limit",
			Value: lib.DefaultPidsLimit,
			Usage: "maximum number of processes allowed in a container",
		},
		cli.Int64Flag{
			Name:  "log-size-max",
			Value: lib.DefaultLogSizeMax,
			Usage: "maximum log size in bytes for a container",
		},
		cli.BoolFlag{
			Name:  "log-journald",
			Usage: "Log to journald in addition to kubernetes log file",
		},
		cli.StringFlag{
			Name:  "cni-config-dir",
			Usage: "CNI configuration files directory",
		},
		cli.StringSliceFlag{
			Name:  "cni-plugin-dir",
			Usage: "CNI plugin binaries directory",
		},
		cli.StringFlag{
			Name:  "image-volumes",
			Value: string(lib.ImageVolumesMkdir),
			Usage: "image volume handling ('mkdir', 'bind', or 'ignore')",
		},
		cli.StringSliceFlag{
			Name:  "hooks-dir",
			Usage: "set the OCI hooks directory path (may be set multiple times)",
		},
		cli.StringSliceFlag{
			Name:  "default-mounts",
			Usage: "add one or more default mount paths in the form host:container (deprecated)",
		},
		cli.StringFlag{
			Name:   "default-mounts-file",
			Usage:  "path to default mounts file",
			Hidden: true,
		},
		cli.StringFlag{
			Name:  "default-capabilities",
			Usage: "capabilities to add to the containers",
		},
		cli.StringFlag{
			Name:  "default-sysctls",
			Usage: "sysctls to add to the containers",
		},
		cli.StringSliceFlag{
			Name:  "default-ulimits",
			Usage: "ulimits to apply to conatainers by default (name=soft:hard)",
		},
		cli.BoolFlag{
			Name:  "profile",
			Usage: "enable pprof remote profiler on localhost:6060",
		},
		cli.IntFlag{
			Name:  "profile-port",
			Value: 6060,
			Usage: "port for the pprof profiler",
		},
		cli.BoolFlag{
			Name:  "enable-metrics",
			Usage: "enable metrics endpoint for the server on localhost:9090",
		},
		cli.IntFlag{
			Name:  "metrics-port",
			Value: 9090,
			Usage: "port for the metrics endpoint",
		},
		cli.BoolFlag{
			Name:  "read-only",
			Usage: "setup all unprivileged containers to run as read-only",
		},
		cli.StringFlag{
			Name:  "bind-mount-prefix",
			Usage: "specify a prefix to prepend to the source of a bind mount",
		},
		cli.StringFlag{
			Name:  "uid-mappings",
			Usage: "specify the UID mappings to use for the user namespace",
			Value: "",
		},
		cli.StringFlag{
			Name:  "gid-mappings",
			Usage: "specify the GID mappings to use for the user namespace",
			Value: "",
		},
		cli.StringSliceFlag{
			Name:  "additional-devices",
			Usage: "devices to add to the containers",
		},
		cli.StringFlag{
			Name:  "version-file",
			Usage: fmt.Sprintf("path to where CRI-O should put the version file. (default: %s)", defConf.VersionFile),
		},
	}
}
