#!/bin/sh -ex

# package installer for *cri-o* container runtime <https://cri-o.io>
# executable version of install.md, needs root so run it with "sudo"

# systemd distro
. /etc/os-release

# cri-o version
export VERSION=1.18

case "${ID}-${VERSION_ID}" in
	fedora-*)
		OS="Fedora_${VERSION_ID}"
		PKG=yum
		;;
	centos-*)
		OS="CentOS_${VERSION_ID}"
		PKG=yum
		;;
	ubuntu-*)
		OS="xUbuntu_${VERSION_ID}"
		PKG=apt
		;;
	debian-*)
		OS="Debian_${VERSION_ID}"
		PKG=apt
		;;
	*)
		echo "ERROR: Unsupported distribution '${PRETTY_NAME}'" >&2
		exit 1
		;;
esac

# <https://build.opensuse.org/project/show/devel:kubic:libcontainers>

URL="https://download.opensuse.org/repositories"
REPO="$URL/devel:kubic:libcontainers"
SLASH="$URL/devel:/kubic:/libcontainers"
FILE="devel:kubic:libcontainers"
NAME="cri-o"
CHANNEL="stable"

if [ "$PKG" = "yum" ]; then
	curl -L "$SLASH:/$CHANNEL/$OS/$FILE:$CHANNEL.repo" -o /etc/yum.repos.d/$FILE:$CHANNEL.repo
	curl -L "$REPO:$CHANNEL:$NAME:$VERSION/$OS/$FILE:$CHANNEL:$NAME:$VERSION.repo" -o /etc/yum.repos.d/$FILE:$CHANNEL:$NAME:$VERSION.repo

	yum install -y cri-o
fi

if [ "$PKG" = "apt" ]; then
	echo "deb $SLASH:/$CHANNEL/$OS/ /" > /etc/apt/sources.list.d/$FILE:$CHANNEL.list
	echo "deb $SLASH:/$CHANNEL:/$NAME:/$VERSION/$OS/ /" > /etc/apt/sources.list.d/$FILE:$CHANNEL:$NAME:$VERSION.list

	curl -L "$REPO:$CHANNEL:$NAME:$VERSION/$OS/Release.key" | apt-key add -
	curl -L "$SLASH:/$CHANNEL/$OS/Release.key" | apt-key add -

	apt-get update
	apt-get install -y cri-o cri-o-runc
fi
