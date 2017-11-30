#!/bin/bash

set -ex

if [ "$1" == "RedHat" ] || [ "$1" == "Fedora" ] || [ "$1" == "CentOS" ]
then
    rm -f *.src.rpm;
    $(type -P dnf || type -P yum) install -y $(find -name '*.rpm');
else
    echo "Distro $1 not supported yet"
    exit 1
fi
