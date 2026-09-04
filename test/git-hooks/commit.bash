#!/usr/bin/env bash

set -euo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
original_tmpdir=${TMPDIR:-/tmp}
suite_tmp=$(mktemp -d "$original_tmpdir/crio-commit-hooks.XXXXXX")
cleanup() {
    rm -rf "$suite_tmp"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p "$suite_tmp/home" "$suite_tmp/xdg" "$suite_tmp/tmp"
export HOME="$suite_tmp/home"
export XDG_CONFIG_HOME="$suite_tmp/xdg"
export TMPDIR="$suite_tmp/tmp"
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME='Hook Test'
export GIT_AUTHOR_EMAIL='hook.test@example.com'
export GIT_COMMITTER_NAME=$GIT_AUTHOR_NAME
export GIT_COMMITTER_EMAIL=$GIT_AUTHOR_EMAIL
export LC_ALL=C
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY
unset GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CONFIG_PARAMETERS

commit_hook="$repo_root/.githooks/commit-msg"
command_log="$suite_tmp/command.log"
test_count=0

fail() {
    printf 'not ok - %s\n' "$1" >&2
    exit 1
}

expect_success() {
    description=$1
    shift
    : >"$command_log"
    if "$@" >"$command_log" 2>&1; then
        test_count=$((test_count + 1))
        printf 'ok %d - %s\n' "$test_count" "$description"
        return
    fi

    cat "$command_log" >&2
    fail "$description"
}

expect_failure() {
    description=$1
    shift
    : >"$command_log"
    if "$@" >"$command_log" 2>&1; then
        fail "$description (unexpected success)"
    fi

    test_count=$((test_count + 1))
    printf 'ok %d - %s\n' "$test_count" "$description"
}

assert_equal() {
    description=$1
    expected=$2
    actual=$3
    if [[ $actual != "$expected" ]]; then
        printf 'expected: %s\nactual:   %s\n' "$expected" "$actual" >&2
        fail "$description"
    fi
}

assert_contains() {
    description=$1
    file=$2
    text=$3
    if ! grep -Fq -- "$text" "$file"; then
        printf 'missing text: %s\n' "$text" >&2
        cat "$file" >&2
        fail "$description"
    fi
}

assert_not_contains() {
    description=$1
    file=$2
    text=$3
    if grep -Fq -- "$text" "$file"; then
        printf 'unexpected text: %s\n' "$text" >&2
        cat "$file" >&2
        fail "$description"
    fi
}

init_repo() {
    git -c init.defaultBranch=main init -q "$1"
}

install_hooks() {
    target_repo=$1
    mkdir -p "$target_repo/.githooks"
    for hook in commit-msg pre-commit applypatch-msg pre-applypatch pre-merge-commit; do
        cp -p "$repo_root/.githooks/$hook" "$target_repo/.githooks/$hook"
    done
    git -C "$target_repo" config core.hooksPath .githooks
}

run_commit_hook() {
    target_repo=$1
    message_file=$2
    (
        cd "$target_repo"
        "$commit_hook" "$message_file"
    )
}

write_message() {
    message_file=$1
    message=$2
    printf '%s\n' "$message" >"$message_file"
}

check_message_success() {
    description=$1
    message=$2
    message_file="$suite_tmp/message-$test_count"
    write_message "$message_file" "$message"
    expect_success "$description" run_commit_hook "$direct_repo" "$message_file"
}

check_message_failure() {
    description=$1
    message=$2
    message_file="$suite_tmp/message-$test_count"
    write_message "$message_file" "$message"
    expect_failure "$description" run_commit_hook "$direct_repo" "$message_file"
    assert_contains "$description reports the expected trailer shape" "$command_log" \
        'Signed-off-by: Name <email>'
}

for hook in commit-msg pre-commit applypatch-msg pre-applypatch pre-merge-commit; do
    [[ -x $repo_root/.githooks/$hook ]] || fail "$hook is executable"
done

direct_repo="$suite_tmp/direct"
init_repo "$direct_repo"
check_message_success 'canonical one-token sign-off is accepted' \
    $'Canonical\n\nSigned-off-by: Alice <alice@example.com>'
check_message_success 'canonical multi-token sign-off is accepted' \
    $'Canonical\n\nSigned-off-by: Alice B Example <alice.b@example.com>'
check_message_success 'canonical sign-off among unrelated trailers is accepted' \
    $'Canonical\n\nReviewed-by: Reviewer <reviewer@example.com>\nSigned-off-by: Alice Example <alice@example.com>\nAcked-by: Acker <acker@example.com>'
check_message_failure 'missing sign-off is rejected' $'No trailer\n\nBody'
check_message_failure 'short sign-off is rejected' $'Short\n\nSigned-off-by: x'
check_message_failure 'name-less sign-off is rejected' \
    $'No name\n\nSigned-off-by: <alice@example.com>'
check_message_failure 'bracket-less sign-off is rejected' \
    $'No brackets\n\nSigned-off-by: Alice alice@example.com'
check_message_failure 'missing-at sign-off is rejected' \
    $'No at\n\nSigned-off-by: Alice <alice.example.com>'
check_message_failure 'empty local address component is rejected' \
    $'Empty local\n\nSigned-off-by: Alice <@example.com>'
check_message_failure 'empty domain address component is rejected' \
    $'Empty domain\n\nSigned-off-by: Alice <alice@>'
check_message_failure 'extra-at sign-off is rejected' \
    $'Extra at\n\nSigned-off-by: Alice <alice@example@com>'
check_message_failure 'address whitespace is rejected' \
    $'Address whitespace\n\nSigned-off-by: Alice <alice @example.com>'
check_message_failure 'lowercase noncanonical key is rejected' \
    $'Wrong key\n\nsigned-off-by: Alice <alice@example.com>'
check_message_failure 'whitespace before the sign-off colon is rejected' \
    $'Wrong separator\n\nSigned-off-by : Alice Example <alice@example.com>'
check_message_failure 'trailing sign-off content is rejected' \
    $'Trailing\n\nSigned-off-by: Alice <alice@example.com> forged'
check_message_failure 'Merge-looking subject without merge state is rejected' \
    $'Merge branch topic\n\nThis is an ordinary message.'

large_valid="$suite_tmp/large-valid-message"
{
    printf 'Large valid message\n\n'
    printf 'Signed-off-by: Alice Example <alice@example.com>\n'
    i=1
    while ((i <= 50000)); do
        printf 'Reviewed-by: Reviewer %d <reviewer%d@example.com>\n' "$i" "$i"
        i=$((i + 1))
    done
} >"$large_valid"
git -C "$direct_repo" interpret-trailers --parse "$large_valid" >"$suite_tmp/large-valid-parsed"
large_valid_count=$(wc -l <"$suite_tmp/large-valid-parsed" | tr -d '[:space:]')
assert_equal 'valid large message has 50,001 parsed trailers' 50001 "$large_valid_count"
expect_success 'valid sign-off before 50,000 parsed trailers is accepted' \
    run_commit_hook "$direct_repo" "$large_valid"

large_invalid="$suite_tmp/large-invalid-message"
{
    printf 'Large invalid message\n\n'
    i=1
    while ((i <= 50000)); do
        printf 'Reviewed-by: Reviewer %d <reviewer%d@example.com>\n' "$i" "$i"
        i=$((i + 1))
    done
} >"$large_invalid"
git -C "$direct_repo" interpret-trailers --parse "$large_invalid" >"$suite_tmp/large-invalid-parsed"
large_invalid_count=$(wc -l <"$suite_tmp/large-invalid-parsed" | tr -d '[:space:]')
assert_equal 'invalid large message has 50,000 parsed trailers' 50000 "$large_invalid_count"
expect_failure '50,000 parsed trailers without a sign-off are rejected' \
    run_commit_hook "$direct_repo" "$large_invalid"
assert_contains 'large rejection reports the expected trailer shape' "$command_log" \
    'Signed-off-by: Name <email>'

real_git=$(command -v git)
mkdir -p "$suite_tmp/failing-parser-bin"
cat >"$suite_tmp/failing-parser-bin/git" <<'EOF'
#!/usr/bin/env bash
if [[ $1 == interpret-trailers ]]; then
    printf 'Signed-off-by: Alice Example <alice@example.com>\n'
    exit 17
fi
exec "$REAL_GIT" "$@"
EOF
chmod +x "$suite_tmp/failing-parser-bin/git"
run_with_failing_parser() {
    target_repo=$1
    message_file=$2
    (
        cd "$target_repo"
        REAL_GIT="$real_git" PATH="$suite_tmp/failing-parser-bin:$PATH" \
            "$commit_hook" "$message_file"
    )
}
expect_failure 'interpret-trailers failure rejects even a produced canonical sign-off' \
    run_with_failing_parser "$direct_repo" "$large_valid"

# Real commit plumbing must preserve the same canonical syntax distinction that
# the direct hook checks exercise.
syntax_repo="$suite_tmp/commit-syntax"
init_repo "$syntax_repo"
printf 'base\n' >"$syntax_repo/content.txt"
git -C "$syntax_repo" add content.txt
git -C "$syntax_repo" commit -q -m 'base'
install_hooks "$syntax_repo"
syntax_base=$(git -C "$syntax_repo" rev-parse HEAD)
printf 'noncanonical\n' >"$syntax_repo/content.txt"
git -C "$syntax_repo" add content.txt
expect_failure 'git commit rejects whitespace before the sign-off colon' \
    git -C "$syntax_repo" commit -m $'noncanonical sign-off\n\nSigned-off-by : Alice Example <alice@example.com>'
assert_equal 'rejected noncanonical commit leaves HEAD unchanged' "$syntax_base" \
    "$(git -C "$syntax_repo" rev-parse HEAD)"
expect_success 'git commit accepts a canonical sign-off' \
    git -C "$syntax_repo" commit -m $'canonical sign-off\n\nSigned-off-by: Alice Example <alice@example.com>'
git -C "$syntax_repo" log -1 --format=%B >"$suite_tmp/canonical-commit-message"
assert_contains 'real commit retains the canonical sign-off' "$suite_tmp/canonical-commit-message" \
    'Signed-off-by: Alice Example <alice@example.com>'

patch_repo="$suite_tmp/patch-source"
init_repo "$patch_repo"
printf 'base\n' >"$patch_repo/base.txt"
git -C "$patch_repo" add base.txt
git -C "$patch_repo" commit -q -m 'base'
base_oid=$(git -C "$patch_repo" rev-parse HEAD)

git -C "$patch_repo" checkout -q -b unsigned-patch
printf 'unsigned clean\n' >"$patch_repo/unsigned.txt"
git -C "$patch_repo" add unsigned.txt
git -C "$patch_repo" commit -q -m 'unsigned clean patch'
git -C "$patch_repo" format-patch -1 --stdout >"$suite_tmp/unsigned.patch"

git -C "$patch_repo" checkout -q main
git -C "$patch_repo" checkout -q -b noncanonical-signoff-patch
printf 'noncanonical sign-off\n' >"$patch_repo/noncanonical.txt"
git -C "$patch_repo" add noncanonical.txt
git -C "$patch_repo" commit -q -m $'noncanonical sign-off patch\n\nSigned-off-by : Alice Example <alice@example.com>'
git -C "$patch_repo" format-patch -1 --stdout >"$suite_tmp/noncanonical-signoff.patch"

git -C "$patch_repo" checkout -q main
git -C "$patch_repo" checkout -q -b whitespace-patch
printf 'signed trailing whitespace   \n' >"$patch_repo/whitespace.txt"
git -C "$patch_repo" add whitespace.txt
git -C "$patch_repo" commit -q -s -m 'signed whitespace patch'
git -C "$patch_repo" format-patch -1 --stdout >"$suite_tmp/whitespace.patch"

git -C "$patch_repo" checkout -q main
git -C "$patch_repo" checkout -q -b clean-patch
printf 'signed clean\n' >"$patch_repo/clean.txt"
git -C "$patch_repo" add clean.txt
git -C "$patch_repo" commit -q -s -m 'signed clean patch'
git -C "$patch_repo" format-patch -1 --stdout >"$suite_tmp/clean.patch"

git -C "$patch_repo" checkout -q main
am_repo="$suite_tmp/am-target"
git clone -q "$patch_repo" "$am_repo"
git -C "$am_repo" checkout -q --detach "$base_oid"
install_hooks "$am_repo"
am_base=$(git -C "$am_repo" rev-parse HEAD)
expect_failure 'git am rejects an unsigned clean patch through applypatch-msg' \
    git -C "$am_repo" am "$suite_tmp/unsigned.patch"
assert_contains 'unsigned git am reports sign-off policy' "$command_log" \
    'missing or malformed Signed-off-by trailer'
assert_equal 'unsigned git am leaves HEAD unchanged' "$am_base" \
    "$(git -C "$am_repo" rev-parse HEAD)"
expect_success 'unsigned git am state can be aborted' git -C "$am_repo" am --abort

expect_failure 'git am rejects whitespace before the sign-off colon through applypatch-msg' \
    git -C "$am_repo" am "$suite_tmp/noncanonical-signoff.patch"
assert_contains 'noncanonical git am reports sign-off policy' "$command_log" \
    'missing or malformed Signed-off-by trailer'
assert_equal 'noncanonical git am leaves HEAD unchanged' "$am_base" \
    "$(git -C "$am_repo" rev-parse HEAD)"
expect_success 'noncanonical git am state can be aborted' git -C "$am_repo" am --abort

expect_failure 'git am rejects signed trailing whitespace through pre-applypatch' \
    git -C "$am_repo" -c apply.whitespace=nowarn am "$suite_tmp/whitespace.patch"
assert_contains 'whitespace git am reports the staged error' "$command_log" 'trailing whitespace.'
assert_equal 'whitespace git am leaves HEAD unchanged' "$am_base" \
    "$(git -C "$am_repo" rev-parse HEAD)"
expect_success 'whitespace git am state can be aborted' git -C "$am_repo" am --abort

expect_success 'git am applies a signed clean patch through both delegates' \
    git -C "$am_repo" am "$suite_tmp/clean.patch"
assert_equal 'signed clean patch creates one commit' "$am_base" \
    "$(git -C "$am_repo" rev-parse HEAD^)"
git -C "$am_repo" log -1 --format=%B >"$suite_tmp/canonical-am-message"
assert_contains 'git am retains the canonical sign-off' "$suite_tmp/canonical-am-message" \
    'Signed-off-by: Hook Test <hook.test@example.com>'

merge_repo="$suite_tmp/merge-common"
merge_linked="$suite_tmp/merge-linked"
init_repo "$merge_repo"
printf 'base\n' >"$merge_repo/base.txt"
git -C "$merge_repo" add base.txt
git -C "$merge_repo" commit -q -m 'base'
git -C "$merge_repo" checkout -q -b topic
echo 'topic' >"$merge_repo/topic.txt"
git -C "$merge_repo" add topic.txt
git -C "$merge_repo" commit -q -m 'topic change'
git -C "$merge_repo" checkout -q main
echo 'main' >"$merge_repo/main.txt"
git -C "$merge_repo" add main.txt
git -C "$merge_repo" commit -q -m 'main change'
git -C "$merge_repo" checkout -q topic
git -C "$merge_repo" worktree add -q "$merge_linked" main
install_hooks "$merge_linked"
expect_success 'unsigned custom-message automatic merge succeeds while MERGE_HEAD exists' \
    git -C "$merge_linked" merge --no-ff -m 'Integrate topic changes' topic
merge_oid=$(git -C "$merge_linked" rev-parse HEAD)
git -C "$merge_linked" log -1 --format=%B >"$suite_tmp/unsigned-merge-message"
assert_not_contains 'automatic merge is unsigned' "$suite_tmp/unsigned-merge-message" 'Signed-off-by:'
merge_head_path=$(git -C "$merge_linked" rev-parse --git-path MERGE_HEAD)
[[ ! -f $merge_head_path ]] || fail 'MERGE_HEAD is removed after an automatic merge'
expect_failure 'immediate unsigned merge amend is rejected without MERGE_HEAD' \
    git -C "$merge_linked" commit --amend --no-edit
assert_equal 'rejected unsigned merge amend leaves HEAD unchanged' "$merge_oid" \
    "$(git -C "$merge_linked" rev-parse HEAD)"
expect_success 'signed merge amend succeeds' \
    git -C "$merge_linked" commit --amend --no-edit -s
signed_merge_oid=$(git -C "$merge_linked" rev-parse HEAD)
[[ $signed_merge_oid != "$merge_oid" ]] || fail 'signed merge amend replaces the commit'
git -C "$merge_linked" log -1 --format=%B >"$suite_tmp/signed-merge-message"
assert_contains 'signed merge amend adds a canonical sign-off' "$suite_tmp/signed-merge-message" \
    'Signed-off-by: Hook Test <hook.test@example.com>'

whitespace_repo="$suite_tmp/whitespace-common"
whitespace_linked="$suite_tmp/whitespace-linked"
init_repo "$whitespace_repo"
printf 'base\n' >"$whitespace_repo/base.txt"
git -C "$whitespace_repo" add base.txt
git -C "$whitespace_repo" commit -q -m 'base'
git -C "$whitespace_repo" checkout -q -b topic
printf 'bad merge content   \n' >"$whitespace_repo/bad.txt"
git -C "$whitespace_repo" add bad.txt
git -C "$whitespace_repo" commit -q -m 'topic with whitespace'
git -C "$whitespace_repo" checkout -q main
printf 'main\n' >"$whitespace_repo/main.txt"
git -C "$whitespace_repo" add main.txt
git -C "$whitespace_repo" commit -q -m 'diverging main change'
git -C "$whitespace_repo" checkout -q topic
git -C "$whitespace_repo" worktree add -q "$whitespace_linked" main
install_hooks "$whitespace_linked"
whitespace_head=$(git -C "$whitespace_linked" rev-parse HEAD)
expect_failure 'automatic merge rejects staged trailing whitespace through pre-merge-commit' \
    git -C "$whitespace_linked" merge --no-ff -m 'Integrate whitespace topic' topic
assert_contains 'automatic merge reports trailing whitespace' "$command_log" 'trailing whitespace.'
assert_equal 'rejected whitespace merge leaves HEAD unchanged' "$whitespace_head" \
    "$(git -C "$whitespace_linked" rev-parse HEAD)"
whitespace_merge_head=$(git -C "$whitespace_linked" rev-parse --git-path MERGE_HEAD)
[[ -f $whitespace_merge_head ]] || fail 'failed automatic merge retains MERGE_HEAD for recovery'
expect_success 'failed automatic merge can be aborted' git -C "$whitespace_linked" merge --abort

printf '1..%d\n' "$test_count"
