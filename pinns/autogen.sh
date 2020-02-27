#!/bin/sh
if ! (pkg-config --version > /dev/null 2>&1); then
    echo "pkg-config not found.  Please install it">&2
    exit 1
fi

git submodule update --init --recursive

exec autoreconf -fis

libtoolize --copy --ltdl --force
aclocal -I aclocal
autoconf
autoheader
automake --add-missing --copy
