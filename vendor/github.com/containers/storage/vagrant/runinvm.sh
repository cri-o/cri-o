#!/bin/bash
set -e
export PKG='github.com/containers/storage'
export VAGRANT_MACHINES="fedora debian"
if test -z "$VAGRANT_PROVIDER" ; then
	if lsmod | grep -q '^kvm ' ; then
		VAGRANT_PROVIDER=libvirt
	elif lsmod | grep -q '^vboxdrv ' ; then
		VAGRANT_PROVIDER=virtualbox
	fi
fi
export VAGRANT_PROVIDER=${VAGRANT_PROVIDER:-libvirt}
export VAGRANT_PROVIDER=${VAGRANT_PROVIDER:-virtualbox}
if ${IN_VAGRANT_MACHINE:-false} ; then
	unset AUTO_GOPATH
	export GOPATH=/go:/go/src/${PKG}/vendor
	export PATH=/usr/lib/go-1.6/bin:/go/src/${PKG}/vendor/src/github.com/golang/lint/golint:${PATH}
	"$@"
else
	vagrant up --provider ${VAGRANT_PROVIDER}
	for machine in ${VAGRANT_MACHINES} ; do
		vagrant reload ${machine}
		vagrant ssh ${machine} -c "cd /go/src/${PKG}; IN_VAGRANT_MACHINE=true sudo -E $0 $*"
		vagrant ssh ${machine} -c "sudo poweroff &"
	done
fi
