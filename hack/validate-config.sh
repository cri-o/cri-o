#!/usr/bin/env bash
if grep -q 'Comment' pkg/config/template.go | grep -vq '{{ $.Comment }}'; then
    exit 1
fi
