package criocli

import (
	"errors"
	"fmt"

	"github.com/urfave/cli/v2"
	"go.podman.io/common/libimage"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/docker/reference"

	"github.com/cri-o/cri-o/internal/ociartifact"
)

// ArtifactCommand is the top-level CLI command for managing OCI artifacts.
var ArtifactCommand = &cli.Command{
	Name:  "artifact",
	Usage: "Manage OCI artifacts in CRI-O's artifact store",
	Subcommands: []*cli.Command{
		artifactPullCommand,
	},
}

var artifactPullCommand = &cli.Command{
	Name:      "pull",
	Usage:     "Pull an OCI artifact into CRI-O's artifact store",
	ArgsUsage: "<reference>",
	Action:    artifactPull,
}

func artifactPull(c *cli.Context) error {
	if c.NArg() != 1 {
		return errors.New("usage: crio artifact pull <reference>")
	}

	refStr := c.Args().First()

	named, err := reference.ParseNormalizedNamed(refStr)
	if err != nil {
		return fmt.Errorf("invalid reference %q: %w", refStr, err)
	}

	config, err := GetConfigFromContext(c)
	if err != nil {
		return err
	}

	store, err := config.GetStore()
	if err != nil {
		return fmt.Errorf("open container storage: %w", err)
	}

	artStore, err := ociartifact.NewStore(store.GraphRoot(), config.AdditionalArtifactStores, config.SystemContext, nil)
	if err != nil {
		return fmt.Errorf("create artifact store: %w", err)
	}

	imageRef, err := docker.NewReference(named)
	if err != nil {
		return fmt.Errorf("create image reference for %q: %w", refStr, err)
	}

	if _, err := artStore.Pull(c.Context, imageRef, &libimage.CopyOptions{
		RemoveSignatures: true, // OCI layout destination does not support signature storage
	}); err != nil {
		return fmt.Errorf("pull artifact %q: %w", refStr, err)
	}

	fmt.Printf("Successfully pulled artifact: %s\n", refStr)

	return nil
}
