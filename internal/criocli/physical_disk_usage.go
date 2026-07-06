//go:build linux

package criocli

import (
	"fmt"
	"path"

	"github.com/urfave/cli/v2"

	"github.com/cri-o/cri-o/utils"
)

var PhysicalDiskUsageCommand = &cli.Command{
	Name:        "physical-disk-usage",
	Usage:       "report real physical disk usage accounting for reflinks/deduplication",
	Description: "Reports actual physical disk usage for container storage directories.",
	Action:      physicalDiskUsage,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "show detailed breakdown per directory",
		},
	},
}

func physicalDiskUsage(c *cli.Context) error {
	config, err := GetConfigFromContext(c)
	if err != nil {
		return err
	}

	store, err := config.GetStore()
	if err != nil {
		return err
	}

	rootPath := store.GraphRoot()
	imagePath := store.ImageStore()
	storageDriver := store.GraphDriverName()

	verbose := c.Bool("verbose")

	// Analyze container storage
	var graphRootPath string
	if imagePath == "" {
		graphRootPath = path.Join(rootPath, storageDriver+"-images")
	} else {
		graphRootPath = path.Join(rootPath, storageDriver+"-containers")
	}

	fmt.Println("=== Physical Disk Usage (FIEMAP - Reflink-Aware) ===")
	fmt.Printf("Storage driver: %s\n", storageDriver)
	fmt.Printf("Graph root: %s\n\n", rootPath)

	// Total usage across all storage
	var totalStandard, totalReal uint64

	// Container storage
	fmt.Printf("Analyzing container storage: %s\n", graphRootPath)
	standardBytes, standardInodes, err := utils.GetDiskUsageStats(graphRootPath)
	if err != nil {
		return fmt.Errorf("failed to get standard usage for %s: %w", graphRootPath, err)
	}

	realBytes, realInodes, err := utils.GetRealPhysicalUsage(graphRootPath)
	if err != nil {
		return fmt.Errorf("failed to get real usage for %s: %w", graphRootPath, err)
	}

	printUsage("Container storage", standardBytes, standardInodes, realBytes, realInodes, verbose)
	totalStandard += standardBytes
	totalReal += realBytes

	// Image storage (if separate)
	if imagePath != "" {
		imageRoot := path.Join(imagePath, storageDriver+"-images")
		fmt.Printf("\nAnalyzing image storage: %s\n", imageRoot)

		standardBytes, standardInodes, err = utils.GetDiskUsageStats(imageRoot)
		if err != nil {
			return fmt.Errorf("failed to get standard usage for %s: %w", imageRoot, err)
		}

		realBytes, realInodes, err = utils.GetRealPhysicalUsage(imageRoot)
		if err != nil {
			return fmt.Errorf("failed to get real usage for %s: %w", imageRoot, err)
		}

		printUsage("Image storage", standardBytes, standardInodes, realBytes, realInodes, verbose)
		totalStandard += standardBytes
		totalReal += realBytes
	}

	// Summary
	fmt.Println("\n=== Summary ===")
	saved := int64(totalStandard) - int64(totalReal)
	if saved > 0 {
		pct := float64(saved) / float64(totalStandard) * 100
		fmt.Printf("Standard reporting:  %s (%d inodes)\n", formatBytes(totalStandard), standardInodes)
		fmt.Printf("Real physical usage: %s (%d inodes)\n", formatBytes(totalReal), realInodes)
		fmt.Printf("Space saved by deduplication: %s (%.1f%%)\n", formatBytes(uint64(saved)), pct)
	} else {
		fmt.Printf("Total usage: %s\n", formatBytes(totalReal))
		fmt.Println("No deduplication detected")
	}

	return nil
}

func printUsage(label string, standardBytes, standardInodes, realBytes, realInodes uint64, verbose bool) {
	saved := int64(standardBytes) - int64(realBytes)

	if verbose {
		fmt.Printf("  Standard (stat.Blocks): %s (%d inodes)\n", formatBytes(standardBytes), standardInodes)
		fmt.Printf("  Real (FIEMAP):          %s (%d inodes)\n", formatBytes(realBytes), realInodes)
	}

	if saved > 0 {
		pct := float64(saved) / float64(standardBytes) * 100
		fmt.Printf("  %s: %s (%.1f%% dedup savings)\n", label, formatBytes(realBytes), pct)
	} else {
		fmt.Printf("  %s: %s (no dedup)\n", label, formatBytes(realBytes))
	}
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
