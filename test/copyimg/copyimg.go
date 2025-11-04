package main

import (
	"os"

	"github.com/containers/common/libimage"
	"github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	sstorage "github.com/containers/storage"
	"github.com/containers/storage/pkg/reexec"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/ociartifact"
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

		copyOptions := &libimage.CopyOptions{
			SystemContext: &systemContext,
		}

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

		copier, err := libimage.NewCopier(copyOptions, &systemContext)
		if err != nil {
			log.Fatalf(ctx, "Error creating copier: %v", err)
		}

		defer func() {
			err = copier.Close()
			if err != nil {
				log.Warnf(ctx, "Unable to close copier: %v", err)
			}
		}()

		if imageName != "" {
			if importFrom != "" {
				_, err = copier.Copy(ctx, importRef, ref)
				if err != nil {
					// Try importing as OCI artifact (stored in <graphRoot>/artifacts, not containers/storage)
					log.Infof(ctx, "Failed to import as image, trying as OCI artifact: %v", err)

					// Only docker:// transport can be pulled to artifact store; dir:// artifacts are already on disk
					if importRef.Transport().Name() == "docker" {
						artifactStore := ociartifact.NewStore(store.GraphRoot(), &systemContext)

						_, artifactErr := artifactStore.PullManifest(ctx, importRef, &ociartifact.PullOptions{
							CopyOptions: copyOptions,
						})
						if artifactErr != nil {
							log.Fatalf(ctx, "Error importing %s as image or artifact: image err: %v; artifact err: %v",
								importFrom, err, artifactErr)
						}

						log.Infof(ctx, "Successfully imported OCI artifact %s", importFrom)
					} else {
						log.Infof(ctx, "Skipping import of artifact from %s - CRI-O will use it from the ociartifact store", importFrom)
					}
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
				_, err = copier.Copy(ctx, ref, exportRef)
				if err != nil {
					log.Fatalf(ctx, "Error exporting %s: %v", exportTo, err)
				}
			}
		} else if importFrom != "" && exportTo != "" {
			_, err = copier.Copy(ctx, importRef, exportRef)
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
