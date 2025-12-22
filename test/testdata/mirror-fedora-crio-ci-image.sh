#!/usr/bin/env bash
set -euo pipefail

SOURCE_IMAGE="quay.io/crio/fedora-crio-ci:latest"
TARGET_IMAGE="quay.io/crio/fedora-crio-ci-mirror:latest"

echo "Pulling $SOURCE_IMAGE..."
skopeo copy --all "docker://$SOURCE_IMAGE" "docker://$TARGET_IMAGE"

echo "Successfully mirrored $SOURCE_IMAGE to $TARGET_IMAGE"
