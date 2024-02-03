//go:build !linux
// +build !linux

package server

import (
	"github.com/opencontainers/runtime-tools/generate"
)

func makeOCIConfigurationRootless(g *generate.Generator) {
}
