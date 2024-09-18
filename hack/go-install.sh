#!/usr/bin/env bash

set -euo pipefail

DEST_DIR=${1:-}
CMD=${2:-}
GO_INSTALL_LOCATION=${3:-}

if [[ -z "$DEST_DIR" || -z "$CMD" || -z "$GO_INSTALL_LOCATION" ]]; then
    echo "Usage: $0 DEST_DIR CMD GO_INSTALL_LOCATION"
    exit 1
fi

CMD_PATH=$(command -v "$CMD" || true)

if [ -z "$CMD_PATH" ]; then
    echo "Installing $CMD from: $GO_INSTALL_LOCATION"
    GOBIN="$DEST_DIR" go install "$GO_INSTALL_LOCATION"
else
    echo "Using existing $CMD from: $CMD_PATH"
    mkdir -p "$DEST_DIR"
    cp "$CMD_PATH" "$DEST_DIR"
fi
