#!/usr/bin/env bash
set -euo pipefail

GCS_SA_PATH=${GCS_SA_PATH:-}

# Defaults to gs://cri-o bucket
GCS_BUCKET_NAME=${GCS_BUCKET_NAME:-cri-o}

if [[ -z $GCS_SA_PATH ]]; then
    echo "Using existing credentials for gsutil"
else
    echo "Activating GCP service account using $GCS_SA_PATH"
    gcloud auth activate-service-account --key-file="$GCS_SA_PATH"
fi

BUCKET=gs://$GCS_BUCKET_NAME

echo "Uploading artifacts to Google Cloud Bucket"
gsutil -m cp -n "build/bundle/*.tar.gz*" "$BUCKET/artifacts"

# update the latest version marker file for the branch
MARKER=$(git rev-parse --abbrev-ref HEAD)
VERSION=$(git rev-parse HEAD)

# if in detached head state, we assume we're on a tag
if [[ $MARKER == HEAD ]]; then
    # use the major.minor as marker
    VERSION=$(git describe --tags --exact-match)
    MARKER=$(echo "$VERSION" | cut -c 2-5)
fi
echo "$VERSION" >"latest-$MARKER.txt"
gsutil cp "latest-$MARKER.txt" "$BUCKET"
