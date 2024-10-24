package main

import (
	"os"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	sstorage "github.com/containers/storage"
	"github.com/containers/storage/pkg/reexec"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/cri-o/cri-o/internal/log"
)

func main() {
	if reexec.Init() {
		return
	}

	app := cli.NewApp()
	app.Name = "copyimg"
	app.Usage = "barebones image copier"
	app.Version = "0.0.1"

	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "turn on debug logging",
		},
		&cli.StringFlag{
			Name:    "root",
			Aliases: []string{"r"},
			Usage:   "graph root directory",
		},
		&cli.StringFlag{
			Name:  "runroot",
			Usage: "run root directory",
		},
		&cli.StringFlag{
			Name:    "storage-driver",
			Aliases: []string{"s"},
			Usage:   "storage driver",
		},
		&cli.StringSliceFlag{
			Name:  "storage-opt",
			Usage: "storage option",
		},
		&cli.StringFlag{
			Name:  "signature-policy",
			Usage: "signature policy",
		},
		&cli.StringFlag{
			Name:  "image-name",
			Usage: "set image name",
		},
		&cli.StringFlag{
			Name:  "add-name",
			Usage: "name to add to image",
		},
		&cli.StringFlag{
			Name:  "import-from",
			Usage: "import source",
		},
		&cli.StringFlag{
			Name:  "export-to",
			Usage: "export target",
		},
	}

	app.Action = func(c *cli.Context) error {
		var store sstorage.Store
		var ref, importRef, exportRef types.ImageReference
		var err error

		debug := c.Bool("debug")
		rootDir := c.String("root")
		runrootDir := c.String("runroot")
		storageDriver := c.String("storage-driver")
		storageOptions := c.StringSlice("storage-opt")
		signaturePolicy := c.String("signature-policy")
		imageName := c.String("image-name")
		addName := c.String("add-name")
		importFrom := c.String("import-from")
		exportTo := c.String("export-to")

		ctx := c.Context

		if debug {
			logrus.SetLevel(logrus.DebugLevel)
		} else {
			logrus.SetLevel(logrus.ErrorLevel)
		}

		if imageName != "" {
			if rootDir == "" && runrootDir != "" {
				log.Fatalf(ctx, "Must set --root and --runroot, or neither")
			}
			if rootDir != "" && runrootDir == "" {
				log.Fatalf(ctx, "Must set --root and --runroot, or neither")
			}
			storeOptions, err := sstorage.DefaultStoreOptions()
			if err != nil {
				return err
			}
			if rootDir != "" && runrootDir != "" {
				storeOptions.GraphDriverName = storageDriver
				storeOptions.GraphDriverOptions = storageOptions
				storeOptions.GraphRoot = rootDir
				storeOptions.RunRoot = runrootDir
			}
			store, err = sstorage.GetStore(storeOptions)
			if err != nil {
				log.Fatalf(ctx, "Error opening storage: %v", err)
			}
			defer func() {
				_, err = store.Shutdown(false)
				if err != nil {
					log.Warnf(ctx, "Unable to shutdown store: %v", err)
				}
			}()

			storage.Transport.SetStore(store)
			ref, err = storage.Transport.ParseStoreReference(store, imageName)
			if err != nil {
				log.Fatalf(ctx, "Error parsing image name: %v", err)
			}
		}

		systemContext := types.SystemContext{
			SignaturePolicyPath: signaturePolicy,
		}
		policy, err := signature.DefaultPolicy(&systemContext)
		if err != nil {
			log.Fatalf(ctx, "Error loading signature policy: %v", err)
		}
		policyContext, err := signature.NewPolicyContext(policy)
		if err != nil {
			log.Fatalf(ctx, "Error loading signature policy: %v", err)
		}
		defer func() {
			err = policyContext.Destroy()
			if err != nil {
				log.Fatalf(ctx, "Unable to destroy policy context: %v", err)
			}
		}()
		options := &copy.Options{}

		if importFrom != "" {
			importRef, err = alltransports.ParseImageName(importFrom)
			if err != nil {
				log.Fatalf(ctx, "Error parsing image name %v: %v", importFrom, err)
			}
		}

		if exportTo != "" {
			exportRef, err = alltransports.ParseImageName(exportTo)
			if err != nil {
				log.Fatalf(ctx, "Error parsing image name %v: %v", exportTo, err)
			}
		}

		if imageName != "" {
			if importFrom != "" {
				_, err = copy.Image(ctx, policyContext, ref, importRef, options)
				if err != nil {
					log.Fatalf(ctx, "Error importing %s: %v", importFrom, err)
				}
			}
			if addName != "" {
				_, destImage, err1 := storage.ResolveReference(ref)
				if err1 != nil {
					log.Fatalf(ctx, "Error finding image: %v", err1)
				}
				err = store.AddNames(destImage.ID, []string{imageName, addName})
				if err != nil {
					log.Fatalf(ctx, "Error adding name to %s: %v", imageName, err)
				}
			}
			if exportTo != "" {
				_, err = copy.Image(ctx, policyContext, exportRef, ref, options)
				if err != nil {
					log.Fatalf(ctx, "Error exporting %s: %v", exportTo, err)
				}
			}
		} else if importFrom != "" && exportTo != "" {
			_, err = copy.Image(ctx, policyContext, exportRef, importRef, options)
			if err != nil {
				log.Fatalf(ctx, "Error copying %s to %s: %v", importFrom, exportTo, err)
			}
		}
		return nil
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
