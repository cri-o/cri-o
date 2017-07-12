package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/containers/storage/pkg/reexec"
	"github.com/kubernetes-incubator/cri-o/server"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"os"
)

const crioConfigPath = "/etc/crio/crio.conf"

var stopTimeout int64
var configPath string

var (
	stopFlags = []cli.Flag{
		cli.Int64Flag{
			Name:        "timeout, t",
			Usage:       "Seconds to wait to kill the container after a graceful stop is requested (default 10",
			Value:       10,
			Destination: &stopTimeout,
		},
		cli.StringFlag{
			Name:        "config",
			Usage:       "path to configuration file",
			Value:       crioConfigPath,
			Destination: &configPath,
		},
	}

	stopDescription = "Stops one or more containers"
	stopCommand     = cli.Command{
		Name:        "stop",
		Usage:       "Stop one or more containers",
		Description: stopDescription,
		Flags:       stopFlags,
		Action:      stopCmd,
		ArgsUsage:   "CONTAINER-ID [CONTAINER-ID...]",
	}
)

func stopCmd(c *cli.Context) error {
	var hasError bool = false
	args := c.Args()
	if len(args) < 1 {
		return errors.Errorf("specify one or more container IDs")
	}
	config := new(server.Config)
	if err := config.FromFile(configPath); err != nil {
		return err
	}
	if reexec.Init() {
		return nil
	}
	service, err := server.New(config)
	if err != nil {
		return err
	}
	for _, cid := range args {
		r, err := service.StopContainer(context.Background(), &pb.StopContainerRequest{
			ContainerId: cid,
			Timeout:     stopTimeout,
		})
		if err != nil {
			hasError = true
			fmt.Fprintf(os.Stderr, "%s\n", err)
		}
		logrus.Debugf("StopContainerResponse: %+v", r)
	}
	if hasError {
		// mimics docker stop behaviour; errors are put on stdout
		// but return code needs to 1, so returning a blank error message
		os.Exit(1)
	}
	return nil
}
