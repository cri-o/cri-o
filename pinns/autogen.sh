#!/bin/sh
set -ex

if ! (pkg-config --version > /dev/null 2>&1); then
    echo "pkg-config not found.  Please install it">&2
    exit 1
fi

exec autoreconf -fis
