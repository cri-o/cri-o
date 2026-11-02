//go:build !linux

package server

import (
	"context"

	"github.com/opencontainers/runtime-tools/generate"
)

func restrictOOMScoreAdjInUserNS(context.Context, *generate.Generator) {
}
