#!/usr/bin/env bash
# Generate test/ci/bats-coverage-map.json, the input for hack/ci/select-bats.sh.
#
# Every bats file below test/ is executed on its own with a dedicated
# GOCOVERDIR. The Go packages which show up with a non zero execution count in
# the resulting coverage profile are recorded for that file.
#
# Requirements are the same as for test/test_runner.sh (Linux, root, the
# runtime dependencies installed) plus binaries built with coverage
# instrumentation:
#
#   GO_BUILDFLAGS=-cover make all test-binaries
#   sudo -E hack/ci/generate-bats-coverage-map.sh
#
# Usage: generate-bats-coverage-map.sh [OUTPUT] [BATS FILE...]
set -euo pipefail

# shellcheck source=hack/ci/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

root=$(ci::git rev-parse --show-toplevel)
cd "$root"

OUT=${1:-test/ci/bats-coverage-map.json}
shift || true
if [[ $# -gt 0 ]]; then
    BATS_FILES=("$@")
else
    mapfile -t BATS_FILES < <(find test -maxdepth 1 -name '*.bats' -exec basename {} \; | grep -v '^critest\.bats$' | sort)
fi

for tool in jq go bats; do
    command -v "$tool" >/dev/null || {
        echo "$tool is required" >&2
        exit 1
    }
done

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/parts"

# Never select tests while generating the map.
export CRIO_TEST_SELECTION=0
export GOCOVERDIR

incomplete=()
for name in "${BATS_FILES[@]}"; do
    name=$(basename "$name")
    dir="$WORK/${name%.bats}"
    mkdir -p "$dir"
    echo "=== $name" >&2

    GOCOVERDIR=$dir
    if ! test/test_runner.sh "$name" >"$dir.log" 2>&1; then
        echo "!!! $name failed, its coverage will be incomplete (see $dir.log)" >&2
        tail -n 50 "$dir.log" >&2
        incomplete+=("$name")
    fi

    : >"$dir.pkgs"
    if compgen -G "$dir/covmeta.*" >/dev/null; then
        go tool covdata textfmt -i="$dir" -o "$dir.txt"
        # Lines look like: <import path>/<file>.go:<start>,<end> <statements> <count>
        awk -F'[: ]' 'NR > 1 && $NF > 0 { sub(/\/[^\/]+$/, "", $1); print $1 }' "$dir.txt" | sort -u >"$dir.pkgs"
    fi
    echo "    $(wc -l <"$dir.pkgs") package(s) covered" >&2

    jq -R . "$dir.pkgs" | jq -s --arg name "$name" '{($name): .}' >"$WORK/parts/$name.json"
done

mkdir -p "$(dirname "$OUT")"
jq -n \
    --arg generated "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg commit "$(ci::git rev-parse HEAD)" \
    --argjson incomplete "$(printf '%s\n' "${incomplete[@]}" | jq -R . | jq -s 'map(select(. != ""))')" \
    --slurpfile parts <(jq -s 'add // {}' "$WORK"/parts/*.json) \
    '{
        version: 1,
        generated: $generated,
        commit: $commit,
        incomplete: $incomplete,
        files: ($parts[0] | to_entries | sort_by(.key) | from_entries)
    }' >"$OUT"

echo "wrote $OUT ($(jq '.files | length' "$OUT") bats files, ${#incomplete[@]} incomplete)" >&2
