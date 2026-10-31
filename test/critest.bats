#!/usr/bin/env bats

load helpers

# critest retries flaky specs itself, retrying the whole suite takes too long.
# shellcheck disable=SC2034
BATS_TEST_RETRIES=0

function setup() {
	if [[ "$RUN_CRITEST" != "1" ]]; then
		skip "critest because RUN_CRITEST is not set"
	fi

	setup_test
}

function teardown() {
	cleanup_test
}

@test "run the critest suite" {
	WEBSOCKET_ARGS=()
	if [[ $RUNTIME_TYPE == pod ]]; then
		start_crio_with_websocket_stream_server
		WEBSOCKET_ARGS=(-websocket-attach -websocket-exec)
	else
		start_crio
	fi

	# TODO: Remove the metrics test skip after https://github.com/cri-o/cri-o/pull/10420.
	critest \
		--runtime-endpoint "unix://${CRIO_SOCKET}" \
		--image-endpoint "unix://${CRIO_SOCKET}" \
		--nri-socket "$NRI_SOCKET" \
		--parallel="$(nproc)" \
		--ginkgo.randomize-all \
		--ginkgo.skip 'runtime should support (returning metrics descriptors|listing pod sandbox metrics)' \
		--ginkgo.timeout 5m \
		--ginkgo.trace \
		--ginkgo.flake-attempts 3 \
		"${WEBSOCKET_ARGS[@]}" >&3

	if [[ $RUNTIME_TYPE == pod ]]; then
		# Validate that we actually used conmonrs
		grep -q "Using conmonrs version:" "$CRIO_LOG"
	fi
}
