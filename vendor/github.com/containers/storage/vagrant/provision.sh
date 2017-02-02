#!/bin/bash
set -e
source /etc/os-release
case "${ID_LIKE:-${ID:-unknown}}" in
	debian)
		export DEBIAN_FRONTEND=noninteractive
		apt-get -q update
		apt-get -q -y install linux-headers-`uname -r`
		echo deb http://httpredir.debian.org/debian testing main    >  /etc/apt/sources.list
		echo deb http://httpredir.debian.org/debian testing contrib >> /etc/apt/sources.list
		apt-get -q update
		apt-get -q -y install systemd
		apt-get -q -y install apt make git gccgo golang btrfs-progs libdevmapper-dev
		apt-get -q -y install zfs-dkms zfsutils-linux
		modprobe aufs
		modprobe zfs
		;;
	fedora)
		dnf -y clean all
		dnf -y install golang-bin make git-core btrfs-progs-devel device-mapper-devel
		dnf -y install gcc-go
		alternatives --set go /usr/lib/golang/bin/go
		;;
	unknown)
		echo Unknown box OS, unsure of how to install required packages.
		exit 1
		;;
esac
mkdir -p /go/src/github.com/containers
rm -f /go/src/github.com/containers/storage
ln -s /vagrant /go/src/github.com/containers/storage
export GOPATH=/go:/go/src/github.com/containers/storage/vendor
export PATH=/usr/lib/go-1.6/bin:/go/src/${PKG}/vendor/src/github.com/golang/lint/golint:${PATH}
go get github.com/golang/lint
