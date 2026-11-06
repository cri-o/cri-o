package criocli

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var DedupCommand = &cli.Command{
	Name:    "dedup",
	Aliases: []string{"d"},
	Usage: "Trigger an on-demand, store-wide image layer deduplication pass and wait for it " +
		"to complete. Requires layer_dedup to be set to \"manual\" or \"after_pull\" in " +
		"crio.conf.d. This can take a long time on nodes with a large amount of layer " +
		"content already on disk (roughly 4-5 seconds per GB of stored layers); there is " +
		"no built-in timeout, so run it during a planned maintenance window. While the " +
		"pass runs it holds the image store write lock, so image pulls on the node will " +
		"block until it completes; running containers are unaffected.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:      socketArg,
			Aliases:   []string{"s"},
			Usage:     "absolute path to the unix socket",
			Value:     defaultSocket,
			TakesFile: true,
		},
	},
	Action:       dedupTrigger,
	OnUsageError: func(c *cli.Context, e error, b bool) error { return e },
}

func dedupTrigger(c *cli.Context) error {
	crioClient, err := crioClient(c)
	if err != nil {
		return err
	}

	fmt.Println("Triggering store-wide layer deduplication pass. This can take a while " +
		"depending on how much layer content is already on disk...")

	result, err := crioClient.TriggerDedup(c.Context)
	if err != nil {
		return err
	}

	fmt.Printf(
		"Deduplication pass complete: saved %.2f MB in %.2fs\n",
		result.SavedMB,
		result.DurationSeconds,
	)
	fmt.Println("Note: saved MB reflects the total currently shared across the store via " +
		"reflinks, not just what was newly saved by this pass.")
	fmt.Println("Note: the pass holds the image store write lock while running, so image " +
		"pulls on this node will block until it completes. To measure what a pass newly " +
		"saved, compare disk usage of the store before and after.")

	return nil
}
