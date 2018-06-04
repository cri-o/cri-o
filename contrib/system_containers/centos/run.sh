#!/bin/sh

# Ensure that new process maintain this SELinux label
PID=$$
LABEL=`tr -d '\000' < /proc/$PID/attr/current`
printf %s $LABEL > /proc/self/attr/exec

test -e /etc/sysconfig/crio-storage && source /etc/sysconfig/crio-storage
test -e /etc/sysconfig/crio-network && source /etc/sysconfig/crio-network

export HTTP_PROXY HTTPS_PROXY NO_PROXY

exec /usr/bin/crio --bind-mount-prefix=/host --log-level=$LOG_LEVEL
