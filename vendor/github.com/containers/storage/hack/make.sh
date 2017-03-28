#!/usr/bin/env bash
set -e

# This script builds various binary artifacts from a checkout of the storage
# source code.
#
# Requirements:
# - The current directory should be a checkout of the storage source code
# (https://github.com/containers/storage). Whatever version is checked out will
# be built.
# - The VERSION file, at the root of the repository, should exist, and
#   will be used as the oci-storage binary version and package version.
# - The hash of the git commit will also be included in the oci-storage binary,
#   with the suffix -unsupported if the repository isn't clean.
# - The right way to call this script is to invoke "make" from
#   your checkout of the storage repository.

set -o pipefail

export PATH=/usr/local/go/bin:${PATH}
export PKG='github.com/containers/storage'
export SCRIPTDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
export MAKEDIR="$SCRIPTDIR/make"
export PKG_CONFIG=${PKG_CONFIG:-pkg-config}

: ${TEST_REPEAT:=0}

# List of bundles to create when no argument is passed
DEFAULT_BUNDLES=(
	validate-dco
	validate-gofmt
	validate-lint
	validate-pkg
	validate-test
	validate-toml
	validate-vet

	binary

	test-unit

	gccgo
	cross
)

VERSION=$(< ./VERSION)
if command -v git &> /dev/null && [ -d .git ] && git rev-parse &> /dev/null; then
	GITCOMMIT=$(git rev-parse --short HEAD)
	if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
		GITCOMMIT="$GITCOMMIT-unsupported"
		echo "#~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~"
		echo "# GITCOMMIT = $GITCOMMIT"
		echo "# The version you are building is listed as unsupported because"
		echo "# there are some files in the git repository that are in an uncommited state."
		echo "# Commit these changes, or add to .gitignore to remove the -unsupported from the version."
		echo "# Here is the current list:"
		git status --porcelain --untracked-files=no
		echo "#~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~"
	fi
	! BUILDTIME=$(date --rfc-3339 ns 2> /dev/null | sed -e 's/ /T/') &> /dev/null
	if [ -z $BUILDTIME ]; then
		# If using bash 3.1 which doesn't support --rfc-3389, eg Windows CI
		BUILDTIME=$(date -u)
	fi
elif [ -n "$GITCOMMIT" ]; then
	:
else
	echo >&2 'error: .git directory missing and GITCOMMIT not specified'
	echo >&2 '  Please either build with the .git directory accessible, or specify the'
	echo >&2 '  exact (--short) commit hash you are building using GITCOMMIT for'
	echo >&2 '  future accountability in diagnosing build issues.  Thanks!'
	exit 1
fi

export GOPATH="${GOPATH:-/go}"

if [ "$(go env GOOS)" = 'solaris' ]; then
  # sys/unix is installed outside the standard library on solaris
  # TODO need to allow for version change, need to get version from go
  export GOPATH="${GOPATH}:/usr/lib/gocode/1.6.2"
fi

if [ ! "$GOPATH" ]; then
	echo >&2 'error: missing GOPATH; please see https://golang.org/doc/code.html#GOPATH'
	exit 1
fi

if [ "$EXPERIMENTAL" ]; then
	echo >&2 '# WARNING! EXPERIMENTAL is set: building experimental features'
	echo >&2
	BUILDTAGS+=" experimental"
fi

# test whether "btrfs/version.h" exists and apply btrfs_noversion appropriately
if \
	command -v gcc &> /dev/null \
	&& ! gcc -E - -o /dev/null &> /dev/null <<<'#include <btrfs/version.h>' \
; then
	BUILDTAGS+=' btrfs_noversion'
fi

# test whether "libdevmapper.h" is new enough to support deferred remove
# functionality.
if \
	command -v gcc &> /dev/null \
	&& ! ( echo -e  '#include <libdevmapper.h>\nint main() { dm_task_deferred_remove(NULL); }'| gcc -xc - -o /dev/null -ldevmapper &> /dev/null ) \
; then
       BUILDTAGS+=' libdm_no_deferred_remove'
fi

# Use these flags when compiling the tests and final binary
source "$SCRIPTDIR/make/.go-autogen"
if [ -z "$DEBUG" ]; then
	LDFLAGS='-w'
fi

BUILDFLAGS=( $BUILDFLAGS "${ORIG_BUILDFLAGS[@]}" )

if [ "$(uname -s)" = 'FreeBSD' ]; then
	# Tell cgo the compiler is Clang, not GCC
	# https://code.google.com/p/go/source/browse/src/cmd/cgo/gcc.go?spec=svne77e74371f2340ee08622ce602e9f7b15f29d8d3&r=e6794866ebeba2bf8818b9261b54e2eef1c9e588#752
	export CC=clang

	# "-extld clang" is a workaround for
	# https://code.google.com/p/go/issues/detail?id=6845
	LDFLAGS="$LDFLAGS -extld clang"
