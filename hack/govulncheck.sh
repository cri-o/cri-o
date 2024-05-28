#!/bin/bash

set -euo pipefail

# Install dependencies
sudo apt-get update
sudo apt-get install -y pkg-config libgpgme-dev libbtrfs-dev libseccomp-dev btrfs-progs

# Set environment variables
export GO111MODULE=on
export GOSUMDB=sum.golang.org
export PKG_CONFIG_PATH=/usr/lib/x86_64-linux-gnu/pkgconfig
GOPATH_BIN=$(go env GOPATH)/bin
export PATH="$PATH:$GOPATH_BIN"

go install golang.org/x/vuln/cmd/govulncheck@latest

# Generate report
report=$(mktemp)
trap 'rm "$report"' EXIT
"$GOPATH_BIN"/govulncheck -json -tags=test,exclude_graphdriver_devicemapper ./... >"$report"

# Parse vulnerabilities from report
modvulns=$(jq -Sr '.vulnerability.modules[]? | select(.path != "stdlib") | [.path, "affected package(s): \(.packages[].path)", "found version: \(.found_version)", "fixed version: \(.fixed_version)"]' <"$report")
libvulns=$(jq -Sr '.vulnerability.modules[]? | select(.path == "stdlib") | [.path, "affected package(s): \(.packages[].path)", "found version: \(.found_version)", "fixed version: \(.fixed_version)"]' <"$report")

# Print vulnerabilities
echo "$modvulns"
echo "$libvulns"

# Exit with non-zero status if there are any vulnerabilities in module dependencies
if [[ -n "$modvulns" ]]; then
    exit 1
fi
