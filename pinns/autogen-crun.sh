#!/bin/sh
set -ex

git submodule update --init --recursive

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd $DIR/crun && $DIR/crun/autogen.sh
cd $DIR/crun && $DIR/crun/configure
make -C $DIR/crun
