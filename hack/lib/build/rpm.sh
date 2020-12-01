#!/usr/bin/env bash

# This library holds utilities for building RPMs from Origin.

# os::build::rpm::generate_nevra_vars determines the NEVRA of the RPMs
# that would be built from the current git state.
#
# Globals:
#  - OS_GIT_VERSION
# Arguments:
#  - None
# Exports:
#  - OS_RPM_VERSION
#  - OS_RPM_RELEASE
#  - OS_RPM_ARCHITECTURE
function os::build::rpm::get_nvra_vars() {
    # the package name can be overwritten but is normally 'origin'
    OS_RPM_ARCHITECTURE="$(uname -i)"

    # we can extract the pacakge version from the build version
    os::build::version::get_vars
    if [[ "${OS_GIT_VERSION}" =~ ^v([0-9](\.[0-9]+)*)(.*) ]]; then
        OS_RPM_VERSION="${BASH_REMATCH[1]}"
        metadata="${BASH_REMATCH[3]}"
    else
        os::log::fatal "Malformed \$OS_GIT_VERSION: ${OS_GIT_VERSION}"
    fi

    # we can generate the package release from the git version metadata
    # OS_GIT_VERSION will always have metadata, but either contain
    # pre-release information _and_ build metadata, or only the latter.
    # Build metadata may or may not contain the number of commits past
    # the last tag. If no commit number exists, we are on a tag and use 0.
    # ex.
    #    -alpha.0+shasums-123-dirty
    #    -alpha.0+shasums-123
    #    -alpha.0+shasums-dirty
    #    -alpha.0+shasums
    #    +shasums-123-dirty
    #    +shasums-123
    #    +shasums-dirty
    #    +shasums
    if [[ "${metadata:0:1}" == "+" ]]; then
        # we only have build metadata, but need to massage it so
        # we can generate a valid RPM release from it
        if [[ "${metadata}" =~ ^\+([a-z0-9]{7,40})(-([0-9]+))?(-dirty)?$ ]]; then
            build_sha="${BASH_REMATCH[1]}"
            build_num="${BASH_REMATCH[3]:-0}"
        else
            os::log::fatal "Malformed git version metadata: ${metadata}"
        fi
        OS_RPM_RELEASE="1.${build_num}.${build_sha}"
    elif [[ "${metadata:0:1}" == "-" ]]; then
        # we have both build metadata and pre-release info
        if [[ "${metadata}" =~ ^-([^\+]+)\+([a-z0-9]{7,40})(-([0-9]+))?(-dirty)?$ ]]; then
            pre_release="${BASH_REMATCH[1]}"
            build_sha="${BASH_REMATCH[2]}"
            build_num="${BASH_REMATCH[4]:-0}"
        else
            os::log::fatal "Malformed git version metadata: ${metadata}"
        fi
        OS_RPM_RELEASE="0.${pre_release}.${build_num}.${build_sha}"
    else
        os::log::fatal "Malformed git version metadata: ${metadata}"
    fi

    OS_RPM_GIT_VARS=$(os::build::version::save_vars | tr '\n' ' ')

    export OS_RPM_VERSION OS_RPM_RELEASE OS_RPM_ARCHITECTURE OS_RPM_GIT_VARS
}
