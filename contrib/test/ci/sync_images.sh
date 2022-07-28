#!/usr/bin/env bash

set -eou pipefail

function error() {
    echo "$@"
    exit 1
}

[ -z "${DESTINATION_REPO}" ] && error "\$DESTINATION_REPO required"

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)

skopeo sync \
    --src yaml \
    --dest docker \
    "${SCRIPT_DIR}"/critest_images.yml \
    "${DESTINATION_REPO}"
