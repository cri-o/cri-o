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
	eval "$(grep -E "^clone [^ ]+ $1" "$0")"
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

# the following lines are in sorted order, FYI
clone git github.com/Microsoft/hcsshim v0.3.6
clone git github.com/Microsoft/go-winio v0.3.4
clone git github.com/Sirupsen/logrus v0.10.0 # logrus is a common dependency among multiple deps
# forked golang.org/x/net package includes a patch for lazy loading trace templates
clone git golang.org/x/net 2beffdc2e92c8a3027590f898fe88f69af48a3f8 https://github.com/tonistiigi/net.git
clone git golang.org/x/sys eb2c74142fd19a79b3f237334c7384d5167b1b46 https://github.com/golang/sys.git
clone git github.com/docker/go-units 651fc226e7441360384da338d0fd37f2440ffbe3
clone git github.com/docker/go-connections fa2850ff103453a9ad190da0df0af134f0314b3d
clone git github.com/docker/engine-api 1d247454d4307fb1ddf10d09fd2996394b085904
# get graph and distribution packages
clone git github.com/vbatts/tar-split v0.9.13
# get go-zfs packages
clone git github.com/mistifyio/go-zfs 22c9b32c84eb0d0c6f4043b6e90fc94073de92fa
clone git github.com/pborman/uuid v1.0
clone git github.com/opencontainers/runc cc29e3dded8e27ba8f65738f40d251c885030a28 # libcontainer

clean
