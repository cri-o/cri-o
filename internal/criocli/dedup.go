package criocli

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/server"
)

// DedupCommand is the `crio dedup` subcommand for running storage
// deduplication on demand. It opens the storage, runs deduplication
// using reflinks, and reports the result. This should be run while
// CRI-O is stopped to avoid lock contention with image pulls.
var DedupCommand = &cli.Command{
	Name: "dedup",
	Usage: `Deduplicate similar files in image layers using filesystem reflinks.

    Deduplication uses copy-on-write (reflink) to share identical blocks across
    image layers, reducing physical disk usage. Unlike hard links, reflinks allow
    independent modification of files. Requires filesystem support (XFS with
    reflink=1, Btrfs, etc).

    Deduplication finds files with identical content across image layers and uses
    the FIEMAP ioctl to deduplicate them via reflinks. The process uses SHA256
    hashing to identify duplicate files.

    This command should be run while CRI-O is stopped to avoid conflicts with
    running containers.

    When deduplication occurs: Deduplication happens after images are pulled
    and stored. Space savings occur when multiple images share common layers or
    when duplicate files exist across different image layers. Storage capacity
    must still accommodate initial image pulls before deduplication runs.`,
	Action: crioDedup,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "physical-disk-usage",
			Aliases: []string{"p"},
			Usage: `Measure and report actual physical disk usage before and after deduplication using FIEMAP ioctl.
    This provides reflink-aware reporting that shows true space savings by accounting for shared extents.
    Linux only.`,
		},
	},
}

func crioDedup(c *cli.Context) error {
	ctx := c.Context

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
			log.Errorf(ctx, "Unable to shutdown storage: %v", err)
		}
	}()

	showPhysicalUsage := c.Bool("physical-disk-usage")

	return server.RunDedup(ctx, store, showPhysicalUsage)
}
