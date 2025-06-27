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

function prepare_stream_server() {
	setup_crio

	cat << EOF > "$CRIO_CONFIG_DIR/99-websocket.conf"
[crio.runtime]
default_runtime = "websocket"
[crio.runtime.runtimes.websocket]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_type = "pod"
monitor_path = ""
stream_websockets = true
EOF
	unset CONTAINER_DEFAULT_RUNTIME
	unset CONTAINER_RUNTIMES

	start_crio_no_setup
}

@test "conmonrs is used" {
	start_crio

	crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json

	# Validate that we actually used conmonrs
	grep -q "Using conmonrs version:" "$CRIO_LOG"
}

@test "conmonrs streaming server for exec" {
	prepare_stream_server
	POD=$(crictl runp "$TESTDATA"/sandbox_config.json)
	CTR=$(crictl create "$POD" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$CTR"

	OUTPUT=$(crictl exec -r websocket "$CTR" echo test)
	[ "$OUTPUT" == "test" ]
	grep -q "Using exec URL from container monitor" "$CRIO_LOG"
}

@test "conmonrs streaming server for attach" {
	prepare_stream_server
	POD=$(crictl runp "$TESTDATA"/sandbox_config.json)
	CTR=$(crictl create "$POD" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$CTR"

	OUTPUT=$(crictl attach -r websocket -i "$CTR")
	[[ "$OUTPUT" == *"Redis is starting"* ]]
	grep -q "Using attach URL from container monitor" "$CRIO_LOG"
}
