package main

import (
	"github.com/containers/podman/v4/pkg/criu"
)

func main() {
	if err := criu.CheckForCriu(criu.PodCriuVersion); err != nil {
		panic(err)
	}
}
