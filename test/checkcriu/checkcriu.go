package main

import (
	"os"

	"github.com/containers/podman/v4/pkg/criu"
)

func main() {
	if !criu.CheckForCriu(criu.PodCriuVersion) {
		os.Exit(1)
	}

	os.Exit(0)
}
