package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/Sirupsen/logrus"
	sreexec "github.com/containers/storage/pkg/reexec"
	dreexec "github.com/docker/docker/pkg/reexec"
	"github.com/kubernetes-incubator/ocid/server"
	"github.com/kubernetes/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
)

const (
	unixDomainSocket = "/var/run/ocid.sock"
)

func main() {
	if sreexec.Init() {
		return
	}
	if dreexec.Init() {
		return
	}

	app := cli.NewApp()
	app.Name = "ocic"
	app.Usage = "client for ocid"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "sandboxdir",
			Value: "/var/lib/ocid/sandboxes",
			Usage: "ocid pod sandbox dir",
		},
		cli.StringFlag{
			Name:  "runtime",
			Value: "/usr/bin/runc",
			Usage: "OCI runtime path",
		},
		cli.StringFlag{
			Name:  "containerdir",
			Value: "/var/lib/ocid/containers",
			Usage: "ocid container dir",
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
		// Remove the socket if it already exists
		if _, err := os.Stat(unixDomainSocket); err == nil {
			if err := os.Remove(unixDomainSocket); err != nil {
				log.Fatal(err)
			}
		}
		lis, err := net.Listen("unix", unixDomainSocket)
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}

		s := grpc.NewServer()

		containerDir := c.String("containerdir")
		sandboxDir := c.String("sandboxdir")
		service, err := server.New(c.String("runtime"), sandboxDir, containerDir)
		if err != nil {
			log.Fatal(err)
		}

		runtime.RegisterRuntimeServiceServer(s, service)
		runtime.RegisterImageServiceServer(s, service)
		s.Serve(lis)
		return nil
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
