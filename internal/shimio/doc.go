// Package shimio provides the container and exec stdio handling used to talk
// to containerd shim-v2 runtimes over FIFOs.
//
// It is derived from containerd v1.7 (github.com/containerd/containerd,
// packages cio, pkg/cri/io and pkg/ioutil), Copyright The containerd Authors,
// licensed under the Apache License, Version 2.0. containerd v2 moved the CRI
// IO code into an internal package, so CRI-O carries its own copy.
package shimio
