#!/usr/bin/env bash
if pkg-config libseccomp 2>/dev/null; then
    echo seccomp
fi
