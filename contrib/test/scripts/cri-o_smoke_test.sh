#!/bin/bash

set -x

# Restarting CRI-O service
systemctl --no-pager restart cri-o

# Dump the CRI-O service journal
journalctl --unit cri-o --no-pager

# Fail if CRI-O service is not active
systemctl is-active cri-o || exit $?

runc --version

crioctl --version

crioctl info

crioctl runtimeversion

crioctl image pull busybox
