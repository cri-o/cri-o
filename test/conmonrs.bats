#!/usr/bin/env bats

load helpers

function setup() {
	if [[ $RUNTIME_TYPE != pod ]]; then
		skip "not using conmonrs"
	fi

	setup_test
}

function teardown() {
	cleanup_test
}

@test "conmonrs is used" {
	start_crio

	crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json

	# Validate that we actually used conmonrs
	grep -q "Using conmonrs version:" "$CRIO_LOG"
}

@test "conmonrs streaming server for exec" {
	start_crio_with_websocket_stream_server
	POD=$(crictl runp "$TESTDATA"/sandbox_config.json)
	CTR=$(crictl create "$POD" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$CTR"

	OUTPUT=$(crictl exec -r websocket "$CTR" echo test)
	[ "$OUTPUT" == "test" ]
	grep -q "Using exec URL from container monitor" "$CRIO_LOG"
}

@test "conmonrs streaming server for attach" {
	start_crio_with_websocket_stream_server
	POD=$(crictl runp "$TESTDATA"/sandbox_config.json)
	# Attach only forwards the output written while attached. With stdin once,
	# the stdin EOF of the client closes the container stdin and the attach
	# keeps forwarding the output written afterwards until the container ends.
	jq '.command = ["/bin/sh", "-c", "cat && echo stdin closed"] | .args = [] | .stdin = true | .stdin_once = true' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/container_attach.json
	CTR=$(crictl create "$POD" "$TESTDIR"/container_attach.json "$TESTDATA"/sandbox_config.json)
	crictl start "$CTR"

	OUTPUT=$(echo attached | crictl attach -r websocket -i "$CTR")
	[[ "$OUTPUT" == *"attached"* ]]
	[[ "$OUTPUT" == *"stdin closed"* ]]
	grep -q "Using attach URL from container monitor" "$CRIO_LOG"
}
