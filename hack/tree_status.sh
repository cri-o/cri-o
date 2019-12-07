#!/bin/bash
set -e

STATUS=$(git status --porcelain)
if [[ -z $STATUS ]]
then
	echo "tree is clean"
else
	echo "tree is dirty, please commit all changes and sync the vendor.conf"
	echo ""
	echo "$STATUS"
	echo ""
	git diff | cat
	exit 1
fi
