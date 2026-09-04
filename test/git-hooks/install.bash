#!/usr/bin/env bash

set -euo pipefail

project_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
real_git=$(command -v git)
readonly project_root real_git
readonly installer=$project_root/hack/install-git-hooks.sh
readonly original_path=$PATH

test_root=$(mktemp -d "${TMPDIR:-/tmp}/crio-git-hooks-install-test.XXXXXX")
cleanup() {
    status=$?
    trap - EXIT INT TERM
    rm -rf "$test_root"
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

export GIT_CONFIG_NOSYSTEM=1
export GIT_CONFIG_GLOBAL=$test_root/global.gitconfig
export HOME=$test_root/home
export GIT_AUTHOR_NAME='Hook Test'
export GIT_AUTHOR_EMAIL=hook-test@example.com
export GIT_COMMITTER_NAME=$GIT_AUTHOR_NAME
export GIT_COMMITTER_EMAIL=$GIT_AUTHOR_EMAIL
export TMPDIR=$test_root/tmp
mkdir -p "$HOME" "$TMPDIR"
unset GIT_CONFIG_COUNT GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR

fail() {
    echo "not ok - $*" >&2
    exit 1
}

pass() {
    echo "ok - $*"
}

assert_equal() {
    expected=$1
    actual=$2
    description=$3
    if [[ $actual != "$expected" ]]; then
        printf "not ok - %s\nexpected: <%s>\nactual:   <%s>\n" "$description" "$expected" "$actual" >&2
        exit 1
    fi
}

new_repo() {
    repo=$test_root/$1
    git init -q "$repo"
}

new_committed_repo() {
    new_repo "$1"
    printf 'initial\n' >"$repo/README"
    git -C "$repo" add README
    git -C "$repo" commit -qm initial
}

worktree_config_path() {
    target_repo=$1
    target_git_dir=$(git -C "$target_repo" rev-parse --git-dir)
    case $target_git_dir in
    /*) ;;
    *) target_git_dir=$target_repo/$target_git_dir ;;
    esac
    printf '%s/config.worktree\n' "$target_git_dir"
}

assert_config_value() {
    expected=$1
    target_repo=$2
    scope=$3
    name=$4
    description="$scope $name"

    case $scope in
    local)
        actual=$(git -C "$target_repo" config --local --get-all "$name") || fail "$description is unset"
        ;;
    worktree)
        config_file=$(worktree_config_path "$target_repo")
        actual=$(git config --file "$config_file" --get-all "$name") || fail "$description is unset"
        ;;
    effective)
        actual=$(git -C "$target_repo" config --get "$name") || fail "$description is unset"
        ;;
    *) fail "unknown configuration scope $scope" ;;
    esac
    assert_equal "$expected" "$actual" "$description"
}

assert_config_unset() {
    target_repo=$1
    scope=$2
    name=$3

    case $scope in
    local)
        if git -C "$target_repo" config --local --get-all "$name" >/dev/null 2>&1; then
            fail "$scope $name should be unset"
        fi
        ;;
    worktree)
        config_file=$(worktree_config_path "$target_repo")
        if git config --file "$config_file" --get-all "$name" >/dev/null 2>&1; then
            fail "$scope $name should be unset"
        fi
        ;;
    effective)
        if git -C "$target_repo" config --get "$name" >/dev/null 2>&1; then
            fail "$scope $name should be unset"
        fi
        ;;
    *) fail "unknown configuration scope $scope" ;;
    esac
}

relevant_state() {
    target_repo=$1
    config_file=$(worktree_config_path "$target_repo")
    printf '%s\n' 'local hooks:'
    git -C "$target_repo" config --local --get-all core.hooksPath 2>/dev/null || true
    printf '%s\n' 'worktree hooks:'
    git config --file "$config_file" --get-all core.hooksPath 2>/dev/null || true
    printf '%s\n' 'extensions:'
    git -C "$target_repo" config --local --get-all extensions.worktreeConfig 2>/dev/null || true
    printf '%s\n' 'effective hooks:'
    git -C "$target_repo" config --get core.hooksPath 2>/dev/null || true
    printf '%s\n' 'end'
}

run_install() {
    target_repo=$1
    (cd "$target_repo" && "$installer") >/dev/null
}

expect_install_failure() {
    target_repo=$1
    expected_message=$2
    if failure_output=$(cd "$target_repo" && "$installer" 2>&1); then
        fail "installer unexpectedly succeeded in $target_repo"
    fi
    case $failure_output in
    *"$expected_message"*) ;;
    *) fail "installer failure did not contain '$expected_message': $failure_output" ;;
    esac
}

assert_refusal_preserves_state() {
    target_repo=$1
    expected_message=$2
    before=$(relevant_state "$target_repo")
    expect_install_failure "$target_repo" "$expected_message"
    after=$(relevant_state "$target_repo")
    assert_equal "$before" "$after" "refusal preserves configuration"
}

make_git_shim() {
    shim_dir=$test_root/git-shim
    mkdir -p "$shim_dir"
    cat >"$shim_dir/git" <<'EOF'
#!/usr/bin/env bash

set -u

is_mutation=false
is_effective_hooks_query=false
if [[ ${1-} == config ]]; then
    if [[ $# -eq 3 && $2 == --get && $3 == core.hooksPath ]]; then
        is_effective_hooks_query=true
    fi
    for arg in "$@"; do
        case $arg in
            --add | --replace-all | --unset-all)
                is_mutation=true
                ;;
        esac
    done
fi

if [[ $is_effective_hooks_query == true && -n ${GIT_EFFECTIVE_AFTER_MUTATION-} && -e $GIT_SHIM_STATE_DIR/mutated ]]; then
    printf '%s\n' "$GIT_EFFECTIVE_AFTER_MUTATION"
    exit 0
fi

status=0
"$REAL_GIT" "$@" || status=$?

if [[ $is_mutation == true && $status -eq 0 ]]; then
    : >"$GIT_SHIM_STATE_DIR/mutated"
    if [[ -n ${GIT_MUTATION_LOG-} ]]; then
        effective=$("$REAL_GIT" config --get core.hooksPath 2>/dev/null || printf '<unset>')
        printf '%s\n' "$effective" >>"$GIT_MUTATION_LOG"
    fi

    if [[ -n ${GIT_FAIL_MUTATION_AT-} && ! -e $GIT_SHIM_STATE_DIR/failed ]]; then
        count=0
        if [[ -f $GIT_SHIM_STATE_DIR/count ]]; then
            IFS= read -r count <"$GIT_SHIM_STATE_DIR/count"
        fi
        count=$((count + 1))
        printf '%s\n' "$count" >"$GIT_SHIM_STATE_DIR/count"
        if [[ $count -eq $GIT_FAIL_MUTATION_AT ]]; then
            : >"$GIT_SHIM_STATE_DIR/failed"
            exit 97
        fi
    fi
fi

exit "$status"
EOF
    chmod +x "$shim_dir/git"
}

# A fresh installation is scoped to only the invoking worktree.
new_committed_repo fresh-linked
main_repo=$repo
linked_repo=$test_root/fresh-linked-sibling
git -C "$main_repo" worktree add -q -b hook-test-linked "$linked_repo"
run_install "$linked_repo"
assert_config_value true "$main_repo" local extensions.worktreeConfig
assert_config_unset "$main_repo" local core.hooksPath
assert_config_value .githooks "$linked_repo" worktree core.hooksPath
assert_config_unset "$main_repo" worktree core.hooksPath
assert_config_unset "$main_repo" effective core.hooksPath
run_install "$main_repo"
assert_config_value .githooks "$main_repo" worktree core.hooksPath
assert_config_value .githooks "$main_repo" effective core.hooksPath
before=$(relevant_state "$linked_repo")
run_install "$linked_repo"
after=$(relevant_state "$linked_repo")
assert_equal "$before" "$after" "repeat installation is a configuration no-op"
pass "fresh and repeat installations are worktree-scoped"

# A dormant but valid raw worktree value is retained when the extension is
# enabled, rather than being mistaken for an unset or shared value.
new_repo dormant-worktree
raw_config=$(worktree_config_path "$repo")
git config --file "$raw_config" core.hooksPath .githooks
run_install "$repo"
assert_config_value true "$repo" local extensions.worktreeConfig
assert_config_unset "$repo" local core.hooksPath
assert_config_value .githooks "$repo" worktree core.hooksPath
assert_config_value .githooks "$repo" effective core.hooksPath
pass "raw worktree configuration is honored while the extension is disabled"

# Enabling worktree configuration must not activate any unrelated setting in
# the invoking worktree's otherwise valid dormant hook configuration.
new_repo dormant-current-extra
raw_config=$(worktree_config_path "$repo")
git config --file "$raw_config" core.hooksPath .githooks
git config --file "$raw_config" user.email dormant-current@example.com
before=$(relevant_state "$repo")
before_shared=$(cat "$repo/.git/config")
before_raw=$(cat "$raw_config")
expect_install_failure "$repo" "current worktree has dormant configuration beyond core.hooksPath=.githooks"
after=$(relevant_state "$repo")
assert_equal "$before" "$after" "dormant current-worktree refusal preserves configuration"
assert_equal "$before_shared" "$(cat "$repo/.git/config")" "dormant current-worktree refusal preserves shared configuration exactly"
assert_equal "$before_raw" "$(cat "$raw_config")" "dormant current-worktree refusal preserves worktree configuration exactly"
assert_config_unset "$repo" local extensions.worktreeConfig
assert_config_unset "$repo" effective core.hooksPath
pass "unrelated dormant current-worktree configuration is not activated"

# Includes are expanded before mutation, so enabling worktree configuration
# cannot reveal a later conflicting value.
new_repo included-worktree-conflict
raw_config=$(worktree_config_path "$repo")
included_config=$repo/included-worktree.config
git config --file "$raw_config" core.hooksPath .githooks
git config --file "$raw_config" include.path "$included_config"
git config --file "$included_config" core.hooksPath custom-included
before=$(relevant_state "$repo")
before_raw=$(cat "$raw_config")
before_included=$(cat "$included_config")
expect_install_failure "$repo" "current worktree core.hooksPath has multiple values"
after=$(relevant_state "$repo")
assert_equal "$before" "$after" "included worktree refusal preserves configuration"
assert_equal "$before_raw" "$(cat "$raw_config")" "included worktree refusal preserves the worktree config file"
assert_equal "$before_included" "$(cat "$included_config")" "included worktree refusal preserves the included config file"
assert_config_unset "$repo" local extensions.worktreeConfig
assert_config_unset "$repo" effective core.hooksPath
pass "included worktree conflicts are refused before mutation"

# Enabling the shared extension must not activate dormant hooks configuration
# in another linked worktree.
new_committed_repo dormant-sibling
main_repo=$repo
sibling_repo=$test_root/dormant-sibling-linked
git -C "$main_repo" worktree add -q -b dormant-sibling-linked "$sibling_repo"
sibling_config=$(worktree_config_path "$sibling_repo")
git config --file "$sibling_config" core.hooksPath custom-sibling
before_main=$(relevant_state "$main_repo")
before_sibling=$(relevant_state "$sibling_repo")
before_shared=$(cat "$main_repo/.git/config")
before_sibling_config=$(cat "$sibling_config")
expect_install_failure "$main_repo" "has dormant worktree configuration"
after_main=$(relevant_state "$main_repo")
after_sibling=$(relevant_state "$sibling_repo")
assert_equal "$before_main" "$after_main" "dormant sibling refusal preserves invoking configuration"
assert_equal "$before_sibling" "$after_sibling" "dormant sibling refusal preserves sibling configuration"
assert_equal "$before_shared" "$(cat "$main_repo/.git/config")" "dormant sibling refusal preserves shared configuration exactly"
assert_equal "$before_sibling_config" "$(cat "$sibling_config")" "dormant sibling refusal preserves sibling configuration exactly"
assert_config_unset "$main_repo" local extensions.worktreeConfig
assert_config_unset "$main_repo" effective core.hooksPath
assert_config_unset "$sibling_repo" effective core.hooksPath
pass "dormant sibling hook configuration is not activated"

# The complete sibling inventory includes include directives and their target
# settings, and it is read in that sibling's own Git context.
new_committed_repo dormant-sibling-include
main_repo=$repo
sibling_repo=$test_root/dormant-sibling-include-linked
git -C "$main_repo" worktree add -q -b dormant-sibling-include-linked "$sibling_repo"
main_config=$(worktree_config_path "$main_repo")
sibling_config=$(worktree_config_path "$sibling_repo")
sibling_include=$sibling_repo/dormant-worktree.config
printf '# invoking worktree marker\n' >"$main_config"
git config --file "$sibling_config" include.path "$sibling_include"
git config --file "$sibling_include" user.email dormant-sibling@example.com
before_main=$(relevant_state "$main_repo")
before_sibling=$(relevant_state "$sibling_repo")
before_shared=$(cat "$main_repo/.git/config")
before_main_config=$(cat "$main_config")
before_sibling_config=$(cat "$sibling_config")
before_sibling_include=$(cat "$sibling_include")
expect_install_failure "$main_repo" "has dormant worktree configuration"
after_main=$(relevant_state "$main_repo")
after_sibling=$(relevant_state "$sibling_repo")
assert_equal "$before_main" "$after_main" "included sibling refusal preserves invoking configuration"
assert_equal "$before_sibling" "$after_sibling" "included sibling refusal preserves sibling configuration"
assert_equal "$before_shared" "$(cat "$main_repo/.git/config")" "included sibling refusal preserves shared configuration exactly"
assert_equal "$before_main_config" "$(cat "$main_config")" "included sibling refusal preserves invoking worktree configuration exactly"
assert_equal "$before_sibling_config" "$(cat "$sibling_config")" "included sibling refusal preserves sibling worktree configuration exactly"
assert_equal "$before_sibling_include" "$(cat "$sibling_include")" "included sibling refusal preserves included configuration exactly"
assert_config_unset "$main_repo" local extensions.worktreeConfig
assert_config_unset "$main_repo" effective core.hooksPath
assert_config_unset "$sibling_repo" effective core.hooksPath
pass "included dormant sibling configuration is not activated"

# An include-expanded shared-local inventory must expose values hidden before a
# direct legacy .githooks entry. Refusal preserves every linked-worktree file
# and leaves both worktrees on the old effective path.
new_committed_repo included-shared-legacy
main_repo=$repo
sibling_repo=$test_root/included-shared-legacy-linked
git -C "$main_repo" worktree add -q -b included-shared-legacy-linked "$sibling_repo"
main_config=$(worktree_config_path "$main_repo")
sibling_config=$(worktree_config_path "$sibling_repo")
shared_include=$test_root/included-shared-legacy.config
git -C "$main_repo" config --local include.path "$shared_include"
git config --file "$shared_include" core.hooksPath custom-hidden
git -C "$main_repo" config --local extensions.worktreeConfig false
printf '\n[core]\n\thooksPath = .githooks\n' >>"$main_repo/.git/config"
git config --file "$main_config" core.hooksPath .githooks
printf '# sibling worktree marker\n' >"$sibling_config"
assert_config_value .githooks "$main_repo" effective core.hooksPath
assert_config_value .githooks "$sibling_repo" effective core.hooksPath
before_main=$(relevant_state "$main_repo")
before_sibling=$(relevant_state "$sibling_repo")
before_shared=$(cat "$main_repo/.git/config")
before_include=$(cat "$shared_include")
before_main_config=$(cat "$main_config")
before_sibling_config=$(cat "$sibling_config")
expect_install_failure "$main_repo" "shared local core.hooksPath has multiple values"
after_main=$(relevant_state "$main_repo")
after_sibling=$(relevant_state "$sibling_repo")
assert_equal "$before_main" "$after_main" "included shared-local refusal preserves invoking configuration"
assert_equal "$before_sibling" "$after_sibling" "included shared-local refusal preserves sibling configuration"
assert_equal "$before_shared" "$(cat "$main_repo/.git/config")" "included shared-local refusal preserves shared configuration exactly"
assert_equal "$before_include" "$(cat "$shared_include")" "included shared-local refusal preserves included configuration exactly"
assert_equal "$before_main_config" "$(cat "$main_config")" "included shared-local refusal preserves invoking worktree configuration exactly"
assert_equal "$before_sibling_config" "$(cat "$sibling_config")" "included shared-local refusal preserves sibling worktree configuration exactly"
assert_config_value false "$main_repo" local extensions.worktreeConfig
assert_config_value .githooks "$main_repo" effective core.hooksPath
assert_config_value .githooks "$sibling_repo" effective core.hooksPath
pass "included shared-local hook conflicts are refused before migration"

# Migrate only the installer's old shared .githooks value. An older sibling
# must stop inheriting that relative path, and every mutation keeps this
# worktree hooked.
new_committed_repo legacy
legacy_repo=$repo
old_oid=$(git -C "$legacy_repo" rev-parse HEAD)
mkdir "$legacy_repo/.githooks"
printf '#!/usr/bin/env bash\nexit 0\n' >"$legacy_repo/.githooks/pre-commit"
chmod +x "$legacy_repo/.githooks/pre-commit"
git -C "$legacy_repo" add .githooks/pre-commit
git -C "$legacy_repo" commit -qm hooks
old_worktree=$test_root/legacy-old-worktree
git -C "$legacy_repo" worktree add -q --detach "$old_worktree" "$old_oid"
git -C "$legacy_repo" config --local core.hooksPath .githooks
make_git_shim
migration_state=$test_root/migration-shim-state
migration_log=$test_root/migration.log
mkdir "$migration_state"
(
    cd "$legacy_repo"
    env REAL_GIT="$real_git" \
        GIT_SHIM_STATE_DIR="$migration_state" \
        GIT_MUTATION_LOG="$migration_log" \
        PATH="$shim_dir:$original_path" \
        "$installer"
) >/dev/null
mutation_count=0
while IFS= read -r effective; do
    mutation_count=$((mutation_count + 1))
    assert_equal .githooks "$effective" "migration mutation $mutation_count remains hooked"
done <"$migration_log"
assert_equal 3 "$mutation_count" "legacy migration mutation count"
assert_config_unset "$legacy_repo" local core.hooksPath
assert_config_value .githooks "$legacy_repo" worktree core.hooksPath
assert_config_unset "$old_worktree" worktree core.hooksPath
assert_config_unset "$old_worktree" effective core.hooksPath
if [[ -e $old_worktree/.githooks ]]; then
    fail "historical sibling unexpectedly contains .githooks"
fi
before=$(relevant_state "$legacy_repo")
run_install "$legacy_repo"
after=$(relevant_state "$legacy_repo")
assert_equal "$before" "$after" "repeat migrated installation is a no-op"
pass "legacy shared configuration migrates without an unhooked interval"

# Hidden and raw repository values are checked independently before mutation.
new_repo hidden-local
git -C "$repo" config --local extensions.worktreeConfig true
git -C "$repo" config --local core.hooksPath custom-local
git -C "$repo" config --worktree core.hooksPath .githooks
assert_refusal_preserves_state "$repo" "shared local core.hooksPath is already set"

new_repo raw-worktree
raw_config=$(worktree_config_path "$repo")
git config --file "$raw_config" core.hooksPath custom-worktree
assert_refusal_preserves_state "$repo" "current worktree core.hooksPath is already set"

new_repo empty-local
git -C "$repo" config --local core.hooksPath ''
assert_refusal_preserves_state "$repo" "shared local core.hooksPath is explicitly empty"

new_repo empty-worktree
raw_config=$(worktree_config_path "$repo")
git config --file "$raw_config" core.hooksPath ''
assert_refusal_preserves_state "$repo" "current worktree core.hooksPath is explicitly empty"

new_repo duplicate-local
git -C "$repo" config --local --add core.hooksPath .githooks
git -C "$repo" config --local --add core.hooksPath .githooks
assert_refusal_preserves_state "$repo" "shared local core.hooksPath has multiple values"

new_repo duplicate-worktree
raw_config=$(worktree_config_path "$repo")
git config --file "$raw_config" --add core.hooksPath .githooks
git config --file "$raw_config" --add core.hooksPath .githooks
assert_refusal_preserves_state "$repo" "current worktree core.hooksPath has multiple values"
pass "local and raw worktree conflicts are refused before mutation"

# Effective settings from outside the repository, including command-level
# overrides that mask an installed worktree value, are never replaced.
new_repo global-effective
git config --global core.hooksPath global-custom
assert_refusal_preserves_state "$repo" "effective core.hooksPath is set to 'global-custom'"
git config --global core.hooksPath .githooks
assert_refusal_preserves_state "$repo" "effective core.hooksPath is set outside"
git config --global --unset-all core.hooksPath

new_repo command-effective
run_install "$repo"
before=$(relevant_state "$repo")
if failure_output=$(
    cd "$repo" &&
        env GIT_CONFIG_COUNT=1 \
            GIT_CONFIG_KEY_0=core.hooksPath \
            GIT_CONFIG_VALUE_0=command-custom \
            "$installer" 2>&1
); then
    fail "installer ignored a command-level core.hooksPath override"
fi
case $failure_output in
*"effective core.hooksPath is set to 'command-custom'"*) ;;
*) fail "command-level conflict had the wrong diagnostic: $failure_output" ;;
esac
after=$(relevant_state "$repo")
assert_equal "$before" "$after" "command-level refusal preserves configuration"
pass "custom effective configurations are refused"

# Default hooks that would be disabled or reactivated block a transition.
new_repo default-fresh-block
printf '#!/usr/bin/env bash\nexit 0\n' >"$repo/.git/hooks/pre-commit"
chmod +x "$repo/.git/hooks/pre-commit"
assert_refusal_preserves_state "$repo" "an executable Git hook already exists"

new_repo default-legacy-block
git -C "$repo" config --local core.hooksPath .githooks
printf '#!/usr/bin/env bash\nexit 0\n' >"$repo/.git/hooks/post-commit"
chmod +x "$repo/.git/hooks/post-commit"
assert_refusal_preserves_state "$repo" "an executable Git hook already exists"

new_repo harmless-defaults
printf '#!/usr/bin/env bash\nexit 0\n' >"$repo/.git/hooks/custom.sample"
chmod +x "$repo/.git/hooks/custom.sample"
printf '#!/usr/bin/env bash\nexit 0\n' >"$repo/.git/hooks/not-executable"
chmod -x "$repo/.git/hooks/not-executable"
run_install "$repo"
printf '#!/usr/bin/env bash\nexit 0\n' >"$repo/.git/hooks/added-later"
chmod +x "$repo/.git/hooks/added-later"
before=$(relevant_state "$repo")
run_install "$repo"
after=$(relevant_state "$repo")
assert_equal "$before" "$after" "completed repeat ignores a later default hook"
pass "default hook conflicts are preserved without breaking completed repeats"

# Failures returned after writes exercise rollback of both a fresh install and
# a legacy migration whose final shared-local removal already took effect.
new_repo rollback-fresh
fresh_rollback_repo=$repo
fresh_state=$test_root/fresh-rollback-shim
mkdir "$fresh_state"
before=$(relevant_state "$fresh_rollback_repo")
if (
    cd "$fresh_rollback_repo"
    env REAL_GIT="$real_git" \
        GIT_SHIM_STATE_DIR="$fresh_state" \
        GIT_FAIL_MUTATION_AT=2 \
        PATH="$shim_dir:$original_path" \
        "$installer"
) >/dev/null 2>&1; then
    fail "injected fresh-install failure unexpectedly succeeded"
fi
after=$(relevant_state "$fresh_rollback_repo")
assert_equal "$before" "$after" "fresh-install rollback restores configuration"
git -C "$fresh_rollback_repo" status --short >/dev/null

new_repo rollback-legacy
git -C "$repo" config --local core.hooksPath .githooks
git -C "$repo" config --local extensions.worktreeConfig false
legacy_rollback_repo=$repo
legacy_state=$test_root/legacy-rollback-shim
mkdir "$legacy_state"
before=$(relevant_state "$legacy_rollback_repo")
if (
    cd "$legacy_rollback_repo"
    env REAL_GIT="$real_git" \
        GIT_SHIM_STATE_DIR="$legacy_state" \
        GIT_FAIL_MUTATION_AT=3 \
        PATH="$shim_dir:$original_path" \
        "$installer"
) >/dev/null 2>&1; then
    fail "injected legacy-migration failure unexpectedly succeeded"
fi
after=$(relevant_state "$legacy_rollback_repo")
assert_equal "$before" "$after" "legacy rollback restores extension, worktree, and local state"
assert_config_value false "$legacy_rollback_repo" local extensions.worktreeConfig
assert_config_value .githooks "$legacy_rollback_repo" local core.hooksPath
assert_config_unset "$legacy_rollback_repo" worktree core.hooksPath
git -C "$legacy_rollback_repo" status --short >/dev/null

# Verification failures after every intended write still trigger rollback.
new_repo rollback-post-verify
post_verify_repo=$repo
post_verify_state=$test_root/post-verify-rollback-shim
mkdir "$post_verify_state"
before=$(relevant_state "$post_verify_repo")
if failure_output=$(
    cd "$post_verify_repo" &&
        env REAL_GIT="$real_git" \
            GIT_SHIM_STATE_DIR="$post_verify_state" \
            GIT_EFFECTIVE_AFTER_MUTATION=custom-post-write \
            PATH="$shim_dir:$original_path" \
            "$installer" 2>&1
); then
    fail "post-write verification failure unexpectedly succeeded"
fi
case $failure_output in
*".githooks is not effective for the current worktree"*) ;;
*) fail "post-write verification failure had the wrong diagnostic: $failure_output" ;;
esac
after=$(relevant_state "$post_verify_repo")
assert_equal "$before" "$after" "post-write verification rollback restores configuration"
assert_config_unset "$post_verify_repo" local extensions.worktreeConfig
assert_config_unset "$post_verify_repo" local core.hooksPath
assert_config_unset "$post_verify_repo" worktree core.hooksPath
assert_config_unset "$post_verify_repo" effective core.hooksPath
pass "configuration writes and post-write verification failures roll back transactionally"
