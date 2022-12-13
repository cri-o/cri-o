#!/usr/bin/env bash

TOPDIR="${0%/*}/.."
NRITEST_BINARY="$TOPDIR/test/nri/nri.test"
NRI_BATS="$TOPDIR/test/nri.bats"

status=0
for i in $($NRITEST_BINARY -test.list Test); do
    if ! grep -q -e "-test.run $i"' *$' "$NRI_BATS"; then
        echo "NRI test case $i missing from $(realpath --relative-to "$TOPDIR" "$NRI_BATS")"
        status=1
    fi
done
exit "$status"
