#!/bin/bash
# install-golangci-lint.sh: a bash script to install golangci-lint

# this script is primarily for use in the CRI-O Makefile.
# it expects GOPATH to be set, and GOPKGDIR to be the
# location of the CRI-O dir in the GOPATH hierarchy.

set -x

# the following commit is one that supports go 1.10
# which this version of CRI-O currently supports
COMMIT=v1.18.0

function return_to_crio() {
	cd $GOPKGDIR
}

trap return_to_crio EXIT

mkdir "$GOPATH/src/github.com/golangci" && \
cd "$GOPATH/src/github.com/golangci" && \
git clone https://github.com/golangci/golangci-lint.git && \
cd golangci-lint && \
git checkout $COMMIT && \
GO111MODULE=off go build -o golangci-lint ./cmd/golangci-lint
cp golangci-lint "$GOPATH/bin"
