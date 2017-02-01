#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

BASH="${BASH_VERSION%.*}"
BASH_MAJOR="${BASH%.*}"
BASH_MINOR="${BASH#*.}"

if test "${BASH_MAJOR}" -eq 3 && test "${BASH_MINOR}" -eq 0
then
	echo "ERROR: ${0} requires Bash version >= 3.1" >&2
	echo "you're running ${BASH}, which doesn't support += array assignment" >&2
	exit 1
fi

RUNTIME="runc"
TEST_ARGS=('--args' '/runtimetest')
KEEP=0 # Track whether we keep the test directory around or clean it up

usage() {
	echo "$0 -l <log-level> -r <runtime> -k -h"
}

error() {
	echo $*
	exit 1
}

info() {
	echo $*
}

while getopts "l:r:kh" opt; do
	case "${opt}" in
		l)
			TEST_ARGS+=('--args' "--log-level=${OPTARG}")
			;;
		r)
			RUNTIME=${OPTARG}
			;;
		h)
			usage
			exit 0
			;;
		k)
			KEEP=1
			;;
		\?)
			usage
			exit 1
			;;
	esac
done

info "-----------------------------------------------------------------------------------"
info "                         VALIDATING RUNTIME: ${RUNTIME}"
info "-----------------------------------------------------------------------------------"

if ! command -v ${RUNTIME} > /dev/null; then
	error "Runtime ${RUNTIME} not found in the path"
fi

TMPDIR=$(mktemp -d)
TESTDIR=${TMPDIR}/busybox
mkdir -p ${TESTDIR}

cleanup() {
	if [ "${KEEP}" -eq 0 ]; then
		rm -rf ${TMPDIR}
	else
		info "Remove the test directory ${TMPDIR} after use"
	fi
}
trap cleanup EXIT

tar -xf  rootfs.tar.gz -C ${TESTDIR}
cp runtimetest ${TESTDIR}

oci-runtime-tool generate --output "${TESTDIR}/config.json" "${TEST_ARGS[@]}" --rootfs-path '.'

TESTCMD="${RUNTIME} start $(uuidgen)"
pushd $TESTDIR > /dev/null
if ! ${TESTCMD}; then
	error "Runtime ${RUNTIME} failed validation"
else
	info "Runtime ${RUNTIME} passed validation"
fi
popd > /dev/null
