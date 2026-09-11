#!/usr/bin/env bash
# Run a command that only works on Linux.
#
# On Linux the command is executed directly. On other operating systems it runs
# inside a container based on the official Go image matching go.mod. The
# container runtime is podman if it is installed and running, else docker; set
# CONTAINER_RUNTIME to force one. The repository is mounted at the same path
# and the Go caches are kept in named volumes so that repeated runs are fast.
set -euo pipefail

if [[ $# -eq 0 ]]; then
    echo "usage: $0 COMMAND [ARGS...]" >&2
    exit 2
fi

if [[ $(uname -s) == Linux ]]; then
    exec "$@"
fi

runtime_is_usable() {
    command -v "$1" >/dev/null 2>&1 && "$1" info >/dev/null 2>&1
}

RUNTIME=${CONTAINER_RUNTIME:-}
if [[ -n $RUNTIME ]]; then
    if ! runtime_is_usable "$RUNTIME"; then
        echo "error: CONTAINER_RUNTIME=$RUNTIME is not installed or not running" >&2
        exit 1
    fi
else
    for candidate in podman docker; do
        if runtime_is_usable "$candidate"; then
            RUNTIME=$candidate
            break
        fi
    done
    if [[ -z $RUNTIME ]]; then
        echo "error: '$*' needs Linux; install and start podman or docker to run it in a container on $(uname -s)" >&2
        exit 1
    fi
fi

REPO=$(git rev-parse --show-toplevel)
GO_VERSION=$(sed -n 's/^go //p' "$REPO/go.mod")
IMAGE=${CRIO_LINUX_IMAGE:-docker.io/library/golang:$GO_VERSION}

echo "Running '$*' in a $RUNTIME container ($IMAGE)" >&2
exec "$RUNTIME" run --rm \
    -v "$REPO:$REPO" \
    -v crio-go-build-cache:/root/.cache/go-build \
    -v crio-go-mod-cache:/go/pkg/mod \
    -w "$REPO" \
    -e GOFLAGS=-mod=vendor \
    "$IMAGE" "$@"
