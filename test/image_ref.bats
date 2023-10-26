#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

IMAGE_REF=quay.io/crio/alpine:3.9
IMAGE_DIGEST=quay.io/crio/alpine@sha256:414e0518bb9228d35e4cd5165567fb91d26c6a214e9c95899e1e056fcd349011

function run_using_digest() {
	jq '.image.image = "'"$IMAGE_DIGEST"'"' "$TESTDATA/container_config.json" > "$TESTDIR/ctr.json"
	CTR_ID=$(crictl run "$TESTDIR"/ctr.json "$TESTDATA"/sandbox_config.json)

	IMAGE=$(crictl inspect "$CTR_ID" | jq -r .status.image.image)
	[[ "$IMAGE" == "$IMAGE_DIGEST" ]]
}

@test "should reference image digest if specified" {
	start_crio
	crictl pull "$IMAGE_DIGEST"
	run_using_digest
}

@test "should reference image digest if specified and tag is referencing digest" {
	start_crio
	crictl pull "$IMAGE_REF"
	run_using_digest
}
