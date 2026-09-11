#!/usr/bin/env bash

set -euo pipefail

readonly hooks_path=.githooks
repo=$(git rev-parse --show-toplevel)
cd "$repo"

common_git_dir=$(git rev-parse --git-common-dir)
common_git_dir=$(cd "$common_git_dir" && pwd -P)
git_dir=$(git rev-parse --git-dir)
git_dir=$(cd "$git_dir" && pwd -P)
readonly common_git_dir git_dir
readonly common_config=$common_git_dir/config
readonly worktree_config=$git_dir/config.worktree
readonly default_hooks=$common_git_dir/hooks

state_dir=$(mktemp -d "${TMPDIR:-/tmp}/crio-install-git-hooks.XXXXXX")
rollback_needed=false

read_config_values() {
    name=$1
    shift

    if git config "$@" --get-all "$name" >"$config_output"; then
        return
    else
        status=$?
    fi

    if [[ $status -ne 1 ]]; then
        echo "error: unable to inspect $name" >&2
        exit "$status"
    fi

    : >"$config_output"
}

read_included_config_values() {
    name=$1
    file=$2
    context=${3:-$repo}
    included_output=$state_dir/included-output

    if git -C "$context" config --includes --null --show-origin --file "$file" --get-all "$name" >"$included_output"; then
        :
    else
        status=$?
        if [[ $status -ne 1 ]]; then
            echo "error: unable to inspect $name in $file" >&2
            exit "$status"
        fi
    fi

    : >"$config_output"
    : >"$config_origin_output"
    exec 3<"$included_output"
    while IFS= read -r -d '' origin <&3; do
        if ! IFS= read -r -d '' value <&3; then
            echo "error: unable to determine the value of $name in $file" >&2
            exit 1
        fi
        printf '%s\n' "$origin" >>"$config_origin_output"
        printf '%s\n' "$value" >>"$config_output"
    done
    exec 3<&-
}

