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

	_ "github.com/containers/libpod/pkg/hooks/0.1.0"
	"github.com/containers/storage/pkg/reexec"
	"github.com/cri-o/cri-o/pkg/clicommon"
	"github.com/cri-o/cri-o/pkg/signals"
	"github.com/cri-o/cri-o/server"
	"github.com/cri-o/cri-o/utils"
	"github.com/cri-o/cri-o/version"
	"github.com/sirupsen/logrus"
	"github.com/soheilhy/cmux"
	"github.com/urfave/cli"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

// gitCommit is the commit that the binary is being built from.
// It will be populated by the Makefile.
var gitCommit = ""

func writeCrioGoroutineStacks() {
	path := filepath.Join("/tmp", fmt.Sprintf("crio-goroutine-stacks-%s.log", strings.Replace(time.Now().Format(time.RFC3339), ":", "", -1)))
	if err := utils.WriteGoroutineStacksToFile(path); err != nil {
		logrus.Warnf("Failed to write goroutine stacks: %s", err)
	}
}

func catchShutdown(ctx context.Context, cancel context.CancelFunc, gserver *grpc.Server, sserver *server.Server, hserver *http.Server, signalled *bool) {

	sig := make(chan os.Signal, 2048)
	signal.Notify(sig, signals.Interrupt, signals.Term, unix.SIGUSR1, unix.SIGPIPE)
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
			hserver.Shutdown(ctx)
			sserver.StopStreamServer()
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
	goflag.CommandLine.Parse([]string{})

	if reexec.Init() {
		return
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

	var err error
	app.Flags, app.Metadata, err = clicommon.GetFlagsAndMetadata()
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.FlagsByName(configCommand.Flags))

	app.Commands = []cli.Command{
		configCommand,
	}

	app.Before = func(c *cli.Context) (err error) {
		config, err := clicommon.GetConfigFromContext(c)
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
				http.ListenAndServe(profileEndpoint, nil)
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

		config := c.App.Metadata["config"].(*server.Config)

		// Validate the configuration during runtime
		if err := config.Validate(true); err != nil {
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

		s := grpc.NewServer(
			grpc.MaxSendMsgSize(config.GRPCMaxSendMsgSize),
			grpc.MaxRecvMsgSize(config.GRPCMaxRecvMsgSize),
		)

		service, err := server.New(ctx, config)
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

		runtime.RegisterRuntimeServiceServer(s, service)
		runtime.RegisterImageServiceServer(s, service)

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
		srv := &http.Server{
			Handler:     infoMux,
			ReadTimeout: 5 * time.Second,
		}

		graceful := false
		catchShutdown(ctx, cancel, s, service, srv, &graceful)

		go s.Serve(grpcL)
		go srv.Serve(httpL)

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

		service.Shutdown(ctx)
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
