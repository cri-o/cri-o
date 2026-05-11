package main

import (
	criu "github.com/checkpoint-restore/go-criu/v8/utils"
)

func main() {
	if err := criu.CheckForCriu(criu.PodCriuVersion); err != nil {
		panic(err)
	}
}
