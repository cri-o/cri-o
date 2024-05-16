package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/containers/kubensmnt"
	"github.com/containers/storage/pkg/reexec"
	"github.com/cri-o/cri-o/internal/criocli"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/opentelemetry"
	"github.com/cri-o/cri-o/internal/signals"
	"github.com/cri-o/cri-o/internal/version"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/server"
	otel_collector "github.com/cri-o/cri-o/server/otel-collector"
	"github.com/cri-o/cri-o/utils"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/sirupsen/logrus"
	"github.com/soheilhy/cmux"
	"github.com/uptrace/opentelemetry-go-extra/otellogrus"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func writeCrioGoroutineStacks() {
	path := filepath.Join("/tmp", fmt.Sprintf(
		"crio-goroutine-stacks-%s.log",
		strings.ReplaceAll(time.Now().Format(time.RFC3339), ":", ""),
	))
	if err := utils.WriteGoroutineStacksToFile(path); err != nil {
		logrus.Warnf("Failed to write goroutine stacks: %s", err)
	}
}

func catchShutdown(ctx context.Context, cancel context.CancelFunc, gserver *grpc.Server, tp *sdktrace.TracerProvider, sserver *server.Server, hserver *http.Server, signalled *bool) {
	sig := make(chan os.Signal, 2048)
	signal.Notify(sig, signals.Interrupt, signals.Term, unix.SIGUSR1, unix.SIGUSR2, unix.SIGPIPE, signals.Hup)
	go func() {
		for s := range sig {
			log.WithFields(ctx, logrus.Fields{
				"signal": s,
			}).Debug("received signal")
			switch s {
			case unix.SIGUSR1:
				writeCrioGoroutineStacks()
				continue
			case unix.SIGUSR2:
				runtime.GC()
				continue
			case unix.SIGPIPE:
				continue
			case signals.Interrupt:
				log.Debugf(ctx, "Caught SIGINT")
			case signals.Term:
				log.Debugf(ctx, "Caught SIGTERM")
			default:
				continue
			}
			*signalled = true
			if tp != nil {
				if err := tp.Shutdown(ctx); err != nil {
					log.Warnf(ctx, "Error shutting down opentelemetry tracer provider: %v", err)
				}
			}
			gserver.GracefulStop()
			hserver.Shutdown(ctx) // nolint: errcheck
			if err := sserver.StopStreamServer(); err != nil {
				log.Warnf(ctx, "Error shutting down streaming server: %v", err)
			}
			sserver.StopMonitors()
			cancel()
			if err := sserver.Shutdown(ctx); err != nil {
				log.Warnf(ctx, "Error shutting down main service %v", err)
			}
			return
		}
	}()
}

const usage = `OCI-based implementation of Kubernetes Container Runtime Interface Daemon

crio is meant to provide an integration path between OCI conformant runtimes
and the kubelet. Specifically, it implements the Kubelet Container Runtime
Interface (CRI) using OCI conformant runtimes. The scope of crio is tied to the
scope of the CRI.

1. Support multiple image formats including the existing Docker and OCI image formats.
2. Support for multiple means to download images including trust & image verification.
3. Container image management (managing image layers, overlay filesystems, etc).
4. Container process lifecycle management.
5. Monitoring and logging required to satisfy the CRI.
6. Resource isolation as required by the CRI.`

const kubensmntHelp = `Path to a bind-mounted mount namespace that CRI-O
should join before launching any containers. If the path does not exist,
or does not point to a mount namespace bindmount, CRI-O will run in its
parent's mount namespace and log a warning that the requested namespace
was not joined.`

