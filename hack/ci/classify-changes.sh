#!/usr/bin/env bash
# Classify the files changed by a pull request so that GitHub Actions jobs can
# be skipped when they cannot be affected by the change.
#
# Prints key=value pairs suitable for appending to $GITHUB_OUTPUT. Every key is
# "true" when the corresponding jobs have to run. When the changed files cannot
# be determined (or CI_FORCE_FULL is set) every key is "true".
#
# Keys:
#   full      - selection disabled, everything runs
#   docs_only - the change touches documentation-like files only
#   code      - anything but documentation changed (build, unit, integration,
#               static builds, security checks, ...)
#   go        - Go sources or the Go toolchain configuration changed (lint)
#   markdown  - markdown files or their lint configuration changed
#   shell     - shell scripts or bats files changed
#   vendor    - go.mod, go.sum or vendor/ changed
#   docs      - anything below docs/ changed (generated docs validation)
#   config    - the crio.conf template or its sources changed
#
# A summary is appended to $GITHUB_STEP_SUMMARY when it is set.
set -euo pipefail

# shellcheck source=hack/ci/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

declare -A out=(
    [full]=false
    [docs_only]=false
    [code]=false
    [go]=false
    [markdown]=false
    [shell]=false
    [vendor]=false
    [docs]=false
    [config]=false
)

set_all() {
    for key in "${!out[@]}"; do
        out[$key]=$1
    done
}

reason=""
CI_CHANGED_FILES=()
if ci::force_full; then
    reason="full run forced (CI_FORCE_FULL=${CI_FORCE_FULL:-})"
elif ! ci::load_changed_files; then
    reason="unable to determine the changed files"
elif [[ ${#CI_CHANGED_FILES[@]} -eq 0 ]]; then
    reason="no changed files detected"
fi

if [[ -n $reason ]]; then
    set_all true
    out[full]=true
    out[docs_only]=false
else
    out[docs_only]=true
    for file in "${CI_CHANGED_FILES[@]}"; do
        if ! ci::is_docs_only_file "$file"; then
            out[docs_only]=false
            out[code]=true
        fi
        if ci::is_full_run_file "$file"; then
            out[go]=true
            out[shell]=true
            out[vendor]=true
            out[docs]=true
            out[config]=true
        fi
        [[ $file =~ \.go$|^(go\.mod|go\.sum)$|^vendor/|^\.golangci ]] && out[go]=true
        [[ $file =~ \.md$|^\.markdownlint|^\.mdtoc|^\.typos\.toml$ ]] && out[markdown]=true
        [[ $file =~ \.(sh|bash|bats)$|^(scripts|hack|contrib|test|completions)/|^Makefile$ ]] && out[shell]=true
        [[ $file =~ ^(go\.mod|go\.sum)$|^vendor/ ]] && out[vendor]=true
        [[ $file =~ ^docs/|^completions/|^cmd/|^internal/criocli/|^pkg/config/|^internal/config/ ]] && out[docs]=true
        [[ $file =~ ^crio\.conf$|^internal/config/|^pkg/config/|^internal/criocli/|^docs/crio\.conf ]] && out[config]=true
    done
    # Generated docs and completions are produced from the binary.
    if [[ ${out[go]} == true ]]; then
        out[docs]=true
        out[config]=true
    fi
    reason="${#CI_CHANGED_FILES[@]} changed file(s)"
fi

for key in full docs_only code go markdown shell vendor docs config; do
    echo "$key=${out[$key]}"
done

{
    echo "### Change classification"
    echo
    echo "$reason"
    echo
    echo "| Key | Value |"
    echo "| --- | --- |"
    for key in full docs_only code go markdown shell vendor docs config; do
        echo "| $key | ${out[$key]} |"
    done
    if [[ ${#CI_CHANGED_FILES[@]} -gt 0 ]]; then
        echo
        echo "<details><summary>Changed files</summary>"
        echo
        # shellcheck disable=SC2016 # markdown code span, not a shell expansion
        printf -- '- `%s`\n' "${CI_CHANGED_FILES[@]}"
        echo
        echo "</details>"
    fi
} >>"${GITHUB_STEP_SUMMARY:-/dev/null}"

ci::notice "$reason"
