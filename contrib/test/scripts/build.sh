#!/bin/bash

set -ex

if [ "$1" == "RedHat" ] || [ "$1" == "CentOS" ] || [ "$1" == "Fedora" ]
then
    make clean-rpm
    make test-rpm
else
    echo "Distro $1 not supported yet"
    exit 1
fi