func main() {
	log.InitKlogShim()

	if reexec.Init() {
		return
	}
	app := cli.NewApp()

	app.Name = "crio"
	app.Usage = "OCI-based implementation of Kubernetes Container Runtime Interface"
	app.Authors = []*cli.Author{{Name: "The CRI-O Maintainers"}}
	app.UsageText = usage
	app.Description = app.Usage

	info, err := version.Get(false)
	if err != nil {
		logrus.Fatal(err)
	}

	app.Version = version.Version + "\n" + info.String()

	app.Flags, app.Metadata, err = criocli.GetFlagsAndMetadata()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.FlagsByName(criocli.ConfigCommand.Flags))

	app.Metadata["Env"] = map[string]string{
		kubensmnt.EnvName: kubensmntHelp,
	}

	app.Commands = criocli.DefaultCommands
	app.Commands = append(app.Commands, []*cli.Command{
		criocli.ConfigCommand,
		criocli.PublishCommand,
		criocli.VersionCommand,
		criocli.WipeCommand,
		criocli.StatusCommand,
	}...)

	app.Before = func(c *cli.Context) (err error) {
		config, err := criocli.GetAndMergeConfigFromContext(c)
		if err != nil {
			return err
		}

		logrus.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000000000Z07:00",
			FullTimestamp:   true,
		})
		info.LogVersion()

		level, err := logrus.ParseLevel(config.LogLevel)
		if err != nil {
			return err
		}
		logrus.SetLevel(level)
		logrus.AddHook(log.NewFilenameHook())
		logrus.AddHook(otellogrus.NewHook(otellogrus.WithLevels(
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
			logrus.WarnLevel,
			logrus.InfoLevel,
			logrus.DebugLevel,
			logrus.TraceLevel,
		)))

		filterHook, err := log.NewFilterHook(config.LogFilter)
		if err != nil {
			return err
		}
		logrus.AddHook(filterHook)

		if path := c.String("log"); path != "" {
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_SYNC, 0o666)
			if err != nil {
				return err
			}
			logrus.SetOutput(f)
		}

		switch c.String("log-format") {
		case "text":
			// retain logrus's default.
		case "json":
			logrus.SetFormatter(new(logrus.JSONFormatter))
		default:
			return fmt.Errorf("unknown log-format %q", c.String("log-format"))
		}

		return nil
	}

	app.Action = func(c *cli.Context) error {
		ctx, cancel := context.WithCancel(context.Background())

		cpuProfilePath := c.String("profile-cpu")
		if cpuProfilePath != "" {
			logrus.Infof("Creating CPU profile in: %v", cpuProfilePath)

			file, err := os.Create(cpuProfilePath)
			if err != nil {
				cancel()
				return fmt.Errorf("could not create CPU profile: %w", err)
			}
			defer file.Close()

			if err := pprof.StartCPUProfile(file); err != nil {
				cancel()
				return fmt.Errorf("could not start CPU profiling: %w", err)
			}
			defer pprof.StopCPUProfile()
		}

		if c.Bool("profile") {
			profilePort := c.Int("profile-port")
			profileEndpoint := fmt.Sprintf("localhost:%v", profilePort)
			go func() {
				logrus.Debugf("Starting profiling server on %v", profileEndpoint)
				if err := http.ListenAndServe(profileEndpoint, nil); err != nil {
					logrus.Fatalf("Unable to run profiling server: %v", err)
				}
			}()
		}

		if c.Args().Len() > 0 {
			cancel()
			return fmt.Errorf("command %q not supported", c.Args().Get(0))
		}

		// Check if we joined a mount namespace
		nsname, err := kubensmnt.Status()
		if nsname != "" {
			if err != nil {
				logrus.Warn(err)
			} else {
				logrus.Infof("Joined mount namespace %q", nsname)
			}
		}

		config, ok := c.App.Metadata["config"].(*libconfig.Config)
		if !ok {
			cancel()
			return errors.New("type assertion error when accessing server config")
		}

		// Validate the configuration during runtime
		if err := config.Validate(true); err != nil {
			cancel()
			return err
		}

		// Print the current CLI flags.
		for _, flagName := range c.FlagNames() {
			flagValue := c.Value(flagName)
			// Turn a multi-value flag into a single comma-separated list
			// of arguments, then wrap into a slice so that %v does it work
			// for us when rendering a slice type in the output.
			if _, ok := flagValue.(cli.StringSlice); ok {
				flagValue = []string{strings.Join(c.StringSlice(flagName), ",")}
			}
			logrus.Infof("FLAG: --%s=\"%v\"\n", flagName, flagValue)
		}

		// Print the current configuration.
		tomlConfig, err := config.ToString()
		if err != nil {
			logrus.Errorf("Unable to print current configuration: %v", err)
		} else {
			logrus.Infof("Current CRI-O configuration:\n%s", tomlConfig)
		}

		lis, err := server.Listen("unix", config.Listen)
		if err != nil {
			logrus.Fatalf("Failed to listen: %v", err)
		}

		if err := os.Chmod(config.Listen, 0o660); err != nil {
			logrus.Fatalf("Failed to chmod listen socket %s: %v", config.Listen, err)
		}

		var (
			tracerProvider *sdktrace.TracerProvider
			opts           []otelgrpc.Option
		)
		if config.EnableTracing {
			tracerProvider, opts, err = opentelemetry.InitTracing(
				ctx,
				config.TracingEndpoint,
				config.TracingSamplingRatePerMillion,
			)
			if err != nil {
				logrus.Fatalf("Failed to initialize tracer provider: %v", err)
			}
		}
		grpcServer := grpc.NewServer(
			grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
				otel_collector.UnaryInterceptor(),
			)),
			grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(
				otel_collector.StreamInterceptor(),
			)),
			grpc.StatsHandler(otelgrpc.NewServerHandler(opts...)),
			grpc.MaxSendMsgSize(config.GRPCMaxSendMsgSize),
			grpc.MaxRecvMsgSize(config.GRPCMaxRecvMsgSize),
		)

		crioServer, err := server.New(ctx, config)
		if err != nil {
			logrus.Fatal(err)
		}

		// Immediately upon start up, write our new version files
		// we write one to a tmpfs, so we can detect when a node rebooted.
		if err := info.WriteVersionFile(config.VersionFile); err != nil {
			logrus.Fatal(err)
		}
		// we then write to a persistent directory. This is to check if crio has upgraded
		// if it has, we should wipe images
		if err := info.WriteVersionFile(config.VersionFilePersist); err != nil {
			logrus.Fatal(err)
		}

		if config.CleanShutdownFile != "" {
			// clear out the shutdown file
			if err := os.Remove(config.CleanShutdownFile); err != nil && !os.IsNotExist(err) {
				logrus.Error(err)
			}

			if err := os.MkdirAll(filepath.Dir(config.CleanShutdownSupportedFileName()), 0o755); err != nil {
				logrus.Errorf("Creating clean shutdown supported parent directory: %v", err)
			}

			// Write "$CleanShutdownFile".supported to show crio-wipe that
			// we should be wiping if the CleanShutdownFile wasn't found.
			// This protects us from wiping after an upgrade from a version that doesn't support
			// CleanShutdownFile.
			f, err := os.Create(config.CleanShutdownSupportedFileName())
			if err != nil {
				logrus.Errorf("Writing clean shutdown supported file: %v", err)
			}
			f.Close()

			// and sync the changes to disk
			if err := utils.SyncParent(config.CleanShutdownFile); err != nil {
				logrus.Errorf("Failed to sync parent directory of clean shutdown file: %v", err)
			}
		}

		// We always use 'Volatile: true' when creating containers, which means that in
		// the event of an unclean shutdown, we might lose track of containers and layers.
		// We need to call the garbage collection function to clean up the redundant files.
		if err := crioServer.Store().GarbageCollect(); err != nil {
			logrus.Errorf("Attempts to clean up unreferenced old container leftovers failed: %v", err)
		}

		v1.RegisterRuntimeServiceServer(grpcServer, crioServer)
		v1.RegisterImageServiceServer(grpcServer, crioServer)

		// after the daemon is done setting up we can notify systemd api
		notifySystem()

		go func() {
			crioServer.StartExitMonitor(ctx)
		}()
		hookSync := make(chan error, 2)
		if crioServer.ContainerServer.Hooks == nil {
			hookSync <- err // so we don't block during cleanup
		} else {
			go crioServer.ContainerServer.Hooks.Monitor(ctx, hookSync)
			err = <-hookSync
			if err != nil {
				cancel()
				logrus.Fatal(err)
			}
		}

		m := cmux.New(lis)
		grpcL := m.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
		httpL := m.Match(cmux.HTTP1Fast())

		infoMux := crioServer.GetExtendInterfaceMux(c.Bool("enable-profile-unix-socket"))
		httpServer := &http.Server{
			Handler:     infoMux,
			ReadTimeout: 5 * time.Second,
		}

		graceful := false
		catchShutdown(ctx, cancel, grpcServer, tracerProvider, crioServer, httpServer, &graceful)

		go func() {
			if err := grpcServer.Serve(grpcL); err != nil {
				logrus.Errorf("Unable to run GRPC server: %v", err)
			}
		}()
		go func() {
			if err := httpServer.Serve(httpL); err != nil {
				logrus.Debugf("Closed http server")
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

		streamServerCloseCh := crioServer.StreamingServerCloseChan()
		serverMonitorsCh := crioServer.MonitorsCloseChan()
		select {
		case <-streamServerCloseCh:
		case <-serverMonitorsCh:
		case <-serverCloseCh:
		}

		if err := crioServer.Shutdown(ctx); err != nil {
			logrus.Warnf("Error shutting down service: %v", err)
		}
		cancel()

		<-streamServerCloseCh
		logrus.Debugf("Closed stream server")
		<-serverMonitorsCh
		logrus.Debugf("Closed monitors")
		err = <-hookSync
		if err == nil || errors.Is(err, context.Canceled) {
			logrus.Debugf("Closed hook monitor")
		} else {
			logrus.Errorf("Hook monitor failed: %v", err)
		}
		<-serverCloseCh
		logrus.Debugf("Closed main server")

		memProfilePath := c.String("profile-mem")
		if memProfilePath != "" {
			logrus.Infof("Creating memory profile in: %v", memProfilePath)

			file, err := os.Create(memProfilePath)
			if err != nil {
				return fmt.Errorf("could not create memory profile: %w", err)
			}
			defer file.Close()
			runtime.GC()

			if err := pprof.WriteHeapProfile(file); err != nil {
				return fmt.Errorf("could not write memory profile: %w", err)
			}
		}

		return nil
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
