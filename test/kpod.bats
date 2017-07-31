#!/usr/bin/env bats

load helpers

IMAGE="alpine:latest"
ROOT="$TESTDIR/crio"
RUNROOT="$TESTDIR/crio-run"
KPOD_OPTIONS="--root $ROOT --runroot $RUNROOT --storage-driver vfs"

function teardown() {
    cleanup_test
}

@test "kpod version test" {
	run ${KPOD_BINARY} version
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull from docker with tag" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull debian:6.0.10
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi debian:6.0.10
	[ "$status" -eq 0 ]
}

@test "kpod pull from docker without tag" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull debian
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi debian
	[ "$status" -eq 0 ]
}

@test "kpod pull from a non-docker registry with tag" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull registry.fedoraproject.org/fedora:rawhide
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi registry.fedoraproject.org/fedora:rawhide
	[ "$status" -eq 0 ]
}

@test "kpod pull from a non-docker registry without tag" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull registry.fedoraproject.org/fedora
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi registry.fedoraproject.org/fedora
	[ "$status" -eq 0 ]
}

@test "kpod pull using digest" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull alpine@sha256:1072e499f3f655a032e88542330cf75b02e7bdf673278f701d7ba61629ee3ebe
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi alpine:latest
	[ "$status" -eq 0 ]
}

@test "kpod pull from a non existent image" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull umohnani/get-started
	echo "$output"
	[ "$status" -ne 0 ]
}

@test "kpod history default" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} history $IMAGE
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
	[ "$status" -eq 0 ]
}

@test "kpod history with format" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} history --format "{{.ID}} {{.Created}}" $IMAGE
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
	[ "$status" -eq 0 ]
}

@test "kpod history human flag" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} history --human=false $IMAGE
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
	[ "$status" -eq 0 ]
}

@test "kpod history quiet flag" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} history -q $IMAGE
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
	[ "$status" -eq 0 ]
}

@test "kpod history no-trunc flag" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} history --no-trunc $IMAGE
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
	[ "$status" -eq 0 ]
}

@test "kpod history json flag" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull $IMAGE
	[ "$status" -eq 0 ]
	run bash -c "${KPOD_BINARY} ${KPOD_OPTIONS} history --json $IMAGE | python -m json.tool"
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
	[ "$status" -eq 0 ]
}

@test "kpod push to containers/storage" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS push "$IMAGE" containers-storage:[$ROOT]busybox:test
    echo "$output"
    [ "$status" -eq 0 ]
    run crioctl image remove "$IMAGE"
    run crioctl image remove busybox:test
    stop_crio
}

@test "kpod push to directory" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
    run mkdir /tmp/busybox
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS push "$IMAGE" dir:/tmp/busybox
    echo "$output"
    [ "$status" -eq 0 ]
    run crioctl image remove "$IMAGE"
    run rm -rf /tmp/busybox
    stop_crio
}

@test "kpod push to docker archive" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS push "$IMAGE" docker-archive:/tmp/busybox-archive:1.26
    echo "$output"
    [ "$status" -eq 0 ]
    rm /tmp/busybox-archive
    run crioctl image remove "$IMAGE"
    stop_crio
}

@test "kpod push to oci without compression" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
    run mkdir /tmp/oci-busybox
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS push --disable-compression "$IMAGE" oci:/tmp/oci-busybox
    echo "$output"
    [ "$status" -eq 0 ]
    run rm -rf /tmp/oci-busybox
    run crioctl image remove "$IMAGE"
    stop_crio
}

@test "kpod push without signatures" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
    run mkdir /tmp/busybox
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS push --remove-signatures "$IMAGE" dir:/tmp/busybox
    echo "$output"
    [ "$status" -eq 0 ]
    run rm -rf /tmp/busybox
    run crioctl image remove "$IMAGE"
    stop_crio
}

@test "kpod inspect image" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull redis:alpine
    [ "$status" -eq 0 ]
    run bash -c "${KPOD_BINARY} $KPOD_OPTIONS inspect redis:alpine | python -m json.tool"
    echo "$output"
    [ "$status" -eq 0 ]
}
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi redis:alpine


@test "kpod inspect non-existent container" {
    run ${KPOD_BINARY} $KPOD_OPTIONS inspect 14rcole/non-existent
    echo "$output"
    [ "$status" -ne 0 ]
}

@test "kpod inspect with format" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull redis:alpine
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS --format {{.ID}} inspect redis:alpine
    [ "$status" -eq 0]
    inspectOutput="$output"
    run ${KPOD_BINARY} $KPOD_OPTIONS images --quiet redis:alpine
    [ "$status" -eq 0]
    [ "$output" -eq "$inspectOutput" ]
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi redis:alpine
}

@test "kpod inspect specified type" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull redis:alpine
    [ "$status" -eq 0 ]
    run bash -c "${KPOD_BINARY} $KPOD_OPTIONS inspect --type image redis:alpine | python -m json.tool"
    echo "$output"
    [ "$status" -eq 0]
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi redis:alpine
}

@test "kpod images" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull debian:6.0.10
    run ${KPOD_BINARY} $KPOD_OPTIONS images
    [ "$status" -eq 0 ]
}

@test "kpod images test valid json" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull debian:6.0.10
    run ${KPOD_BINARY} $KPOD_OPTIONS images --format json
    echo "$output" | python -m json.tool
    [ "$status" -eq 0 ]
}

@test "kpod images check name json output" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull debian:6.0.10
    run ${KPOD_BINARY} $KPOD_OPTIONS images --format json
    name=$(echo $output | python -c 'import sys; import json; print(json.loads(sys.stdin.read())[0])["names"][0]')
    [ "$name" == "docker.io/library/debian:6.0.10" ]
}