read_config_inventory() {
    context=$1
    file=$2
    inventory_output=$state_dir/config-inventory
    inventory_count=0
    inventory_origin=
    inventory_name=
    inventory_value=

    if [[ ! -e $file && ! -L $file ]]; then
        return
    fi
    if ! git -C "$context" config --includes --null --show-origin --file "$file" --list >"$inventory_output"; then
        echo "error: unable to inspect worktree configuration in $file" >&2
        exit 1
    fi

    exec 3<"$inventory_output"
    while IFS= read -r -d '' origin <&3; do
        if ! IFS= read -r -d '' entry <&3; then
            echo "error: unable to parse worktree configuration in $file" >&2
            exit 1
        fi
        inventory_count=$((inventory_count + 1))
        if [[ $inventory_count -eq 1 ]]; then
            inventory_origin=$origin
            inventory_name=${entry%%$'\n'*}
            if [[ $entry == *$'\n'* ]]; then
                inventory_value=${entry#*$'\n'}
            fi
        fi
    done
    exec 3<&-
}

count_config_values() {
    config_count=0
    config_value=
    while IFS= read -r value || [[ -n $value ]]; do
        config_count=$((config_count + 1))
        if [[ $config_count -eq 1 ]]; then
            config_value=$value
        fi
    done <"$config_output"
}

write_worktree_list() {
    worktree_list=$state_dir/worktrees
    if ! git worktree list --porcelain -z >"$worktree_list"; then
        echo "error: unable to enumerate linked worktrees" >&2
        exit 1
    fi
}

resolve_worktree_git_dir() {
    worktree_path=$1
    if ! worktree_git_dir=$(git -C "$worktree_path" rev-parse --git-dir); then
        echo "error: unable to inspect linked worktree at '$worktree_path'" >&2
        exit 1
    fi
    case $worktree_git_dir in
    /*) ;;
    *) worktree_git_dir=$worktree_path/$worktree_git_dir ;;
    esac
    worktree_git_dir=$(cd "$worktree_git_dir" && pwd -P)
}

refuse_unsafe_shared_hook_migration() {
    if [[ $prior_local_count -ne 1 ]]; then
        return
    fi

    write_worktree_list
    while IFS= read -r -d '' field; do
        if [[ $field != worktree\ * ]]; then
            continue
        fi

        worktree_path=${field#worktree }
        config_output=$state_dir/shared-hooks
        config_origin_output=$state_dir/shared-hook-origins
        read_included_config_values core.hooksPath "$common_config" "$worktree_path"
        count_config_values
        IFS= read -r config_origin <"$config_origin_output" || config_origin=
        if [[ $config_count -ne 1 || $config_value != "$hooks_path" || $config_origin != "file:$common_config" ]]; then
            echo "error: linked worktree at '$worktree_path' has additional shared local core.hooksPath configuration" >&2
            exit 1
        fi
    done <"$worktree_list"
}

refuse_dormant_worktree_configuration() {
    write_worktree_list
    while IFS= read -r -d '' field; do
        if [[ $field != worktree\ * ]]; then
            continue
        fi

        worktree_path=${field#worktree }
        resolve_worktree_git_dir "$worktree_path"
        candidate_config=$worktree_git_dir/config.worktree
        read_config_inventory "$worktree_path" "$candidate_config"
        if [[ $worktree_git_dir == "$git_dir" ]]; then
            if [[ $inventory_count -eq 0 ]]; then
                continue
            fi
            if [[ $inventory_count -eq 1 && $inventory_origin == "file:$candidate_config" && $inventory_name == core.hookspath && $inventory_value == "$hooks_path" ]]; then
                continue
            fi
            echo "error: current worktree has dormant configuration beyond core.hooksPath=$hooks_path" >&2
            exit 1
        fi
        if [[ $inventory_count -gt 0 ]]; then
            echo "error: sibling worktree at '$worktree_path' has dormant worktree configuration" >&2
            exit 1
        fi
    done <"$worktree_list"
}

restore_configuration() {
    restore_failed=false

    # Worktree configuration must remain enabled until its prior state is back.
    git config --local --replace-all extensions.worktreeConfig true || restore_failed=true

    if [[ $prior_worktree_count -eq 0 ]]; then
        git config --worktree --unset-all core.hooksPath >/dev/null 2>&1 || {
            status=$?
            [[ $status -eq 5 ]] || restore_failed=true
        }
    else
        git config --worktree --replace-all core.hooksPath "$prior_worktree_value" || restore_failed=true
    fi

    if [[ $prior_local_count -eq 0 ]]; then
        git config --local --unset-all core.hooksPath >/dev/null 2>&1 || {
            status=$?
            [[ $status -eq 5 ]] || restore_failed=true
        }
    else
        git config --local --replace-all core.hooksPath "$prior_local_value" || restore_failed=true
    fi

    git config --local --unset-all extensions.worktreeConfig >/dev/null 2>&1 || {
        status=$?
        [[ $status -eq 5 ]] || restore_failed=true
    }
    while IFS= read -r value || [[ -n $value ]]; do
        git config --local --add extensions.worktreeConfig "$value" || restore_failed=true
    done <"$state_dir/extensions"

    if [[ $restore_failed == true ]]; then
        echo "error: failed to fully restore Git hook configuration" >&2
    fi
}

cleanup() {
    status=$?
    trap - EXIT INT TERM
    if [[ $rollback_needed == true ]]; then
        set +e
        restore_configuration
    fi
    rm -rf "$state_dir"
    exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

config_output=$state_dir/local
read_config_values core.hooksPath --local
count_config_values
readonly prior_local_count=$config_count
readonly prior_local_value=$config_value

config_output=$state_dir/included-local
config_origin_output=$state_dir/included-local-origins
read_included_config_values core.hooksPath "$common_config"
count_config_values
readonly prior_included_local_count=$config_count
readonly prior_included_local_value=$config_value
IFS= read -r prior_included_local_origin <"$config_origin_output" || prior_included_local_origin=
readonly prior_included_local_origin

config_output=$state_dir/worktree
read_config_values core.hooksPath --file "$worktree_config"
count_config_values
readonly prior_worktree_count=$config_count
readonly prior_worktree_value=$config_value

config_output=$state_dir/included-worktree
config_origin_output=$state_dir/included-worktree-origins
read_included_config_values core.hooksPath "$worktree_config"
count_config_values
readonly prior_included_worktree_count=$config_count
readonly prior_included_worktree_value=$config_value

config_output=$state_dir/extensions
read_config_values extensions.worktreeConfig --local
count_config_values
readonly prior_extension_count=$config_count

if [[ $prior_included_local_count -gt 1 ]]; then
    echo "error: shared local core.hooksPath has multiple values" >&2
    exit 1
fi
if [[ $prior_included_local_count -eq 1 && -z $prior_included_local_value ]]; then
    echo "error: shared local core.hooksPath is explicitly empty" >&2
    exit 1
fi
if [[ $prior_included_local_count -eq 1 && $prior_included_local_value != "$hooks_path" ]]; then
    echo "error: shared local core.hooksPath is already set to '$prior_included_local_value'" >&2
    exit 1
fi
if [[ $prior_included_local_count -eq 1 && $prior_included_local_origin != "file:$common_config" ]]; then
    echo "error: shared local core.hooksPath is configured through an include" >&2
    exit 1
fi

if [[ $prior_included_worktree_count -gt 1 ]]; then
    echo "error: current worktree core.hooksPath has multiple values" >&2
    exit 1
fi
if [[ $prior_included_worktree_count -eq 1 && -z $prior_included_worktree_value ]]; then
    echo "error: current worktree core.hooksPath is explicitly empty" >&2
    exit 1
fi
if [[ $prior_included_worktree_count -eq 1 && $prior_included_worktree_value != "$hooks_path" ]]; then
    echo "error: current worktree core.hooksPath is already set to '$prior_included_worktree_value'" >&2
    exit 1
fi

config_output=$state_dir/effective
if git config --get core.hooksPath >"$config_output"; then
    count_config_values
    if [[ $config_count -ne 1 || -z $config_value ]]; then
        echo "error: effective core.hooksPath is explicitly empty or malformed" >&2
        exit 1
    fi
    if [[ $config_value != "$hooks_path" ]]; then
        echo "error: effective core.hooksPath is set to '$config_value'" >&2
        exit 1
    fi
    if [[ $prior_local_count -eq 0 && $prior_included_worktree_count -eq 0 ]]; then
        echo "error: effective core.hooksPath is set outside the repository's local or worktree configuration" >&2
        exit 1
    fi
else
    status=$?
    if [[ $status -ne 1 ]]; then
        echo "error: unable to inspect the effective core.hooksPath" >&2
        exit "$status"
    fi
fi

extension_enabled=false
if [[ $prior_extension_count -gt 0 ]]; then
    if extension_value=$(git config --local --type=bool --get extensions.worktreeConfig); then
        if [[ $extension_value == true ]]; then
            extension_enabled=true
        fi
    else
        echo "error: shared local extensions.worktreeConfig is not a valid boolean" >&2
        exit 1
    fi
fi

refuse_unsafe_shared_hook_migration

if [[ $extension_enabled == false ]]; then
    refuse_dormant_worktree_configuration
fi

installation_complete=false
if [[ $extension_enabled == true && $prior_included_worktree_count -eq 1 && $prior_local_count -eq 0 ]]; then
    installation_complete=true
fi

if [[ $installation_complete == false ]]; then
    for hook in "$default_hooks"/*; do
        if [[ -f $hook && -x $hook && $hook != *.sample ]]; then
            cat >&2 <<EOF
error: an executable Git hook already exists at '$hook'

Changing core.hooksPath would disable or reactivate that hook. Remove it or
migrate it into $hooks_path before running this installer again.
EOF
            exit 1
        fi
    done

    rollback_needed=true

    if [[ $extension_enabled == false ]]; then
        git config --local --replace-all extensions.worktreeConfig true
    fi

    if [[ $prior_included_worktree_count -eq 0 ]]; then
        git config --worktree --replace-all core.hooksPath "$hooks_path"
    fi

    if [[ $prior_local_count -eq 1 ]]; then
        git config --local --unset-all core.hooksPath '^\.githooks$'
    fi
fi

config_output=$state_dir/verify-local
config_origin_output=$state_dir/verify-local-origins
read_included_config_values core.hooksPath "$common_config"
count_config_values
if [[ $config_count -ne 0 ]]; then
    echo "error: shared local core.hooksPath remains configured" >&2
    exit 1
fi

config_output=$state_dir/verify-worktree
config_origin_output=$state_dir/verify-worktree-origins
read_included_config_values core.hooksPath "$worktree_config"
count_config_values
if [[ $config_count -ne 1 || $config_value != "$hooks_path" ]]; then
    echo "error: current worktree core.hooksPath was not configured" >&2
    exit 1
fi

if [[ $(git config --local --type=bool --get extensions.worktreeConfig) != true ]]; then
    echo "error: extensions.worktreeConfig was not enabled" >&2
    exit 1
fi
if [[ $(git config --get core.hooksPath) != "$hooks_path" ]]; then
    echo "error: $hooks_path is not effective for the current worktree" >&2
    exit 1
fi

rollback_needed=false
echo "Enabled the CRI-O Git hooks from $hooks_path for this worktree"
