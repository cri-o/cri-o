package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/server"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
	"k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

const (
	ocidRoot   = "/var/lib/ocid"
	conmonPath = "/usr/libexec/ocid/conmon"
)

func main() {
	app := cli.NewApp()
	app.Name = "ocid"
	app.Usage = "ocid server"
	app.Version = "0.0.1"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "conmon",
			Value: conmonPath,
			Usage: "path to the conmon executable",
		},
		cli.StringFlag{
			Name:  "root",
			Value: ocidRoot,
			Usage: "ocid root dir",
		},
		cli.StringFlag{
			Name:  "sandboxdir",
			Value: filepath.Join(ocidRoot, "sandboxes"),
			Usage: "ocid pod sandbox dir",
		},
		cli.StringFlag{
			Name:  "containerdir",
			Value: filepath.Join(ocidRoot, "containers"),
			Usage: "ocid container dir",
		},
		cli.StringFlag{
			Name:  "socket",
			Value: "/var/run/ocid.sock",
			Usage: "path to ocid socket",
		},
		cli.StringFlag{
			Name:  "runtime",
			Value: "/usr/bin/runc",
			Usage: "OCI runtime path",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug output for logging",
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
		socketPath := c.String("socket")
		// Remove the socket if it already exists
		if _, err := os.Stat(socketPath); err == nil {
			if err := os.Remove(socketPath); err != nil {
				logrus.Fatal(err)
			}
		}
		lis, err := net.Listen("unix", socketPath)
		if err != nil {
			logrus.Fatalf("failed to listen: %v", err)
		}

		s := grpc.NewServer()

		containerDir := c.String("containerdir")
		sandboxDir := c.String("sandboxdir")
		service, err := server.New(c.String("runtime"), c.String("root"), sandboxDir, containerDir, conmonPath)
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
