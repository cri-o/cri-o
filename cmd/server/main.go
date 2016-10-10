package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/server"
	"github.com/opencontainers/runc/libcontainer/selinux"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
	"k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

const (
	ocidRoot      = "/var/lib/ocid"
	conmonPath    = "/usr/libexec/ocid/conmon"
	pausePath     = "/usr/libexec/ocid/pause"
	ociConfigPath = "/etc/ocid.conf"
)

// DefaultConfig returns the default configuration for ocid.
func DefaultConfig() *server.Config {
	return &server.Config{
		RootConfig: server.RootConfig{
			Root:         ocidRoot,
			SandboxDir:   filepath.Join(ocidRoot, "sandboxes"),
			ContainerDir: filepath.Join(ocidRoot, "containers"),
		},
		APIConfig: server.APIConfig{
			Listen: "/var/run/ocid.sock",
		},
		RuntimeConfig: server.RuntimeConfig{
			Runtime: "/usr/bin/runc",
			Conmon:  conmonPath,
			SELinux: selinux.SelinuxEnabled(),
		},
		ImageConfig: server.ImageConfig{
			Pause:      pausePath,
			ImageStore: filepath.Join(ocidRoot, "store"),
		},
	}
}

func mergeConfig(config *server.Config, ctx *cli.Context) error {
	// Don't parse the config if the user explicitly set it to "".
	if path := ctx.GlobalString("config"); path != "" {
		if err := config.FromFile(path); err != nil {
			if ctx.GlobalIsSet("config") || !os.IsNotExist(err) {
				return err
			}

			// We don't error out if --config wasn't explicitly set and the
			// default doesn't exist. But we will log a warning about it, so
			// the user doesn't miss it.
			logrus.Warnf("default configuration file does not exist: %s", ociConfigPath)
		}
	}

	// Override options set with the CLI.
	if ctx.GlobalIsSet("conmon") {
		config.Conmon = ctx.GlobalString("conmon")
	}
	if ctx.GlobalIsSet("pause") {
		config.Pause = ctx.GlobalString("pause")
	}
	if ctx.GlobalIsSet("root") {
		config.Root = ctx.GlobalString("root")
	}
	if ctx.GlobalIsSet("sandboxdir") {
		config.SandboxDir = ctx.GlobalString("sandboxdir")
	}
	if ctx.GlobalIsSet("containerdir") {
		config.ContainerDir = ctx.GlobalString("containerdir")
	}
	if ctx.GlobalIsSet("listen") {
		config.Listen = ctx.GlobalString("listen")
	}
	if ctx.GlobalIsSet("runtime") {
		config.Runtime = ctx.GlobalString("runtime")
	}
	if ctx.GlobalIsSet("selinux") {
		config.SELinux = ctx.GlobalBool("selinux")
	}
	return nil
}

func main() {
	app := cli.NewApp()
	app.Name = "ocid"
	app.Usage = "ocid server"
	app.Version = "0.0.1"
	app.Metadata = map[string]interface{}{
		"config": DefaultConfig(),
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config",
			Value: ociConfigPath,
			Usage: "path to configuration file",
		},
		cli.StringFlag{
			Name:  "conmon",
			Usage: "path to the conmon executable",
		},
		cli.StringFlag{
			Name:  "pause",
			Usage: "path to the pause executable",
		},
		cli.StringFlag{
			Name:  "root",
			Usage: "ocid root dir",
		},
		cli.StringFlag{
			Name:  "sandboxdir",
			Usage: "ocid pod sandbox dir",
		},
		cli.StringFlag{
			Name:  "containerdir",
			Usage: "ocid container dir",
		},
		cli.StringFlag{
			Name:  "listen",
			Usage: "path to ocid socket",
		},
		cli.StringFlag{
			Name:  "runtime",
			Usage: "OCI runtime path",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug output for logging",
		},
		cli.BoolFlag{
			Name:  "selinux",
			Usage: "enable selinux support",
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
	}

	app.Before = func(c *cli.Context) error {
		// Load the configuration file.
		config := c.App.Metadata["config"].(*server.Config)
		if err := mergeConfig(config, c); err != nil {
			return err
		}

		if c.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}

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
		config := c.App.Metadata["config"].(*server.Config)

		if !config.SELinux {
			selinux.SetDisabled()
		}

		if _, err := os.Stat(config.Runtime); os.IsNotExist(err) {
			// path to runtime does not exist
			return fmt.Errorf("invalid --runtime value %q", err)
		}

		// Remove the socket if it already exists
		if _, err := os.Stat(config.Listen); err == nil {
			if err := os.Remove(config.Listen); err != nil {
				logrus.Fatal(err)
			}
		}
		lis, err := net.Listen("unix", config.Listen)
		if err != nil {
			logrus.Fatalf("failed to listen: %v", err)
		}

		s := grpc.NewServer()

		service, err := server.New(config)
		if err != nil {
			logrus.Fatal(err)
		}

		runtime.RegisterRuntimeServiceServer(s, service)
		runtime.RegisterImageServiceServer(s, service)
		if err := s.Serve(lis); err != nil {
			logrus.Fatal(err)
		}
		return nil
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
