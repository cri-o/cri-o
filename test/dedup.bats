#!/usr/bin/env bats

load helpers

IMAGE=quay.io/crio/pause
IMAGE2=quay.io/crio/fedora-crio-ci:latest
IMAGE3=quay.io/crio/alpine:3.9
IMAGE4=quay.io/saschagrunert/hello-world

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "dedup: crio dedup command succeeds with populated storage" {
	start_crio

	crictl pull "$IMAGE"

	stop_crio

	run "$CRIO_BINARY_PATH" \
		-c "$CRIO_CONFIG" \
		-d "$CRIO_CONFIG_DIR" \
		dedup

	if [[ "$output" == *"storage deduplication not supported on current filesystem"* ]]; then
		skip "filesystem does not support reflinks"
	fi

	[ "$status" -eq 0 ]
	[[ "$output" == *"Starting storage deduplication"* ]]
	[[ "$output" == *"Storage deduplication complete"* ]]
}

@test "dedup: crio dedup with --physical-disk-usage shows savings" {
	start_crio

	crictl pull "$IMAGE"
	crictl pull "$IMAGE2"
	crictl pull "$IMAGE3"
	crictl pull "$IMAGE4"

	stop_crio

	run "$CRIO_BINARY_PATH" \
		-c "$CRIO_CONFIG" \
		-d "$CRIO_CONFIG_DIR" \
		dedup --physical-disk-usage

	if [[ "$output" == *"storage deduplication not supported on current filesystem"* ]]; then
		skip "filesystem does not support reflinks"
	fi

	[ "$status" -eq 0 ]
	[[ "$output" == *"Starting storage deduplication"* ]]
	[[ "$output" == *"Storage deduplication complete"* ]]
	[[ "$output" == *"Measuring physical disk usage before deduplication"* ]]
	[[ "$output" == *"Measuring physical disk usage after deduplication"* ]]

	before_line=$(echo "$output" | grep "Physical disk usage before dedup:")
	after_line=$(echo "$output" | grep "Physical disk usage after dedup:")

	[ -n "$before_line" ]
	[ -n "$after_line" ]

	if [[ "$output" == *"No space savings detected"* ]]; then
		skip "no deduplication savings (filesystem may not support reflinks or no duplicate data found)"
	fi

	[[ "$output" == *"Space saved by deduplication"* ]]

	savings_line=$(echo "$output" | grep "Space saved by deduplication:")
	[ -n "$savings_line" ]
}

@test "dedup: server remains functional after dedup on populated storage" {
	start_crio

	crictl pull "$IMAGE"
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	stop_crio

	run "$CRIO_BINARY_PATH" \
		-c "$CRIO_CONFIG" \
		-d "$CRIO_CONFIG_DIR" \
		dedup

	if [[ "$output" == *"storage deduplication not supported on current filesystem"* ]]; then
		skip "filesystem does not support reflinks"
	fi

	[ "$status" -eq 0 ]

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}
