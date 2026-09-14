#!/usr/bin/env bash
# Shared helpers for change based CI test selection (test impact analysis).
#
# The functions in this file are sourced by the scripts in hack/ci and by
# test/test_runner.sh. They only depend on git, coreutils and (optionally) jq.
#
# Every helper is designed to fail "open": whenever the set of changed files
# cannot be determined reliably, callers fall back to running everything.

# Files which never influence any build or test result. Changes limited to
# these paths allow CI to skip the expensive jobs entirely. This mirrors the
# skip_if_only_changed regular expression used for the Prow jobs in
# openshift/release.
CI_DOCS_ONLY_REGEX='^(docs|tutorials|logo)/|\.md$|^(\.gitignore|OWNERS|OWNERS_ALIASES|SECURITY_CONTACTS|PROJECT|LICENSE|\.github/CODEOWNERS|\.github/ISSUE_TEMPLATE/.*|\.github/PULL_REQUEST_TEMPLATE\.md|\.github/dependabot\.yml)$'

# Files which influence the whole build/test toolchain. Any change to them
# disables test selection and runs the full suites.
CI_FULL_RUN_REGEX='^(go\.mod|go\.sum|Makefile|flake\.nix|flake\.lock|crio\.conf|dependencies\.yaml)$|^(vendor|hack|scripts|nix|pinns|\.github/workflows)/|^\.golangci|^test/(helpers\.bash|common\.sh|test_runner\.sh|ci/)'

# Resolve the git base revision of the change under test. Prints the SHA on
# stdout and returns 1 when it cannot be determined.
#
# Sources, in order of precedence:
#   CI_BASE_SHA     - explicit override
#   PULL_BASE_SHA   - set by Prow for presubmit jobs
#   GITHUB_BASE_SHA - set by the GitHub Actions workflows from the pull_request
#                     event payload
ci::base_sha() {
    local base=${CI_BASE_SHA:-${PULL_BASE_SHA:-${GITHUB_BASE_SHA:-}}}
    if [[ -z $base ]]; then
        return 1
    fi
    if [[ -n ${JOB_TYPE:-} && ${JOB_TYPE} != presubmit && -z ${CI_BASE_SHA:-} && -z ${GITHUB_BASE_SHA:-} ]]; then
        # Prow post-submits and periodics also export PULL_BASE_SHA, but there
        # is no "change" to select tests for.
        return 1
    fi
    echo "$base"
}

# git wrapper which works when the checkout is owned by another user (the
# integration tests run via sudo) and never prompts.
ci::git() {
    GIT_TERMINAL_PROMPT=0 git -c safe.directory='*' "$@"
}

# Print the list of files changed between the base revision and HEAD, one per
# line. Returns 1 when the list cannot be computed. In that case callers must
# treat every file as changed.
ci::changed_files() {
    local base
    base=$(ci::base_sha) || return 1

    local root
    root=$(ci::git rev-parse --show-toplevel 2>/dev/null) || return 1

    if ! ci::git -C "$root" cat-file -e "${base}^{commit}" 2>/dev/null; then
        # Shallow checkouts (GitHub Actions) do not have the base commit,
        # fetch it on demand.
        ci::git -C "$root" fetch --quiet --depth=1 origin "$base" 2>/dev/null || return 1
    fi

    local merge_base
    if merge_base=$(ci::git -C "$root" merge-base "$base" HEAD 2>/dev/null) && [[ -n $merge_base ]]; then
        base=$merge_base
    fi
    # Otherwise HEAD is a merge of the change onto $base (GitHub's refs/pull/N/merge
    # or Prow's clonerefs merge), so diffing against $base directly is correct.

    ci::git -C "$root" diff --name-only --no-renames "$base" HEAD
}

# Populate the CI_CHANGED_FILES array with the output of ci::changed_files.
# Returns 1 when the list cannot be computed.
ci::load_changed_files() {
    local output
    output=$(ci::changed_files) || return 1
    # shellcheck disable=SC2034 # consumed by the callers
    CI_CHANGED_FILES=()
    if [[ -n $output ]]; then
        # shellcheck disable=SC2034
        mapfile -t CI_CHANGED_FILES <<<"$output"
    fi
    return 0
}

# Return 0 when the file matches the docs-only regular expression.
ci::is_docs_only_file() {
    [[ $1 =~ $CI_DOCS_ONLY_REGEX ]]
}

# Return 0 when the file requires a full run of every suite.
ci::is_full_run_file() {
    [[ $1 =~ $CI_FULL_RUN_REGEX ]]
}

# Return 0 when test selection is forced off. CI_FORCE_FULL accepts the values
# "1", "true" and "yes" (case insensitive).
ci::force_full() {
    case "${CI_FORCE_FULL:-}" in
    1 | [Tt][Rr][Uu][Ee] | [Yy][Ee][Ss]) return 0 ;;
    esac
    return 1
}

# Return 0 when the file lives inside a Go package of this module.
ci::is_go_package_file() {
    local file=$1
    [[ $file =~ \.go$ ]] && return 0
    local dir
    dir=$(dirname "$file")
    while [[ $dir != . && $dir != / ]]; do
        if compgen -G "$dir/*.go" >/dev/null; then
            return 0
        fi
        # Only walk up for test fixtures.
        [[ $(basename "$dir") == testdata ]] || return 1
        dir=$(dirname "$dir")
    done
    return 1
}

# Print the import path of the Go package a changed file belongs to. Test
# fixtures (testdata directories) map to the package which contains them.
ci::go_package_of_file() {
    local file=$1 module=$2
    local dir
    dir=$(dirname "$file")
    while [[ $dir == */testdata || $(basename "$dir") == testdata ]]; do
        dir=$(dirname "$dir")
    done
    if [[ $dir == . ]]; then
        echo "$module"
    else
        echo "$module/$dir"
    fi
}

# Log a message on stderr (stdout is reserved for the results of the scripts),
# additionally as a GitHub Actions notice when running there.
ci::notice() {
    if [[ -n ${GITHUB_ACTIONS:-} ]]; then
        echo "::notice title=test selection::$*" >&2
    fi
    echo "test selection: $*" >&2
}
