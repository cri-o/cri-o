package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	imagecopy "go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/directory"
	"go.podman.io/image/v5/signature"
	"go.podman.io/image/v5/storage"
	"go.podman.io/image/v5/transports/alltransports"
	"go.podman.io/image/v5/types"
	sstorage "go.podman.io/storage"
	"go.podman.io/storage/pkg/reexec"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/cri-o/cri-o/internal/log"
)

// isRemoteSource checks if the image reference is from a remote source
// that requires network access (and thus might benefit from retries).
func isRemoteSource(ref string) bool {
	// Remote transports that require network access
	remoteTransports := []string{"docker://", "docker-archive:", "docker-daemon:"}
	for _, transport := range remoteTransports {
		if strings.HasPrefix(ref, transport) {
			return true
		}
	}

	return false
}

// cleanupDirDestination empties a dir: destination so that a retry starts from
// scratch. The dir transport writes its version marker only on a successful
// commit and refuses to write into a non-empty directory without it, so a copy
// which failed halfway through makes every following attempt fail while
// initializing the destination.
//
// Only destinations the copy owns may be cleaned up, see destinationIsForeign.
func cleanupDirDestination(dest string) error {
	const dirPrefix = "dir:"

	if !strings.HasPrefix(dest, dirPrefix) {
		return nil
	}

	path := strings.TrimPrefix(dest, dirPrefix)
	if path == "" {
		return nil
	}

	return os.RemoveAll(path)
}

// destinationIsForeign reports whether err means that the copy never got as far
// as writing the destination, because the directory existed before and is not a
// containers image directory the copy may overwrite. Such a directory is not
// ours to remove: it can hold unrelated data, and retrying against it fails the
// same way anyway.
func destinationIsForeign(err error) bool {
	if errors.Is(err, directory.ErrNotContainerImageDir) {
		return true
	}

	var unsupportedVersion directory.UnsupportedVersionError

	return errors.As(err, &unsupportedVersion)
}

// copyImageWithRetry attempts to copy an image with exponential backoff retry.
// The cleanup callback, if set, gets called after every failed attempt to reset
// the destination before the next one. The retries stop early when the
// destination is not the copy's to clean up, or when cleaning it up fails.
func copyImageWithRetry(
	ctx context.Context,
	policyContext *signature.PolicyContext,
	dest, src types.ImageReference,
	options *imagecopy.Options,
	retryAttempts int,
	cleanup func() error,
) error {
	backoff := wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   2.0,
		Steps:    retryAttempts,
	}

	var firstErr, lastErr, cleanupErr error

	attempts := 0

	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		attempts++

		_, copyErr := imagecopy.Image(ctx, policyContext, dest, src, options)
		if copyErr == nil {
			return true, nil
		}

		if firstErr == nil {
			firstErr = copyErr
		}

		lastErr = copyErr

		if destinationIsForeign(copyErr) {
			return false, copyErr
		}

		logrus.Warnf("Image copy attempt %d failed (retrying): %v", attempts, copyErr)

		if cleanup != nil {
			cleanupErr = cleanup()
			if cleanupErr != nil {
				// The next attempt would run against the untouched
				// destination and fail while initializing it, which hides
				// the reason the cleanup failed.
				return false, cleanupErr
			}
		}

		return false, nil
	})

	switch {
	case err == nil:
		return nil
	case cleanupErr != nil:
		return fmt.Errorf(
			"copy failed after %d attempts: %w (cleaning up the destination failed: %w)",
			attempts, lastErr, cleanupErr,
		)
	case destinationIsForeign(err):
		return fmt.Errorf("copy failed after %d attempts: %w", attempts, err)
	case firstErr == nil:
		return err
	}

	// The last error usually only says that the retry could not start,
	// while the first one carries the reason why the copy actually
	// failed, so report both to not hide the root cause.
	if firstErr.Error() == lastErr.Error() {
		return lastErr
	}

	return fmt.Errorf(
		"copy failed after %d attempts: %w (first error: %w)",
		attempts, lastErr, firstErr,
	)
}

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
		&cli.IntFlag{
			Name:  "retry-attempts",
			Usage: "number of retry attempts for image pull with exponential backoff (0 to disable, default: 3)",
			Value: 3,
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
		retryAttempts := c.Int("retry-attempts")

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

		options := &imagecopy.Options{}

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
				// Use retry logic only when pulling from remote sources and retry is enabled (retryAttempts > 0)
				if retryAttempts > 0 && isRemoteSource(importFrom) {
					logrus.Infof(
						"Pulling image with retry enabled (max attempts: %d)",
						retryAttempts,
					)
					err = copyImageWithRetry(
						ctx,
						policyContext,
						ref,
						importRef,
						options,
						retryAttempts,
						nil,
					)
				} else {
					_, err = imagecopy.Image(ctx, policyContext, ref, importRef, options)
				}

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
				_, err = imagecopy.Image(ctx, policyContext, exportRef, ref, options)
				if err != nil {
					log.Fatalf(ctx, "Error exporting %s: %v", exportTo, err)
				}
			}
		} else if importFrom != "" && exportTo != "" {
			// Use retry logic only when pulling from remote sources and retry is enabled (retryAttempts > 0)
			if retryAttempts > 0 && isRemoteSource(importFrom) {
				logrus.Infof("Pulling image with retry enabled (max attempts: %d)", retryAttempts)
				err = copyImageWithRetry(
					ctx,
					policyContext,
					exportRef,
					importRef,
					options,
					retryAttempts,
					func() error { return cleanupDirDestination(exportTo) },
				)
			} else {
				_, err = imagecopy.Image(ctx, policyContext, exportRef, importRef, options)
			}

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
