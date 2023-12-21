#!/usr/bin/env bash
set -euox pipefail

REGISTRY=quay.io/crio
IMAGE=$REGISTRY/fedora-crio-ci

ARCHES=(amd64 arm64)
QEMUVERSION=7.2.0-1
TAGS=(latest)

docker run --rm --privileged multiarch/qemu-user-static:$QEMUVERSION --reset -p yes
docker buildx version
BUILDER=$(docker buildx create --use)

cleanup() {
    docker buildx rm "$BUILDER"
}
trap cleanup EXIT

for ARCH in "${ARCHES[@]}"; do
    docker buildx build \
        --pull \
        --load \
        --platform "linux/$ARCH" \
        -t "$IMAGE-$ARCH:latest" \
        .
    for T in "${TAGS[@]}"; do
        docker push "$IMAGE-$ARCH:$T"
    done
done

for T in "${TAGS[@]}"; do
    docker manifest create --amend "$IMAGE:$T" \
        "$IMAGE-amd64:$T" \
        "$IMAGE-arm64:$T"

    for ARCH in "${ARCHES[@]}"; do
        docker manifest annotate --arch "$ARCH" "$IMAGE:$T" "$IMAGE-$ARCH:$T"
    done

    docker manifest push --purge "$IMAGE:$T"
done
