#!/bin/bash
if ! pkg-config gpgme 2>/dev/null; then
    echo containers_image_openpgp
fi
