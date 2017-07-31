#!/bin/bash

set -ex

git clone https://github.com/ostreedev/ostree ../ostree
cd ../ostree
./autogen.sh --prefix=/usr/local
make all
sudo make install
