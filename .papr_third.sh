#!/bin/bash

set -ex

make .gitvalidation
make gofmt
make lint
make integration
make docs
make

echo "... pretending to send notification to chat.freenode.net#cri-o ..."
