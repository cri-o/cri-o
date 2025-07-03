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
	setup_crio

	cat << EOF > "$CRIO_CONFIG_DIR/99-websocket.conf"
[crio.runtime]
default_runtime = "websocket"
[crio.runtime.runtimes.websocket]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_type = "pod"
stream_websockets = true
EOF
	unset CONTAINER_DEFAULT_RUNTIME
	unset CONTAINER_RUNTIMES

	start_crio_no_setup

	CTR=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	OUTPUT=$(crictl exec -r websocket "$CTR" echo test)
	[ "$OUTPUT" == "test" ]
	grep -q "Using exec URL from container monitor " "$CRIO_LOG"
}
