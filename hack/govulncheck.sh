#!/usr/bin/env bash

set -euo pipefail

# The govulncheck version should match supported Go version.
GOVULNCHECK_VERSION="v1.1.3"

# Install build time dependencies.
sudo apt-get update
sudo apt-get install -y pkg-config libgpgme-dev libbtrfs-dev libseccomp-dev btrfs-progs

# Set environment variables.
export GOGC=off
export GO111MODULE=on
export GOSUMDB="sum.golang.org"
export PKG_CONFIG_PATH="/usr/lib/x86_64-linux-gnu/pkgconfig"
GOPATH_BIN="$(go env GOPATH)"/bin
export PATH="${PATH}:${GOPATH_BIN}"

# Install govulncheck.
go install golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}

# Generate the report.
report=$(mktemp)
trap 'rm "$report"' EXIT
"$GOPATH_BIN"/govulncheck -json -tags=test,exclude_graphdriver_devicemapper ./... >"$report"

# Parse vulnerabilities from the report.
modvulns=$(jq -Sr '.vulnerability.modules[]? | select(.path != "stdlib") | [.path, "affected package(s): \(.packages[].path)", "found version: \(.found_version)", "fixed version: \(.fixed_version)"]' <"$report")
libvulns=$(jq -Sr '.vulnerability.modules[]? | select(.path == "stdlib") | [.path, "affected package(s): \(.packages[].path)", "found version: \(.found_version)", "fixed version: \(.fixed_version)"]' <"$report")

# Print vulnerabilities information, if any.
echo "$modvulns"
echo "$libvulns"

# Exit with non-zero status if there were any vulnerabilities detected in module dependencies.
if [[ -n "$modvulns" ]]; then
    exit 1
fi
