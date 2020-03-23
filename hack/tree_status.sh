#!/usr/bin/env bash

if git diff --exit-code; then
    echo tree is clean
else
    echo please commit your local changes
fi
