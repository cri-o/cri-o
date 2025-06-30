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
	start_crio

	CTR=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	OUTPUT=$(crictl exec -r websocket "$CTR" echo test)
	[ "$OUTPUT" == "test" ]
	grep -q "Using exec URL from conmon-rs:" "$CRIO_LOG"
}
