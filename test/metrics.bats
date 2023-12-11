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

@test "metrics with operations quantile" {
	# start crio with custom port
	PORT=$(free_port)
	CONTAINER_ENABLE_METRICS=true CONTAINER_METRICS_PORT=$PORT start_crio

	for ((i = 0; i < 100; i++)); do
		crictl version
	done

	# get metrics
	curl -sf "http://localhost:$PORT/metrics" | grep 'container_runtime_crio_operations_latency_microseconds_total{operation_type="Version",quantile="0.5"}'
	curl -sf "http://localhost:$PORT/metrics" | grep 'container_runtime_crio_operations_latency_microseconds_total{operation_type="Version",quantile="0.9"}'
	curl -sf "http://localhost:$PORT/metrics" | grep 'container_runtime_crio_operations_latency_microseconds_total{operation_type="Version",quantile="0.99"}'
}

@test "secure metrics with random port" {
	openssl req -new -newkey rsa:4096 -days 365 -nodes -x509 \
		-subj "/C=US/ST=State/L=City/O=Org/CN=Name" \
		-keyout "$TESTDIR/key.pem" \
		-out "$TESTDIR/cert.pem"

	# start crio with custom port
	PORT=$(free_port)

	CONTAINER_ENABLE_METRICS=true \
		CONTAINER_METRICS_CERT="$TESTDIR/cert.pem" \
		CONTAINER_METRICS_KEY="$TESTDIR/key.pem" \
		CONTAINER_METRICS_PORT=$PORT \
		start_crio

	crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json

	# get metrics
	curl -sfk "https://localhost:$PORT/metrics" | grep crio_operations

	# remove the watched cert
	rm "$TESTDIR/cert.pem"

	# serving metrics should still work
	curl -sfk "https://localhost:$PORT/metrics" | grep crio_operations
}

@test "secure metrics with random port and missing cert/key" {
	# start crio with custom port
	PORT=$(free_port)

	CONTAINER_ENABLE_METRICS=true \
		CONTAINER_METRICS_CERT="$TESTDIR/sub/dir/cert.pem" \
		CONTAINER_METRICS_KEY="$TESTDIR/another/dir/key.pem" \
		CONTAINER_METRICS_PORT=$PORT \
		start_crio

	crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json

	# get metrics
	curl -sfk "https://localhost:$PORT/metrics" | grep crio_operations
}

# TODO: deflake and re-enable the test
#@test "metrics container oom" {
#	PORT=$(free_port)
#	CONTAINER_ENABLE_METRICS=true CONTAINER_METRICS_PORT=$PORT start_crio
#
#	jq '.linux.resources.memory_limit_in_bytes = 15728640
#        | .command = ["sh", "-c", "sleep 5; dd if=/dev/zero of=/dev/null bs=20M"]' \
#		"$TESTDATA/container_config.json" > "$TESTDIR/config.json"
#	CTR_ID=$(crictl run "$TESTDIR/config.json" "$TESTDATA/sandbox_config.json")
#
#	# Wait for container to OOM.
#	EXPECTED_EXIT_STATUS=137 wait_until_exit "$CTR_ID"
#	if ! crictl inspect "$CTR_ID" | jq -e '.status.reason == "OOMKilled"'; then
#		# The container has exited but it was not OOM-killed.
#		# Provide some details to debug the issue.
#		echo "--- crictl inspect :: ---"
#		crictl inspect --output yaml "$CTR_ID" | grep -A40 'status:'
#		echo "--- --- ---"
#		# Most probably it's a conmon bug.
#		if [ "$RUNTIME_TYPE" == "oci" ]; then
#			echo "--- conmon log :: ---"
#			journalctl -t conmon --grep "${CTR_ID::20}"
#			echo "--- --- ---"
#		fi
#		# Systemd should have caught the OOM event.
#		if [[ "$CONTAINER_CGROUP_MANAGER" == "systemd" ]]; then
#			echo "--- systemd log :: ---"
#			journalctl --unit "crio-${CTR_ID}.scope"
#			echo "--- --- ---"
#		fi
#
#		# Alas, we have utterly failed.
#		false
#	fi
#
#	METRIC=$(curl -sf "http://localhost:$PORT/metrics" | grep '^container_runtime_crio_containers_oom_total')
#	[[ "$METRIC" == 'container_runtime_crio_containers_oom_total 1' ]]
#
#	METRIC=$(curl -sf "http://localhost:$PORT/metrics" | grep 'crio_containers_oom{')
#	[[ "$METRIC" == 'container_runtime_crio_containers_oom{name="k8s_container1_podsandbox1_redhat.test.crio_redhat-test-crio_1"} 1' ]]
#}
