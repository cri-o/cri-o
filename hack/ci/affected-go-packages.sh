#!/usr/bin/env bash
# Print the Go packages whose unit tests can be affected by the change under
# test, one per line, as paths relative to the repository root (./internal/oci).
#
# A package is affected when it, or any of its (test) dependencies inside this
# module, contains a changed file. Test fixtures below testdata/ directories
# count as part of the package containing them.
#
# Prints the single word "ALL" when everything has to be tested, for example
# when go.mod, vendor/ or the build scripts changed or when the changed files
# cannot be determined. Prints nothing when no unit test package is affected.
#
# Environment:
#   GO_LIST_TAGS          - build tags passed to go list, must match the tags
#                           used to build the tests (use `make affected-go-packages`)
#   GINKGO_SKIP_PACKAGES  - comma separated package dirs to never return
set -euo pipefail

# shellcheck source=hack/ci/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

root=$(ci::git rev-parse --show-toplevel)
cd "$root"

if ci::force_full; then
    ci::notice "unit tests: full run forced"
    echo ALL
    exit 0
fi

if ! ci::load_changed_files; then
    ci::notice "unit tests: unable to determine changed files, running everything"
    echo ALL
    exit 0
fi

module=$(go list -m)

# Files which are known to be irrelevant for the unit tests.
IGNORE_REGEX='^(contrib|completions|tutorials)/|^test/[^/]+\.(bats|bash|sh)$|^\.(typos\.toml|markdownlint|mdtoc|prettier|editorconfig|codecov)'

declare -A changed_pkgs=()
# Packages where only _test.go files changed: nothing else compiles them, so
# only their own tests are affected.
declare -A changed_test_pkgs=()
for file in "${CI_CHANGED_FILES[@]}"; do
    if ci::is_full_run_file "$file" || [[ $file =~ ^test/testdata/ ]]; then
        ci::notice "unit tests: $file requires a full run"
        echo ALL
        exit 0
    fi
    if ci::is_docs_only_file "$file" || [[ $file =~ $IGNORE_REGEX ]]; then
        continue
    fi
    if ci::is_go_package_file "$file"; then
        if [[ $file =~ _test\.go$ ]]; then
            changed_test_pkgs[$(ci::go_package_of_file "$file" "$module")]=1
        else
            changed_pkgs[$(ci::go_package_of_file "$file" "$module")]=1
        fi
        continue
    fi
    # Unknown file type: be conservative.
    ci::notice "unit tests: unable to map $file to a package, running everything"
    echo ALL
    exit 0
done

if [[ ${#changed_pkgs[@]} -eq 0 && ${#changed_test_pkgs[@]} -eq 0 ]]; then
    ci::notice "unit tests: no Go package changed"
    exit 0
fi

skip_regex='^$'
if [[ -n ${GINKGO_SKIP_PACKAGES:-} ]]; then
    skip_regex="^${module}/(${GINKGO_SKIP_PACKAGES//,/|})$"
fi

# One go list invocation yields the transitive dependencies of every test
# binary (<pkg>.test) in the module. The build tags have to match the ones
# used by `make testunit`, otherwise packages fail to load; `make
# affected-go-packages` passes them.
tags=${GO_LIST_TAGS:-test containers_image_ostree_stub containers_image_openpgp}
deps_file=$(mktemp)
trap 'rm -f "$deps_file" "$deps_file.err"' EXIT
if ! go list -test -tags "$tags" -f '{{.ImportPath}} {{join .Deps " "}}' ./... >"$deps_file" 2>"$deps_file.err"; then
    ci::notice "unit tests: go list failed, running everything: $(head -n 3 "$deps_file.err" | tr '\n' ' ')"
    echo ALL
    exit 0
fi

affected=()
while IFS= read -r line; do
    pkg=${line%% *}
    deps=" ${line#* } "
    [[ $pkg == *.test ]] || continue
    pkg=${pkg%.test}
    [[ $pkg =~ $skip_regex ]] && continue
    if [[ -n ${changed_test_pkgs[$pkg]:-} ]]; then
        affected+=("$pkg")
        continue
    fi
    for changed in "${!changed_pkgs[@]}"; do
        if [[ $pkg == "$changed" || $deps == *" $changed "* ]]; then
            affected+=("$pkg")
            break
        fi
    done
done <"$deps_file"

if [[ ${#affected[@]} -eq 0 ]]; then
    ci::notice "unit tests: no unit test package depends on the changed packages (${!changed_pkgs[*]})"
    exit 0
fi

ci::notice "unit tests: ${#affected[@]} package(s) affected by changes in ${!changed_pkgs[*]} ${!changed_test_pkgs[*]}"
for pkg in "${affected[@]}"; do
    if [[ $pkg == "$module" ]]; then
        echo "."
    else
        echo "./${pkg#"$module"/}"
    fi
done | sort -u