fi

HAVE_GO_TEST_COVER=
if \
	go help testflag | grep -- -cover > /dev/null \
	&& go tool -n cover > /dev/null 2>&1 \
; then
	HAVE_GO_TEST_COVER=1
fi
TIMEOUT=5m

# If $TESTFLAGS is set in the environment, it is passed as extra arguments to 'go test'.
# You can use this to select certain tests to run, eg.
#
#     TESTFLAGS='-test.run ^TestBuild$' ./hack/make.sh test-unit
#
# For integration-cli test, we use [gocheck](https://labix.org/gocheck), if you want
# to run certain tests on your local host, you should run with command:
#
#     TESTFLAGS='-check.f DockerSuite.TestBuild*' ./hack/make.sh binary test-integration-cli
#
go_test_dir() {
	dir=$1
	coverpkg=$2
	testcover=()
	testcoverprofile=()
	testbinary="$DEST/test.main"
	if [ "$HAVE_GO_TEST_COVER" ]; then
		# if our current go install has -cover, we want to use it :)
		mkdir -p "$DEST/coverprofiles"
		coverprofile="storage${dir#.}"
		coverprofile="$ABS_DEST/coverprofiles/${coverprofile//\//-}"
		testcover=( -test.cover )
		testcoverprofile=( -test.coverprofile "$coverprofile" $coverpkg )
	fi
	(
		echo '+ go test' $TESTFLAGS "${PKG}${dir#.}"
		cd "$dir"
		export DEST="$ABS_DEST" # we're in a subshell, so this is safe -- our integration-cli tests need DEST, and "cd" screws it up
		go test -c -o "$testbinary" ${testcover[@]} -ldflags "$LDFLAGS" "${BUILDFLAGS[@]}"
		i=0
		while ((++i)); do
			test_env "$testbinary" ${testcoverprofile[@]} $TESTFLAGS
			if [ $i -gt "$TEST_REPEAT" ]; then
				break
			fi
			echo "Repeating test ($i)"
		done
	)
}
test_env() {
	# use "env -i" to tightly control the environment variables that bleed into the tests
	env -i \
		DEST="$DEST" \
		GOPATH="$GOPATH" \
		GOTRACEBACK=all \
		HOME="$ABS_DEST/fake-HOME" \
		PATH="${GOPATH}/bin:/usr/local/go/bin:$PATH" \
		TEMP="$TEMP" \
		"$@"
}

# a helper to provide ".exe" when it's appropriate
binary_extension() {
	echo -n $(go env GOEXE)
}

hash_files() {
	while [ $# -gt 0 ]; do
		f="$1"
		shift
		dir="$(dirname "$f")"
		base="$(basename "$f")"
		for hashAlgo in md5 sha256; do
			if command -v "${hashAlgo}sum" &> /dev/null; then
				(
					# subshell and cd so that we get output files like:
					#   $HASH oci-storage-$VERSION
					# instead of:
					#   $HASH /go/src/github.com/.../$VERSION/binary/oci-storage-$VERSION
					cd "$dir"
					"${hashAlgo}sum" "$base" > "$base.$hashAlgo"
				)
			fi
		done
	done
}

bundle() {
	local bundle="$1"; shift
	echo "---> Making bundle: $(basename "$bundle") (in $DEST)"
	source "$SCRIPTDIR/make/$bundle" "$@"
}

main() {
	# We want this to fail if the bundles already exist and cannot be removed.
	# This is to avoid mixing bundles from different versions of the code.
	mkdir -p bundles
	if [ -e "bundles/$VERSION" ] && [ -z "$KEEPBUNDLE" ]; then
		echo "bundles/$VERSION already exists. Removing."
		rm -fr "bundles/$VERSION" && mkdir "bundles/$VERSION" || exit 1
		echo
	fi

	if [ "$(go env GOHOSTOS)" != 'windows' ]; then
		# Windows and symlinks don't get along well

		rm -f bundles/latest
		ln -s "$VERSION" bundles/latest
	fi

	if [ $# -lt 1 ]; then
		bundles=(${DEFAULT_BUNDLES[@]})
	else
		bundles=($@)
	fi

	for bundle in ${bundles[@]}; do
		export DEST="bundles/$VERSION/$(basename "$bundle")"
		# Cygdrive paths don't play well with go build -o.
		if [[ "$(uname -s)" == CYGWIN* ]]; then
			export DEST="$(cygpath -mw "$DEST")"
		fi
		mkdir -p "$DEST"
		ABS_DEST="$(cd "$DEST" && pwd -P)"
		bundle "$bundle"
		echo
	done
}

main "$@"
