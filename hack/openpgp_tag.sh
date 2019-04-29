#!/bin/bash
if ! gpgme-config --libs &>/dev/null; then
    echo containers_image_openpgp
fi
