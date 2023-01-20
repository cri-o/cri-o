package criocli

import (
	"github.com/urfave/cli/v2"
)

var PublishCommand = &cli.Command{
	Name:  "publish",
	Usage: "receive shimv2 events",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:   "topic",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "namespace",
			Hidden: true,
		},
	},
	HideHelp: true,
	Hidden:   true,
	Action: func(c *cli.Context) error {
		return nil
	},
}
