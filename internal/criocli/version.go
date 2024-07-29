package criocli

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/cri-o/cri-o/internal/version"
)

const (
	jsonFlag    = "json"
	verboseFlag = "verbose"
)

var VersionCommand = &cli.Command{
	Name:  "version",
	Usage: "display detailed version information",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    jsonFlag,
			Aliases: []string{"j"},
			Usage:   "print JSON instead of text",
		},
		&cli.BoolFlag{
			Name:    verboseFlag,
			Aliases: []string{"v"},
			Usage:   "print verbose information (for example all golang dependencies)",
		},
	},
	Action: func(c *cli.Context) error {
		verbose := c.Bool(verboseFlag)
		v, err := version.Get(verbose)
		if err != nil {
			logrus.Fatal(err)
		}
		res := v.String()
		if c.Bool(jsonFlag) {
			j, err := v.JSONString()
			if err != nil {
				return fmt.Errorf("unable to generate JSON from version info: %w", err)
			}
			res = j

		}
		fmt.Print(res)
		return nil
	},
}
