package criocli

import (
	"fmt"

	"github.com/containers/storage"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/utils"
)

type checkErrors map[string][]error

var CheckCommand = &cli.Command{
	Name:   "check",
	Usage:  usageText,
	Action: crioCheck,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "age",
			Aliases: []string{"a"},
			Value:   "24h",
			Usage:   "Maximum allowed age for unreferenced layers",
		},
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Remove damaged containers",
		},
		&cli.BoolFlag{
			Name:    "repair",
			Aliases: []string{"r"},
			Usage:   "Remove damaged images and layers",
		},
		&cli.BoolFlag{
			Name:    "quick",
			Aliases: []string{"q"},
			Usage:   "Perform only quick checks",
		},
		&cli.BoolFlag{
			Name:    "wipe",
			Aliases: []string{"w"},
			Usage:   "Wipe storage directory on repair failure",
		},
	},
}

func crioCheck(c *cli.Context) error {
	config, err := GetConfigFromContext(c)
	if err != nil {
		return fmt.Errorf("unable to load configuration: %w", err)
	}

	store, err := config.GetStore()
	if err != nil {
		return fmt.Errorf("unable to open storage: %w", err)
	}
	defer func() {
		if _, err := store.Shutdown(true); err != nil {
			logrus.Errorf("Unable to shutdown storage: %v", err)
		}
	}()

	graphRoot := store.GraphRoot()
	logrus.Infof("Checking storage directory %s for errors", graphRoot)

	checkOptions := storage.CheckEverything()
	if c.Bool("quick") {
		// This is not the same as the "quick" check that CRI-O performs during its start-up
		// following an unclean shutdown, as this one would set the `LayerDigests` option,
		// which is I/O and CPU intensive, whereas the other one does not.
		checkOptions = storage.CheckMost()
	}

	// The maximum unreferenced layer age.
	layerAge := c.String("age")
	if layerAge != "" {
		age, err := utils.ParseDuration(layerAge)
		if err != nil {
			return fmt.Errorf("unable to parse age duration: %w", err)
		}
		checkOptions.LayerUnreferencedMaximumAge = &age
	}

	report, err := store.Check(checkOptions)
	if err != nil {
		return fmt.Errorf("unable to check storage: %w", err)
	}

	// Walk each report and show details...
	for prefix, checkReport := range map[string]checkErrors{
		"layer":           report.Layers,
		"read-only layer": report.ROLayers,
		"image":           report.Images,
		"read-only image": report.ROImages,
		"container":       report.Containers,
	} {
		for identifier, errs := range checkReport {
			for _, err := range errs {
				logrus.Debugf("%s: %s: %v", prefix, identifier, err)
			}
		}
	}

	seenStorageErrors := lib.CheckReportHasErrors(report)
	logrus.Debugf("Storage directory %s has errors: %t", graphRoot, seenStorageErrors)

	if !c.Bool("repair") {
		if seenStorageErrors {
			logrus.Warnf("Errors found while checking storage directory %s for errors", graphRoot)
			return fmt.Errorf(
				"%d layer errors, %d read-only layer errors, %d image errors, %d read-only image errors, %d container errors",
				len(report.Layers),
				len(report.ROLayers),
				len(report.Images),
				len(report.ROImages),
				len(report.Containers),
			)
		}
		return nil
	}

	force := c.Bool("force")
	if force {
		logrus.Warn("The `force` option has been set, repair will attempt to remove damaged containers")
	}
	logrus.Infof("Attempting to repair storage directory %s", graphRoot)

	errs := store.Repair(report, &storage.RepairOptions{
		RemoveContainers: force,
	})
	if len(errs) != 0 {
		for _, err := range errs {
			logrus.Error(err)
		}

		if c.Bool("wipe") {
			// Depending on whether the `force` option is set or not, this will remove the
			// storage directory completely while ignoring any running containers. Otherwise,
			// this will fail if there are any containers currently running.
			if force {
				logrus.Warn("The `force` option has been set, storage directory will be forcefully removed")
			}
			logrus.Infof("Wiping storage directory %s", graphRoot)
			return lib.RemoveStorageDirectory(config, store, force)
		}

		return errs[0]
	}

	if len(report.ROLayers) > 0 || len(report.ROImages) > 0 || (!force && len(report.Containers) > 0) {
		if force {
			// Any damaged containers would have been deleted at this point.
			return fmt.Errorf(
				"%d read-only layer errors, %d read-only image errors",
				len(report.ROLayers),
				len(report.ROImages),
			)
		}
		return fmt.Errorf(
			"%d read-only layer errors, %d read-only image errors, %d container errors",
			len(report.ROLayers),
			len(report.ROImages),
			len(report.Containers),
		)
	}

	return nil
}

// The `Description` field will not be rendered when the documentation
// is generated, and using `Usage` makes the formatting wrong when the
// command-line help is rendered. Shell completions might also be
// incorrect.
var usageText = `Check CRI-O storage directory for errors.

This command can also repair damaged containers, images and layers.

By default, the data integrity of the storage directory is verified,
which can be an I/O and CPU-intensive operation. The --quick option
can be used to reduce the number of checks run.

When using the --repair option, especially with the --force option,
CRI-O and any currently running containers should be stopped if
possible to ensure no concurrent access to the storage directory
occurs.

The --wipe option can be used to automatically attempt to remove
containers and images on a repair failure. This option, combined
with the --force option, can be used to entirely remove the storage
directory content in case of irrecoverable errors. This should be
used as a last resort, and similarly to the --repair option, it's
best if CRI-O and any currently running containers are stopped.`
