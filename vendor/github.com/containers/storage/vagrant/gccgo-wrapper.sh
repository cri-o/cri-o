#!/bin/bash
additional=
for arg in "$@" ; do
	if test -d "$arg"/github.com/containers/storage ; then
		additional="$additional -I $arg/github.com/containers/storage/vendor"
	fi
done
echo gccgo $additional "$@" > /tmp/gccgo
gccgo $additional "$@"
