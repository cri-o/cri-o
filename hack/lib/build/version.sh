#!/usr/bin/env bash

# This library holds utility functions for determining
# product versions from Git repository state.

# os::build::version::get_vars loads the standard version variables as
# ENV vars
function os::build::version::get_vars() {
    if [[ -n "${OS_VERSION_FILE-}" ]]; then
        if [[ -f "${OS_VERSION_FILE}" ]]; then
            source "${OS_VERSION_FILE}"
            return
        fi
        if [[ ! -d "${OS_ROOT}/.git" ]]; then
            os::log::fatal "No version file at ${OS_VERSION_FILE}"
        fi
        os::log::warning "No version file at ${OS_VERSION_FILE}, falling back to git versions"
    fi
    os::build::version::git_vars
}
readonly -f os::build::version::get_vars

# os::build::version::git_vars looks up the current Git vars if they have not been calculated.
function os::build::version::git_vars() {
    if [[ -n "${OS_GIT_VERSION-}" ]]; then
        return 0
    fi

    local git=(git --work-tree "${OS_ROOT}")

    if [[ -n ${OS_GIT_COMMIT-} ]] || OS_GIT_COMMIT=$("${git[@]}" rev-parse --short "HEAD^{commit}" 2>/dev/null); then
        if [[ -z ${OS_GIT_TREE_STATE-} ]]; then
            # Check if the tree is dirty.  default to dirty
            if git_status=$("${git[@]}" status --porcelain 2>/dev/null) && [[ -z ${git_status} ]]; then
                OS_GIT_TREE_STATE="clean"
            else
                OS_GIT_TREE_STATE="dirty"
            fi
        fi
        # Use git describe to find the version based on annotated tags.
        if [[ -n ${OS_GIT_VERSION-} ]] || OS_GIT_VERSION=$("${git[@]}" describe --long --tags --abbrev=7 --match 'v[0-9]*' "${OS_GIT_COMMIT}^{commit}" 2>/dev/null); then
            # Try to match the "git describe" output to a regex to try to extract
            # the "major" and "minor" versions and whether this is the exact tagged
            # version or whether the tree is between two tagged versions.
            if [[ "${OS_GIT_VERSION}" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)(\.[0-9]+)*([-].*)?$ ]]; then
                OS_GIT_MAJOR=${BASH_REMATCH[1]}
                OS_GIT_MINOR=${BASH_REMATCH[2]}
                OS_GIT_PATCH=${BASH_REMATCH[3]}
                if [[ -n "${BASH_REMATCH[5]}" ]]; then
                    OS_GIT_MINOR+="+"
                fi
            fi

            # This translates the "git describe" to an actual semver.org
            # compatible semantic version that looks something like this:
            #   v1.1.0-alpha.0.6+84c76d1-345
            OS_GIT_VERSION=$(echo "${OS_GIT_VERSION}" | sed "s/-\([0-9]\{1,\}\)-g\([0-9a-f]\{7,40\}\)$/\+\2-\1/")
            # If this is an exact tag, remove the last segment.
            OS_GIT_VERSION=$(echo "${OS_GIT_VERSION}" | sed "s/-0$//")
            if [[ "${OS_GIT_TREE_STATE}" == "dirty" ]]; then
                # git describe --dirty only considers changes to existing files, but
                # that is problematic since new untracked .go files affect the build,
                # so use our idea of "dirty" from git status instead.
                OS_GIT_VERSION+="-dirty"
            fi
        fi
    fi
}
readonly -f os::build::version::git_vars

# Saves the environment flags to $1
function os::build::version::save_vars() {
    cat <<EOF
OS_GIT_COMMIT='${OS_GIT_COMMIT-}'
OS_GIT_TREE_STATE='${OS_GIT_TREE_STATE-}'
OS_GIT_VERSION='${OS_GIT_VERSION-}'
OS_GIT_MAJOR='${OS_GIT_MAJOR-}'
OS_GIT_MINOR='${OS_GIT_MINOR-}'
OS_GIT_PATCH='${OS_GIT_PATCH-}'
KUBE_GIT_MAJOR='${KUBE_GIT_MAJOR-}'
KUBE_GIT_MINOR='${KUBE_GIT_MINOR-}'
KUBE_GIT_COMMIT='${KUBE_GIT_COMMIT-}'
KUBE_GIT_VERSION='${KUBE_GIT_VERSION-}'
ETCD_GIT_VERSION='${ETCD_GIT_VERSION-}'
ETCD_GIT_COMMIT='${ETCD_GIT_COMMIT-}'
EOF
}
readonly -f os::build::version::save_vars
