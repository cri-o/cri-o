package main

import (
	"context"
	goflag "flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/containers/image/v5/types"
	_ "github.com/containers/libpod/pkg/hooks/0.1.0"
	"github.com/containers/storage/pkg/reexec"
	libconfig "github.com/cri-o/cri-o/internal/lib/config"
	"github.com/cri-o/cri-o/internal/pkg/criocli"
	"github.com/cri-o/cri-o/internal/pkg/log"
	"github.com/cri-o/cri-o/internal/pkg/signals"
	"github.com/cri-o/cri-o/internal/version"
	"github.com/cri-o/cri-o/server"
	"github.com/cri-o/cri-o/server/metrics"
	"github.com/cri-o/cri-o/utils"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
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

func writeCrioGoroutineStacks() {
	path := filepath.Join("/tmp", fmt.Sprintf("crio-goroutine-stacks-%s.log", strings.Replace(time.Now().Format(time.RFC3339), ":", "", -1))) // nolint: gocritic
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
	if gitCommit != "" && gitCommit != "unknown" {
		v = append(v, fmt.Sprintf("commit: %s", gitCommit))
	}
	app.Name = "crio"
	app.Usage = "crio server"
	app.Version = strings.Join(v, "\n")

	systemContext := &types.SystemContext{}

	var err error
	app.Flags, app.Metadata, err = criocli.GetFlagsAndMetadata(systemContext)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.FlagsByName(configCommand.Flags))

	app.Commands = []cli.Command{
		configCommand,
		criocli.Completion,
		wipeCommand,
	}

	var configPath string
	app.Before = func(c *cli.Context) (err error) {
		var config *libconfig.Config
		configPath, config, err = criocli.GetConfigFromContext(c)
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
		logrus.AddHook(log.NewFilenameHook())

		filterHook, err := log.NewFilterHook(config.LogFilter)
		if err != nil {
			return err
		}
		logrus.AddHook(filterHook)

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

		config, ok := c.App.Metadata["config"].(*libconfig.Config)
		if !ok {
			cancel()
			return fmt.Errorf("type assertion error when accessing server config")
		}

		// Validate the configuration during runtime
		if err := config.Validate(systemContext, true); err != nil {
			cancel()
			return err
		}

		lis, err := server.Listen("unix", config.Listen)
		if err != nil {
			logrus.Fatalf("failed to listen: %v", err)
		}

		grpcServer := grpc.NewServer(
			grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
				metrics.UnaryInterceptor(),
				log.UnaryInterceptor(),
			)),
			grpc.StreamInterceptor(log.StreamInterceptor()),
			grpc.MaxSendMsgSize(config.GRPCMaxSendMsgSize),
			grpc.MaxRecvMsgSize(config.GRPCMaxRecvMsgSize),
		)

		service, err := server.New(ctx, systemContext, configPath, config)
		if err != nil {
			logrus.Fatal(err)
		}

		// Immediately upon start up, write our new version file
		if err := version.WriteVersionFile(config.VersionFile, gitCommit); err != nil {
			logrus.Fatal(err)
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
		grpcL := m.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
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
