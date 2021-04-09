#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "metrics with default port" {
	# start crio with default port 9090
	PORT="9090"
	CONTAINER_ENABLE_METRICS=true start_crio
	if ! port_listens "$PORT"; then
		skip "Metrics port $PORT not listening"
	fi

	# get metrics
	curl -sf "http://localhost:$PORT/metrics"
}

@test "metrics with random port" {
	# start crio with custom port
	PORT=$(free_port)
	CONTAINER_ENABLE_METRICS=true CONTAINER_METRICS_PORT=$PORT start_crio

	crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json

	# get metrics
	curl -sf "http://localhost:$PORT/metrics" | grep crio_operations
}

@test "metrics container oom" {
	PORT=$(free_port)
	CONTAINER_ENABLE_METRICS=true CONTAINER_METRICS_PORT=$PORT start_crio

	jq '.image.image = "quay.io/crio/oom"
        | .linux.resources.memory_limit_in_bytes = 25165824
        | .command = ["/oom"]' \
		"$TESTDATA/container_config.json" > "$TESTDIR/config.json"
	CTR_ID=$(crictl run "$TESTDIR/config.json" "$TESTDATA/sandbox_config.json")

	# Wait for container to OOM
	CNT=0
	while [ $CNT -le 100 ]; do
		CNT=$((CNT + 1))
		OUTPUT=$(crictl inspect --output yaml "$CTR_ID")
		if [[ "$OUTPUT" == *"OOMKilled"* ]]; then
			break
		fi
		sleep 10
	done
	[[ "$OUTPUT" == *"OOMKilled"* ]]

	METRIC=$(curl -sf "http://localhost:$PORT/metrics" | grep '^container_runtime_crio_containers_oom_total')
	[[ "$METRIC" == 'container_runtime_crio_containers_oom_total 1' ]]

	METRIC=$(curl -sf "http://localhost:$PORT/metrics" | grep 'crio_containers_oom{')
	[[ "$METRIC" == 'container_runtime_crio_containers_oom{name="k8s_container1_podsandbox1_redhat.test.crio_redhat-test-crio_1"} 1' ]]
}
