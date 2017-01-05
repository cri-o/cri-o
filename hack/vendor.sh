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

clone git github.com/coreos/go-systemd master
clone git github.com/BurntSushi/toml v0.2.0
clone git github.com/Sirupsen/logrus v0.10.0

clone git github.com/containers/storage master
clone git github.com/mistifyio/go-zfs master
clone git github.com/pborman/uuid master

clone git github.com/containers/image master
clone git github.com/mtrmac/gpgme master
clone git gopkg.in/cheggaaa/pb.v1 master
clone git github.com/mattn/go-runewidth master
clone git github.com/docker/engine-api v0.4.0

clone git github.com/opencontainers/image-spec master
clone git golang.org/x/net 991d3e32f76f19ee6d9caadb3a22eae8d23315f7 https://github.com/golang/net.git
clone git github.com/docker/docker master
clone git github.com/urfave/cli v1.19.1
clone git github.com/opencontainers/runtime-tools master
clone git github.com/tchap/go-patricia v2.2.6
clone git github.com/rajatchopra/ocicni master
clone git github.com/containernetworking/cni master
clone git k8s.io/kubernetes 43110dd64d058786e975ce30d4c12a4853d1778c https://github.com/kubernetes/kubernetes
clone git google.golang.org/grpc v1.0.1-GA https://github.com/grpc/grpc-go.git
clone git github.com/opencontainers/runtime-spec bb6925ea99f0e366a3f7d1c975f6577475ca25f0
clone git github.com/docker/distribution d22e09a6686c32be8c17b684b639da4b90efe320
clone git github.com/vbatts/tar-split v0.10.1
clone git github.com/docker/go-units f2145db703495b2e525c59662db69a7344b00bb8
clone git github.com/docker/go-connections 4ccf312bf1d35e5dbda654e57a9be4c3f3cd0366
clone git github.com/docker/libtrust 9cbd2a1374f46905c68a4eb3694a130610adc62a
clone git github.com/ghodss/yaml 73d445a93680fa1a78ae23a5839bad48f32ba1ee
clone git gopkg.in/yaml.v2 d466437aa4adc35830964cffc5b5f262c63ddcb4
clone git github.com/golang/protobuf 3c84672111d91bb5ac31719e112f9f7126a0e26e
clone git github.com/golang/glog 44145f04b68cf362d9c4df2182967c2275eaefed
clone git github.com/gorilla/mux v1.1
clone git github.com/imdario/mergo 6633656539c1639d9d78127b7d47c622b5d7b6dc
clone git github.com/opencontainers/runc 3abefdff18bc201199c5dfd0e91e941cb4c61376
clone git github.com/syndtr/gocapability 2c00daeb6c3b45114c80ac44119e7b8801fdd852
clone git github.com/gogo/protobuf 43a2e0b1c32252bfbbdf81f7faa7a88fb3fa4028
clone git github.com/gorilla/context v1.1
clone git golang.org/x/sys 8f0908ab3b2457e2e15403d3697c9ef5cb4b57a9 https://github.com/golang/sys.git
clone git github.com/seccomp/libseccomp-golang 32f571b70023028bd57d9288c20efbcb237f3ce0

clean
