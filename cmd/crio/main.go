package main

import (
	"context"
	goflag "flag"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/containers/image/types"
	_ "github.com/containers/libpod/pkg/hooks/0.1.0"
	"github.com/containers/storage/pkg/reexec"
	libconfig "github.com/cri-o/cri-o/lib/config"
	"github.com/cri-o/cri-o/pkg/signals"
	"github.com/cri-o/cri-o/server"
	"github.com/cri-o/cri-o/utils"
	"github.com/cri-o/cri-o/version"
	"github.com/sirupsen/logrus"
	"github.com/soheilhy/cmux"
	"github.com/urfave/cli"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// gitCommit is the commit that the binary is being built from.
// It will be populated by the Makefile.
var gitCommit = ""

// DefaultsPath is the path to default configuration files set at build time
var DefaultsPath string

func mergeConfig(config *server.Config, ctx *cli.Context) (string, error) {
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
	if ctx.GlobalIsSet("log-dir") {
		config.LogDir = ctx.GlobalString("log-dir")
	}
	if ctx.GlobalIsSet("additional-devices") {
		config.AdditionalDevices = ctx.GlobalStringSlice("additional-devices")
	}
	return path, nil
}

func writeCrioGoroutineStacks() {
	path := filepath.Join("/tmp", fmt.Sprintf("crio-goroutine-stacks-%s.log", strings.ReplaceAll(time.Now().Format(time.RFC3339), ":", "")))
	if err := utils.WriteGoroutineStacksToFile(path); err != nil {
		logrus.Warnf("Failed to write goroutine stacks: %s", err)
	}
}

func catchShutdown(ctx context.Context, cancel context.CancelFunc, gserver *grpc.Server, sserver *server.Server, hserver *http.Server, signalled *bool) {

	sig := make(chan os.Signal, 2048)
	signal.Notify(sig, signals.Interrupt, signals.Term, unix.SIGUSR1, unix.SIGPIPE, signals.Hup)
	go func() {
		for s := range sig {
			logrus.WithFields(logrus.Fields{
				"signal": s,
			}).Debug("received signal")
			switch s {
			case unix.SIGUSR1:
				writeCrioGoroutineStacks()
				continue
			case unix.SIGPIPE:
				continue
			case signals.Interrupt:
				logrus.Debugf("Caught SIGINT")
			case signals.Term:
				logrus.Debugf("Caught SIGTERM")
			default:
				continue
			}
			*signalled = true
			gserver.GracefulStop()
			hserver.Shutdown(ctx) // nolint: errcheck
			if err := sserver.StopStreamServer(); err != nil {
				logrus.Warnf("error shutting down streaming server: %v", err)
			}
			sserver.StopMonitors()
			cancel()
			if err := sserver.Shutdown(ctx); err != nil {
				logrus.Warnf("error shutting down main service %v", err)
			}
			return
		}
	}()
}

func main() {
	// https://github.com/kubernetes/kubernetes/issues/17162
	if err := goflag.CommandLine.Parse([]string{}); err != nil {
		fmt.Fprintf(os.Stderr, "unable to parse command line flags\n")
		os.Exit(-1)
	}

	if reexec.Init() {
		fmt.Fprintf(os.Stderr, "unable to initialize container storage\n")
		os.Exit(-1)
	}
	app := cli.NewApp()

	var v []string
	v = append(v, version.Version)
	if gitCommit != "" {
		v = append(v, fmt.Sprintf("commit: %s", gitCommit))
	}
	app.Name = "crio"
	app.Usage = "crio server"
	app.Version = strings.Join(v, "\n")

	systemContext := &types.SystemContext{}
	defConf, err := server.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading server config: %v", err)
		os.Exit(1)
	}
	app.Metadata = map[string]interface{}{
		"config": defConf,
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config",
			Value: server.CrioConfigPath,
			Usage: "path to configuration file",
		},
		cli.StringFlag{
			Name:  "conmon",
			Usage: fmt.Sprintf("path to the conmon executable (default: %q)", defConf.Conmon),
		},
		cli.StringFlag{
			Name:  "conmon-cgroup",
			Usage: fmt.Sprintf("cgroup used for conmon process (default: %q)", defConf.ConmonCgroup),
		},
		cli.StringFlag{
			Name:  "listen",
			Usage: fmt.Sprintf("path to crio socket (default: %q)", defConf.Listen),
		},
		cli.StringFlag{
			Name:  "stream-address",
			Usage: fmt.Sprintf("bind address for streaming socket (default: %q)", defConf.StreamAddress),
		},
		cli.StringFlag{
			Name:  "stream-port",
			Usage: fmt.Sprintf("bind port for streaming socket (default: %q)", defConf.StreamPort),
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
			Name:  "log-dir",
			Value: "",
			Usage: "default log directory where all logs will go unless directly specified by the kubelet",
		},
		cli.StringFlag{
			Name:  "pause-command",
			Usage: fmt.Sprintf("name of the pause command in the pause image (default: %q)", defConf.PauseCommand),
		},
		cli.StringFlag{
			Name:  "pause-image",
			Usage: fmt.Sprintf("name of the pause image (default: %q)", defConf.PauseImage),
		},
		cli.StringFlag{
			Name:  "pause-image-auth-file",
			Usage: fmt.Sprintf("path to a config file containing credentials for --pause-image (default: %s)", defConf.PauseImageAuthFile),
		},
		cli.StringFlag{
			Name:  "signature-policy",
			Usage: fmt.Sprintf("path to signature policy file (default: %q)", defConf.SignaturePolicyPath),
		},
		cli.StringFlag{
			Name:  "root",
			Usage: fmt.Sprintf("crio root dir (default: %q)", defConf.Root),
		},
		cli.StringFlag{
			Name:  "runroot",
			Usage: fmt.Sprintf("crio state dir (default: %q)", defConf.RunRoot),
		},
		cli.StringFlag{
			Name:  "storage-driver",
			Usage: fmt.Sprintf("storage driver (default: %q)", defConf.Storage),
		},
		cli.StringSliceFlag{
			Name:  "storage-opt",
			Usage: fmt.Sprintf("storage driver option (default: %q)", defConf.StorageOptions),
		},
		// XXX: DEPRECATED
		cli.BoolFlag{
			Name:   "file-locking",
			Usage:  fmt.Sprintf("enable or disable file-based locking (depreciated) (default: %t)", defConf.FileLocking),
			Hidden: true,
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
			Name:  "default-transport",
			Usage: fmt.Sprintf("default transport (default: %q)", defConf.DefaultTransport),
		},
		// XXX: DEPRECATED
		cli.StringFlag{
			Name:   "runtime",
			Usage:  "OCI runtime path",
			Hidden: true,
		},
		cli.StringFlag{
			Name:  "default-runtime",
			Usage: fmt.Sprintf("default OCI runtime from the runtimes config (default: %q)", defConf.DefaultRuntime),
		},
		cli.StringSliceFlag{
			Name:  "runtimes",
			Usage: "OCI runtimes, format is runtime_name:runtime_path:runtime_root",
		},
		cli.StringFlag{
			Name:  "seccomp-profile",
			Usage: fmt.Sprintf("default seccomp profile path (default: %q)", defConf.SeccompProfile),
		},
		cli.StringFlag{
			Name:  "apparmor-profile",
			Usage: fmt.Sprintf("default apparmor profile name (default: %q)", defConf.ApparmorProfile),
		},
		cli.BoolFlag{
			Name:  "selinux",
			Usage: fmt.Sprintf("enable selinux support (default: %t)", defConf.SELinux),
		},
		cli.StringFlag{
			Name:  "cgroup-manager",
			Usage: fmt.Sprintf("cgroup manager (cgroupfs or systemd) (default: %q)", defConf.CgroupManager),
		},
		cli.Int64Flag{
			Name:  "pids-limit",
			Value: libconfig.DefaultPidsLimit,
			Usage: "maximum number of processes allowed in a container",
		},
		cli.Int64Flag{
			Name:  "log-size-max",
			Value: libconfig.DefaultLogSizeMax,
			Usage: "maximum log size in bytes for a container",
		},
		cli.BoolFlag{
			Name:  "log-journald",
			Usage: fmt.Sprintf("Log to journald in addition to kubernetes log file (default: %t)", defConf.LogToJournald),
		},
		cli.StringFlag{
			Name:  "cni-config-dir",
			Usage: fmt.Sprintf("CNI configuration files directory (default: %q)", defConf.NetworkDir),
		},
		cli.StringSliceFlag{
			Name:  "cni-plugin-dir",
			Usage: fmt.Sprintf("CNI plugin binaries directory (default: %q)", defConf.PluginDir),
		},
		cli.StringFlag{
			Name:  "image-volumes",
			Value: string(libconfig.ImageVolumesMkdir),
			Usage: "image volume handling ('mkdir', 'bind', or 'ignore')",
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
			Hidden: true,
		},
		cli.StringFlag{
			Name:  "default-capabilities",
			Usage: fmt.Sprintf("capabilities to add to the containers (default: %q)", defConf.DefaultCapabilities),
		},
		cli.StringSliceFlag{
			Name:  "default-sysctls",
			Usage: fmt.Sprintf("sysctls to add to the containers (default: %q)", defConf.DefaultSysctls),
		},
		cli.StringSliceFlag{
			Name:  "default-ulimits",
			Usage: fmt.Sprintf("ulimits to apply to conatainers by default (name=soft:hard) (default: %q)", defConf.DefaultUlimits),
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
			Usage: fmt.Sprintf("setup all unprivileged containers to run as read-only (default: %t)", defConf.ReadOnly),
		},
		cli.StringFlag{
			Name:  "bind-mount-prefix",
			Usage: fmt.Sprintf("specify a prefix to prepend to the source of a bind mount (default: %q)", defConf.BindMountPrefix),
		},
		cli.StringFlag{
			Name:  "uid-mappings",
			Usage: fmt.Sprintf("specify the UID mappings to use for the user namespace (default: %q)", defConf.UIDMappings),
			Value: "",
		},
		cli.StringFlag{
			Name:  "gid-mappings",
			Usage: fmt.Sprintf("specify the GID mappings to use for the user namespace (default: %q)", defConf.GIDMappings),
			Value: "",
		},
		cli.StringSliceFlag{
			Name:  "additional-devices",
			Usage: fmt.Sprintf("devices to add to the containers (default: %q)", defConf.AdditionalDevices),
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.FlagsByName(configCommand.Flags))

	app.Commands = []cli.Command{
		configCommand,
	}

	var configPath string
	app.Before = func(c *cli.Context) (err error) {
		// Load the configuration file.
		config, ok := c.App.Metadata["config"].(*server.Config)
		if !ok {
			return fmt.Errorf("type assertion error when accessing server config")
		}
		configPath, err = mergeConfig(config, c)
		if err != nil {
			return err
		}

		cf := &logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000000000Z07:00",
			FullTimestamp:   true,
		}

		logrus.SetFormatter(cf)

		level, err := logrus.ParseLevel(config.LogLevel)
		if err != nil {
			return err
		}
		logrus.SetLevel(level)

		if path := c.GlobalString("log"); path != "" {
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_SYNC, 0666)
			if err != nil {
				return err
			}
			logrus.SetOutput(f)
		}

		switch c.GlobalString("log-format") {
		case "text":
			// retain logrus's default.
		case "json":
			logrus.SetFormatter(new(logrus.JSONFormatter))
		default:
			return fmt.Errorf("unknown log-format %q", c.GlobalString("log-format"))
		}

		return nil
	}

	app.Action = func(c *cli.Context) error {
		ctx, cancel := context.WithCancel(context.Background())
		if c.GlobalBool("profile") {
			profilePort := c.GlobalInt("profile-port")
			profileEndpoint := fmt.Sprintf("localhost:%v", profilePort)
			go func() {
				logrus.Debugf("starting profiling server on %v", profileEndpoint)
				if err := http.ListenAndServe(profileEndpoint, nil); err != nil {
					logrus.Fatalf("unable to run profiling server: %v", err)
				}
			}()
		}

		args := c.Args()
		if len(args) > 0 {
			for i := range app.Commands {
				command := &app.Commands[i]
				if args[0] == command.Name {
					break
				}
			}
			cancel()
			return fmt.Errorf("command %q not supported", args[0])
		}

		config, ok := c.App.Metadata["config"].(*server.Config)
		if !ok {
			cancel()
			return fmt.Errorf("type assertion error when accessing server config")
		}

		// Validate the configuration during runtime
		if err := config.Validate(systemContext, true); err != nil {
			cancel()
			return err
		}

		if config.GRPCMaxSendMsgSize <= 0 {
			config.GRPCMaxSendMsgSize = server.DefaultGRPCMaxMsgSize
		}
		if config.GRPCMaxRecvMsgSize <= 0 {
			config.GRPCMaxRecvMsgSize = server.DefaultGRPCMaxMsgSize
		}

		if !config.SELinux {
			disableSELinux()
		}

		if err := os.MkdirAll(filepath.Dir(config.Listen), 0755); err != nil {
			cancel()
			return err
		}

		// Remove the socket if it already exists
		if _, err := os.Stat(config.Listen); err == nil {
			if err := os.Remove(config.Listen); err != nil {
				logrus.Fatal(err)
			}
		}
		lis, err := server.Listen("unix", config.Listen)
		if err != nil {
			logrus.Fatalf("failed to listen: %v", err)
		}

		grpcServer := grpc.NewServer(
			grpc.MaxSendMsgSize(config.GRPCMaxSendMsgSize),
			grpc.MaxRecvMsgSize(config.GRPCMaxRecvMsgSize),
		)

		service, err := server.New(ctx, systemContext, configPath, config)
		if err != nil {
			logrus.Fatal(err)
		}

		// Immediately upon start up, write our new version file
		if err := version.WriteVersionFile(server.CrioVersionPath, gitCommit); err != nil {
			logrus.Fatal(err)
		}

		if c.GlobalBool("enable-metrics") {
			metricsPort := c.GlobalInt("metrics-port")
			me, err := service.CreateMetricsEndpoint()
			if err != nil {
				logrus.Fatalf("Failed to create metrics endpoint: %v", err)
			}
			l, err := net.Listen("tcp", fmt.Sprintf(":%v", metricsPort))
			if err != nil {
				logrus.Fatalf("Failed to create listener for metrics: %v", err)
			}
			go func() {
				if err := http.Serve(l, me); err != nil {
					logrus.Fatalf("Failed to serve metrics endpoint: %v", err)
				}
			}()
		}

		runtime.RegisterRuntimeServiceServer(grpcServer, service)
		runtime.RegisterImageServiceServer(grpcServer, service)

		// after the daemon is done setting up we can notify systemd api
		notifySystem()

		go func() {
			service.StartExitMonitor()
		}()
		hookSync := make(chan error, 2)
		if service.ContainerServer.Hooks == nil {
			hookSync <- err // so we don't block during cleanup
		} else {
			go service.ContainerServer.Hooks.Monitor(ctx, hookSync)
			err = <-hookSync
			if err != nil {
				cancel()
				logrus.Fatal(err)
			}
		}

		m := cmux.New(lis)
		grpcL := m.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
		httpL := m.Match(cmux.HTTP1Fast())

		infoMux := service.GetInfoMux()
		httpServer := &http.Server{
			Handler:     infoMux,
			ReadTimeout: 5 * time.Second,
		}

		graceful := false
		catchShutdown(ctx, cancel, grpcServer, service, httpServer, &graceful)

		go func() {
			if err := grpcServer.Serve(grpcL); err != nil {
				logrus.Errorf("unable to run GRPC server: %v", err)
			}
		}()
		go func() {
			if err := httpServer.Serve(httpL); err != nil {
				logrus.Debugf("closed http server")
			}
		}()

		serverCloseCh := make(chan struct{})
		go func() {
			defer close(serverCloseCh)
			if err := m.Serve(); err != nil {
				if graceful && strings.Contains(strings.ToLower(err.Error()), "use of closed network connection") {
					err = nil
				} else {
					logrus.Errorf("Failed to serve grpc request: %v", err)
				}
			}
		}()

		streamServerCloseCh := service.StreamingServerCloseChan()
		serverMonitorsCh := service.MonitorsCloseChan()
		select {
		case <-streamServerCloseCh:
		case <-serverMonitorsCh:
		case <-serverCloseCh:
		}

		if err := service.Shutdown(ctx); err != nil {
			logrus.Warnf("error shutting down service: %v", err)
		}
		cancel()

		<-streamServerCloseCh
		logrus.Debug("closed stream server")
		<-serverMonitorsCh
		logrus.Debug("closed monitors")
		err = <-hookSync
		if err == nil || err == context.Canceled {
			logrus.Debug("closed hook monitor")
		} else {
			logrus.Errorf("hook monitor failed: %v", err)
		}
		<-serverCloseCh
		logrus.Debug("closed main server")

		return nil
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
