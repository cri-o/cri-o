#!/bin/bash

set -ex

if [ "$1" == "RedHat" ] || [ "$1" == "CentOS" ] || [ "$1" == "Fedora" ]
then
    iptables -F
    sysctl -w net.ipv4.conf.all.route_localnet=1
    iptables -t nat -I POSTROUTING -s 127.0.0.1 ! -d 127.0.0.1 -j MASQUERADE

    if [ "$1" == "RedHat" ] || [ "$1" == "CentOS" ]
    then
        grubby --update-kernel=ALL --args="rootflags=pquota"
    fi
else
    echo "Distro $1 not supported yet"
    exit 1
fi
