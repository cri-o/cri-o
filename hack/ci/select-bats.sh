#!/usr/bin/env bash
# Select the bats integration test files which can be affected by the change
# under test, using the coverage map in test/ci/bats-coverage-map.json.
#
# The map records, for every bats file, the Go packages whose code was executed
# while running that file (see hack/ci/generate-bats-coverage-map.sh). A bats
# file is selected when
#   - it was added or modified itself,
#   - it is not present in the map (new tests since the map was generated), or
#   - any changed Go package appears in its coverage list.
#
# Prints the selected file names (relative to test/), one per line, or the
# single word "ALL" when the full suite has to run: no map, toolchain or test
# harness changes, changed packages unknown to the map, or when the changed
# files cannot be determined. Prints nothing when no test is affected.
set -euo pipefail

# shellcheck source=hack/ci/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

root=$(ci::git rev-parse --show-toplevel)
cd "$root"

MAP=${CRIO_BATS_COVERAGE_MAP:-test/ci/bats-coverage-map.json}

all() {
    ci::notice "integration tests: $1, running everything"
    echo ALL
    exit 0
}

if ci::force_full; then
    all "full run forced"
fi
if ! command -v jq >/dev/null; then
    all "jq is not installed"
fi
if [[ ! -f $MAP ]]; then
    all "$MAP does not exist"
fi
if ! ci::load_changed_files; then
    all "unable to determine changed files"
fi

module=$(go list -m 2>/dev/null || sed -n 's/^module //p' go.mod)

# Files which never influence the integration tests.
IGNORE_REGEX='^(contrib|completions|tutorials)/|^\.(typos\.toml|markdownlint|mdtoc|prettier|editorconfig|codecov)|_test\.go$|^test/mocks/|^test/docs-validation/'

declare -A changed_pkgs=()
declare -A selected=()
for file in "${CI_CHANGED_FILES[@]}"; do
    if ci::is_full_run_file "$file" || [[ $file =~ ^test/(testdata|checkcriu|checkseccomp|copyimg|updateunified|nri)/ ]]; then
        all "$file changed"
    fi
    if ci::is_docs_only_file "$file" || [[ $file =~ $IGNORE_REGEX ]]; then
        continue
    fi
    if [[ $file =~ ^test/([^/]+\.bats)$ ]]; then
        if [[ -f $file ]]; then
            selected[${BASH_REMATCH[1]}]=1
        fi
        continue
    fi
    if ci::is_go_package_file "$file"; then
        changed_pkgs[$(ci::go_package_of_file "$file" "$module")]=1
        continue
    fi
    all "unable to map $file to a package"
done

# Packages known to the map. A changed package which never showed up in any
# coverage profile means the map is stale (or the package is new), so run
# everything to be safe.
mapfile -t known_pkgs < <(jq -r '.files[] | .[]' "$MAP" | sort -u)
declare -A known=()
for pkg in "${known_pkgs[@]}"; do
    known[$pkg]=1
done
for pkg in "${!changed_pkgs[@]}"; do
    if [[ -z ${known[$pkg]:-} ]]; then
        all "package $pkg is not covered by the map"
    fi
done

# Bats files which exist but are missing from the map always run.
for path in test/*.bats; do
    name=$(basename "$path")
    if ! jq -e --arg f "$name" '.files[$f]' "$MAP" >/dev/null; then
        selected[$name]=1
    fi
done

# Files whose coverage intersects the changed packages.
if [[ ${#changed_pkgs[@]} -gt 0 ]]; then
    while IFS= read -r name; do
        selected[$name]=1
    done < <(jq -r --argjson pkgs "$(printf '%s\n' "${!changed_pkgs[@]}" | jq -R . | jq -s .)" \
        '.files | to_entries[] | select(any(.value[]; . as $p | $pkgs | index($p))) | .key' "$MAP")
fi

# critest is driven by RUN_CRITEST, never by selection.
unset 'selected[critest.bats]'

if [[ ${#selected[@]} -eq 0 ]]; then
    ci::notice "integration tests: no bats file is affected by the change"
    exit 0
fi

total=$(find test -maxdepth 1 -name '*.bats' | wc -l | tr -d ' ')
ci::notice "integration tests: ${#selected[@]} of $total bats files selected"
printf '%s\n' "${!selected[@]}" | sort
