#!/usr/bin/env bash

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
hook=$repo_root/.githooks/pre-push
base_tmp=${TMPDIR:-/tmp}
test_root=$(mktemp -d "$base_tmp/crio-pre-push-test.XXXXXX")
hook_tmp=$test_root/hook-tmp
original_path=$PATH

cleanup() {
    rm -rf "$test_root"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

fail() {
    echo "pre-push test failed: $*" >&2
    exit 1
}

assert_contains() {
    local expected=$1
    local file=$2

    grep -Fq "$expected" "$file" || fail "expected '$expected' in $file"
}

assert_not_contains() {
    local unexpected=$1
    local file=$2

    if grep -Fq "$unexpected" "$file"; then
        fail "did not expect '$unexpected' in $file"
    fi
}

assert_count() {
    local expected=$1
    local text=$2
    local file=$3
    local actual

    actual=$(grep -Fxc "$text" "$file" || true)
    [[ $actual == "$expected" ]] || fail "expected $expected '$text' lines in $file, found $actual"
}

directory_is_empty() {
    local directory=$1
    local entry

    for entry in "$directory"/* "$directory"/.[!.]* "$directory"/..?*; do
        if [[ -e $entry || -L $entry ]]; then
            return 1
        fi
    done
    return 0
}

assert_hook_cleanup() {
    local repo=$1
    local worktree_count

    if ! directory_is_empty "$hook_tmp"; then
        fail "pre-push temporary data remains in $hook_tmp"
    fi

    worktree_count=$(git -C "$repo" worktree list --porcelain | awk '$1 == "worktree" { count++ } END { print count + 0 }')
    [[ $worktree_count == 1 ]] || fail "a disposable worktree remains registered in $repo"
}

init_repo() {
    local repo=$1

    mkdir -p "$repo"
    git init -q "$repo"
}

write_check_fixture() {
    local repo=$1
    local marker=$2

    mkdir -p "$repo/hack"
    cat >"$repo/Makefile" <<'EOF'
BUILD_PATH ?= $(CURDIR)/build

.PHONY: lint check-vendor docs-validation
check-vendor:
	@test -z "$$(git status --porcelain --untracked-files=all)"
	@! test -e "$(BUILD_PATH)"
	@printf 'vendor:%s\n' "$$(cat expected-marker)" >>"$(TEST_LOG)"

lint:
	@test "$(BUILD_PATH)" = "$(CURDIR)/build"
	@! test -e "$(BUILD_PATH)/lint.marker"
	@mkdir -p "$(BUILD_PATH)"
	@./fake-lint >"$(BUILD_PATH)/lint.marker"
	@cmp -s "$(BUILD_PATH)/lint.marker" expected-marker
	@printf 'lint:%s\n' "$$(cat "$(BUILD_PATH)/lint.marker")" >>"$(TEST_LOG)"

docs-validation:
	@printf 'docs:%s\n' "$$(cat expected-marker)" >>"$(TEST_LOG)"
EOF
    printf '%s\n' "$marker" >"$repo/expected-marker"
    cat >"$repo/fake-lint" <<EOF
#!/usr/bin/env bash
printf '%s\\n' '$marker'
EOF
    chmod +x "$repo/fake-lint"
}

write_helper() {
    local repo=$1

    mkdir -p "$repo/hack"
    cat >"$repo/hack/run-on-linux.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'helper:%s\n' "$(cat expected-marker)" >>"$TEST_LOG"
"$@"
EOF
    chmod +x "$repo/hack/run-on-linux.sh"
}

commit_all() {
    local repo=$1
    local subject=$2

    git -C "$repo" add .
    git -C "$repo" commit -q -m "$subject"
    git -C "$repo" rev-parse HEAD
}

write_push_record() {
    local file=$1
    local source_ref=$2
    local source_oid=$3
    local destination_ref=$4

    printf '%s %s %s %040d\n' "$source_ref" "$source_oid" "$destination_ref" 0 >>"$file"
}

run_hook() {
    local repo=$1
    local input=$2
    local output=$3

    (
        cd "$repo"
        "$hook" origin <"$input"
    ) >"$output.stdout" 2>"$output.stderr"
}

mkdir -p "$hook_tmp" "$test_root/home" "$test_root/xdg"
export TMPDIR=$hook_tmp
export HOME=$test_root/home
export XDG_CONFIG_HOME=$test_root/xdg
export GIT_CONFIG_NOSYSTEM=1
export GIT_CONFIG_GLOBAL=$test_root/gitconfig
export GIT_CONFIG_COUNT=0
unset GIT_CONFIG_PARAMETERS GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX
: >"$GIT_CONFIG_GLOBAL"
git config --global user.name "Git Hook Test"
git config --global user.email "git-hook@example.com"

# Mixed input validates source-ref notes filtering, commit peeling,
# deletion and non-commit skipping, unique-tip deduplication, and tip-local
# build/helper use. The tag commits are not present in branch records so their
# markers are meaningful controls for lightweight and annotated tag handling.
mixed_repo=$test_root/mixed
mixed_log=$test_root/mixed.log
mixed_input=$test_root/mixed.input
mixed_output=$test_root/mixed-output
active_helper_log=$test_root/active-helper.log
init_repo "$mixed_repo"
write_check_fixture "$mixed_repo" tip-one
write_helper "$mixed_repo"
tip_one=$(commit_all "$mixed_repo" "tip one")
write_check_fixture "$mixed_repo" tip-two
write_helper "$mixed_repo"
tip_two=$(commit_all "$mixed_repo" "tip two")
write_check_fixture "$mixed_repo" lightweight-tag-only
write_helper "$mixed_repo"
lightweight_tip=$(commit_all "$mixed_repo" "lightweight tag-only tip")
git -C "$mixed_repo" tag light-only "$lightweight_tip"
write_check_fixture "$mixed_repo" annotated-tag-only
write_helper "$mixed_repo"
annotated_tip=$(commit_all "$mixed_repo" "annotated tag-only tip")
git -C "$mixed_repo" tag -a -m "annotated tag-only tip" annotated-only "$annotated_tip"
annotated_object=$(git -C "$mixed_repo" rev-parse annotated-only)
write_check_fixture "$mixed_repo" notes-source
write_helper "$mixed_repo"
notes_tip=$(commit_all "$mixed_repo" "notes source tip")
blob_oid=$(printf 'not a commit\n' | git -C "$mixed_repo" hash-object -w --stdin)
cat >"$mixed_repo/hack/run-on-linux.sh" <<EOF
#!/usr/bin/env bash
printf 'active helper ran\\n' >>'$active_helper_log'
exit 97
EOF
chmod +x "$mixed_repo/hack/run-on-linux.sh"
active_helper_diff=$(git -C "$mixed_repo" diff --binary -- hack/run-on-linux.sh)
: >"$mixed_input"
write_push_record "$mixed_input" refs/heads/one "$tip_one" refs/heads/one
write_push_record "$mixed_input" refs/heads/duplicate "$tip_one" refs/heads/duplicate
write_push_record "$mixed_input" refs/tags/light-only "$lightweight_tip" refs/tags/light-only
write_push_record "$mixed_input" refs/tags/annotated-only "$annotated_object" refs/tags/annotated-only
write_push_record "$mixed_input" refs/heads/deleted 0000000000000000000000000000000000000000 refs/heads/deleted
write_push_record "$mixed_input" refs/tags/blob "$blob_oid" refs/tags/blob
write_push_record "$mixed_input" refs/notes/review "$notes_tip" refs/heads/from-notes
write_push_record "$mixed_input" refs/heads/to-notes "$tip_two" refs/notes/review
export TEST_LOG=$mixed_log
run_hook "$mixed_repo" "$mixed_input" "$mixed_output" || fail "mixed push input was rejected"
assert_count 1 lint:tip-one "$mixed_log"
assert_count 1 vendor:tip-one "$mixed_log"
assert_count 1 helper:tip-one "$mixed_log"
assert_count 1 docs:tip-one "$mixed_log"
assert_count 1 lint:lightweight-tag-only "$mixed_log"
assert_count 1 vendor:lightweight-tag-only "$mixed_log"
assert_count 1 helper:lightweight-tag-only "$mixed_log"
assert_count 1 docs:lightweight-tag-only "$mixed_log"
assert_count 1 lint:annotated-tag-only "$mixed_log"
assert_count 1 vendor:annotated-tag-only "$mixed_log"
assert_count 1 helper:annotated-tag-only "$mixed_log"
assert_count 1 docs:annotated-tag-only "$mixed_log"
assert_count 1 lint:tip-two "$mixed_log"
assert_count 1 vendor:tip-two "$mixed_log"
assert_count 1 helper:tip-two "$mixed_log"
assert_count 1 docs:tip-two "$mixed_log"
assert_not_contains notes-source "$mixed_log"
[[ ! -e $active_helper_log ]] || fail "the dirty active-checkout helper ran"
[[ ! -e $mixed_repo/build ]] || fail "tip checks wrote build output to the active checkout"
[[ $(git -C "$mixed_repo" diff --binary -- hack/run-on-linux.sh) == "$active_helper_diff" ]] || fail "the active checkout changed"
assert_hook_cleanup "$mixed_repo"

# A helper-less historical tip validates docs directly only on Linux. A
# present non-executable helper must fail rather than enable the fallback.
historical_repo=$test_root/historical
historical_log=$test_root/historical.log
historical_input=$test_root/historical.input
shim_dir=$test_root/uname-shim
init_repo "$historical_repo"
write_check_fixture "$historical_repo" historical
rm -f "$historical_repo/hack/run-on-linux.sh"
historical_tip=$(commit_all "$historical_repo" "historical helper-less tip")
mkdir -p "$shim_dir"
cat >"$shim_dir/uname" <<'EOF'
#!/usr/bin/env bash
[[ ${1:-} == -s ]] || exit 2
printf '%s\n' "$FAKE_UNAME"
EOF
chmod +x "$shim_dir/uname"
export PATH=$shim_dir:$original_path
: >"$historical_input"
write_push_record "$historical_input" refs/heads/historical "$historical_tip" refs/heads/historical
export TEST_LOG=$historical_log
export FAKE_UNAME=Linux
run_hook "$historical_repo" "$historical_input" "$test_root/historical-linux" || fail "helper-less Linux tip was rejected"
assert_count 1 lint:historical "$historical_log"
assert_count 1 vendor:historical "$historical_log"
assert_count 0 helper:historical "$historical_log"
assert_count 1 docs:historical "$historical_log"
assert_hook_cleanup "$historical_repo"

: >"$historical_log"
export FAKE_UNAME=Darwin
if run_hook "$historical_repo" "$historical_input" "$test_root/historical-darwin"; then
    fail "helper-less non-Linux tip was accepted"
fi
assert_count 1 lint:historical "$historical_log"
assert_count 1 vendor:historical "$historical_log"
assert_count 0 helper:historical "$historical_log"
assert_count 0 docs:historical "$historical_log"
assert_contains "$historical_tip" "$test_root/historical-darwin.stderr"
assert_contains "pushed tip lacks hack/run-on-linux.sh and requires Linux" "$test_root/historical-darwin.stderr"
assert_hook_cleanup "$historical_repo"

write_helper "$historical_repo"
chmod -x "$historical_repo/hack/run-on-linux.sh"
nonexec_tip=$(commit_all "$historical_repo" "non-executable helper")
: >"$historical_input"
write_push_record "$historical_input" refs/heads/nonexec "$nonexec_tip" refs/heads/nonexec
: >"$historical_log"
export FAKE_UNAME=Linux
if run_hook "$historical_repo" "$historical_input" "$test_root/historical-nonexec"; then
    fail "tip with a non-executable helper was accepted"
fi
assert_count 1 lint:historical "$historical_log"
assert_count 1 vendor:historical "$historical_log"
assert_count 0 helper:historical "$historical_log"
assert_count 0 docs:historical "$historical_log"
assert_hook_cleanup "$historical_repo"

# Failures from each check propagate, while success and every failure remove
# disposable worktrees and their temporary data.
failure_repo=$test_root/failures
failure_log=$test_root/failures.log
failure_input=$test_root/failures.input
init_repo "$failure_repo"
write_check_fixture "$failure_repo" failure
write_helper "$failure_repo"
printf 'lint\n' >"$failure_repo/fail-stage"
cat >"$failure_repo/Makefile" <<'EOF'
BUILD_PATH ?= $(CURDIR)/build

.PHONY: lint check-vendor docs-validation
lint:
	@printf 'lint:failure\n' >>"$(TEST_LOG)"
	@test "$$(cat fail-stage)" != lint
	@test "$(BUILD_PATH)" = "$(CURDIR)/build"
	@mkdir -p "$(BUILD_PATH)"
	@./fake-lint >"$(BUILD_PATH)/lint.marker"

check-vendor:
	@printf 'vendor:failure\n' >>"$(TEST_LOG)"
	@test "$$(cat fail-stage)" != vendor

docs-validation:
	@printf 'docs:failure\n' >>"$(TEST_LOG)"
	@test "$$(cat fail-stage)" != docs
EOF
lint_failure=$(commit_all "$failure_repo" "lint failure")
printf 'vendor\n' >"$failure_repo/fail-stage"
vendor_failure=$(commit_all "$failure_repo" "vendor failure")
printf 'docs\n' >"$failure_repo/fail-stage"
docs_failure=$(commit_all "$failure_repo" "docs failure")
printf 'none\n' >"$failure_repo/fail-stage"
success_tip=$(commit_all "$failure_repo" "successful checks")
export TEST_LOG=$failure_log

for failure_case in "lint:$lint_failure" "vendor:$vendor_failure" "docs:$docs_failure"; do
    stage=${failure_case%%:*}
    tip=${failure_case#*:}
    : >"$failure_input"
    write_push_record "$failure_input" "refs/heads/$stage" "$tip" "refs/heads/$stage"
    : >"$failure_log"
    if run_hook "$failure_repo" "$failure_input" "$test_root/failure-$stage"; then
        fail "$stage failure did not propagate"
    fi
    assert_contains vendor:failure "$failure_log"
    if [[ $stage == vendor ]]; then
        assert_not_contains lint:failure "$failure_log"
    else
        assert_contains lint:failure "$failure_log"
    fi
    if [[ $stage == docs ]]; then
        assert_contains helper:failure "$failure_log"
        assert_contains docs:failure "$failure_log"
    else
        assert_not_contains helper:failure "$failure_log"
        assert_not_contains docs:failure "$failure_log"
    fi
    assert_hook_cleanup "$failure_repo"
done

: >"$failure_input"
write_push_record "$failure_input" refs/heads/success "$success_tip" refs/heads/success
: >"$failure_log"
run_hook "$failure_repo" "$failure_input" "$test_root/failure-success" || fail "successful checks were rejected"
assert_contains lint:failure "$failure_log"
assert_contains vendor:failure "$failure_log"
assert_contains helper:failure "$failure_log"
assert_contains docs:failure "$failure_log"
[[ ! -e $failure_repo/build ]] || fail "successful tip wrote build output to the active checkout"
assert_hook_cleanup "$failure_repo"

printf 'pre-push hook tests passed\n'
