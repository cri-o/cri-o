package main

import (
	"fmt"
	"runtime"
	"strconv"
	"time"

	"github.com/urfave/cli"
)

// Overwritten at build time
var (
	gitCommit string
	buildInfo string
)

// versionCmd gets and prints version info for version command
func versionCmd(c *cli.Context) error {

	// Converts unix time from string to int64
	buildTime, err := strconv.ParseInt(buildInfo, 10, 64)
	if err != nil {
		return err
	}

	fmt.Println("Version:      ", Version)
	fmt.Println("Go Version:   ", runtime.Version())
	fmt.Println("Git Commit:   ", gitCommit)

	// Prints out the build time in readable format
	fmt.Println("Built:        ", time.Unix(buildTime, 0).Format(time.ANSIC))
	fmt.Println("OS/Arch:      ", runtime.GOOS+"/"+runtime.GOARCH)

	return nil
}

// Cli command to print out the full version of kpod
var versionCommand = cli.Command{
	Name:   "version",
	Usage:  "Display the KPOD Version Information",
	Action: versionCmd,
}
