package main

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/criocli"
	"github.com/cri-o/cri-o/internal/version"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"k8s.io/release/pkg/command"
)

const crictl = "crictl"

var wipeCommand = &cli.Command{
	Name:   "wipe",
	Usage:  "wipe CRI-O's container and image storage",
	Action: crioWipe,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "force wipe by skipping the version check",
		},
	},
}

func crioWipe(c *cli.Context) error {
	cfg, err := criocli.GetConfigFromContext(c)
	if err != nil {
		return err
	}

	shouldWipe := true
	// First, check if we need to upgrade at all
	if !c.IsSet("force") {
		shouldWipe, err = version.ShouldCrioWipe(cfg.VersionFile)
		if err != nil {
			logrus.Warnf("%v", err)
		}
	}

	// if we should not wipe, exit with no error
	if !shouldWipe {
		logrus.Info("Major and minor version unchanged; no wipe needed")
		return nil
	}

	if !command.Available(crictl) {
		return errors.Errorf("%s not found in $PATH", crictl)
	}

	return removeContainersAndImages(cfg)
}

func removeContainersAndImages(c *config.Config) error {
	logrus.Info("Removing all containers and pods")
	e := fmt.Sprintf("--runtime-endpoint=unix://%s", c.Listen)
	if err := command.New(crictl, e, "rmp", "-fa").RunSuccess(); err != nil {
		return errors.Wrap(err, "removing all containers and pods")
	}

	logrus.Info("Removing all images")
	if err := command.New(crictl, e, "rmi", "-a").RunSuccess(); err != nil {
		return errors.Wrap(err, "removing all images")
	}
	return nil
}
