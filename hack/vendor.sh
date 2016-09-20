#!/usr/bin/env bash
set -e

# this script is used to update vendored dependencies
#
# Usage:
# vendor.sh revendor all dependencies
# vendor.sh github.com/docker/engine-api revendor only the engine-api dependency.
# vendor.sh github.com/docker/engine-api v0.3.3 vendor only engine-api at the specified tag/commit.
# vendor.sh git github.com/docker/engine-api v0.3.3 is the same but specifies the VCS for cases where the VCS is something else than git
# vendor.sh git golang.org/x/sys eb2c74142fd19a79b3f237334c7384d5167b1b46 https://github.com/golang/sys.git vendor only golang.org/x/sys downloading from the specified URL

cd "$(dirname "$BASH_SOURCE")/.."
source 'hack/.vendor-helpers.sh'

case $# in
0)
	rm -rf vendor/
	;;
# If user passed arguments to the script
1)
	path="$PWD/hack/vendor.sh"
	if ! cloneGrep="$(grep -E "^clone [^ ]+ $1" "$path")"; then
		echo >&2 "error: failed to find 'clone ... $1' in $path"
		exit 1
	fi
	eval "$cloneGrep"
	clean
	exit 0
	;;
2)
	rm -rf "vendor/src/$1"
	clone git "$1" "$2"
	clean
	exit 0
	;;
[34])
	rm -rf "vendor/src/$2"
	clone "$@"
	clean
	exit 0
	;;
*)
	>&2 echo "error: unexpected parameters"
	exit 1
	;;
esac

clone git github.com/Sirupsen/logrus v0.10.0
clone git github.com/containers/image f6f11ab5cf8b1e70ef4aa3f8b6fdb4b671d16abd
clone git golang.org/x/net 991d3e32f76f19ee6d9caadb3a22eae8d23315f7 https://github.com/golang/net.git
clone git github.com/docker/docker master
clone git github.com/urfave/cli v1.18.1
clone git github.com/opencontainers/runtime-tools master
clone git github.com/tchap/go-patricia v2.2.6
clone git github.com/rajatchopra/ocicni master
clone git github.com/containernetworking/cni master
clone git github.com/kubernetes/kubernetes ff3ca3d616518087dc20180f69bb4038379f1028
clone git google.golang.org/grpc v1.0.1-GA https://github.com/grpc/grpc-go.git
clone git github.com/opencontainers/runtime-spec bb6925ea99f0e366a3f7d1c975f6577475ca25f0

clean

mv vendor/src/* vendor/
