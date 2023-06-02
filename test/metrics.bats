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

@test "metrics container oom" {
	PORT=$(free_port)
	CONTAINER_ENABLE_METRICS=true CONTAINER_METRICS_PORT=$PORT start_crio

#		| .command = ["sh", "-c", "sleep 3; exit 137"]' \

	jq '	  .linux.resources.memory_limit_in_bytes = 15728640
		| .linux.resources.memory_swap_limit_in_bytes = 15728640
		| .command = ["sh", "-c", "sleep 1; for ((i=0; i<1000; i++)); do val=$(seq -w -s - $i $((i + 100000))); eval var$i=$val_$val_$val; done"]' \
		"$TESTDATA/container_config.json" > "$TESTDIR/config.json"

	for ((i = 0; i < 100; i++)); do
		echo "iter $i" >&3

		CTR_ID=$(crictl run "$TESTDIR/config.json" "$TESTDATA/sandbox_config.json")

		# Wait for container to OOM.
		EXPECTED_EXIT_STATUS=137 wait_until_exit "$CTR_ID"
		if ! crictl inspect "$CTR_ID" | jq -e '.status.reason == "OOMKilled"'; then
			# The container has exited but it was not OOM-killed.
			echo "--- crlctl inspect :: ---"
			crictl inspect --output yaml "$CTR_ID" | grep -A40 'status:'
			echo "--- --- ---"
			# Most probably it's a conmon bug.
			if [ "$RUNTIME_TYPE" == "oci" ]; then
				echo "--- conmon log :: ---"
				journalctl -t conmon --grep "${CTR_ID::20}"
				echo "--- --- ---"
			fi
			# Systemd should have caught the OOM event.
			if [[ "$CONTAINER_CGROUP_MANAGER" == "systemd" ]]; then
				echo "--- systemd log :: ---"
				journalctl --unit "crio-${CTR_ID}.scope"
				echo "--- --- ---"
			fi
			# Kernel definitely has the OOM event.
			echo "--- dmesg :: ---"
			dmesg -k --since "-60s" # -l info | grep oom-kill:
			echo "--- --- ---"

			# Alas, we have utterly failed.
			false
		fi
		cleanup_ctrs
		cleanup_pods
	done

	METRIC=$(curl -sf "http://localhost:$PORT/metrics" | grep '^container_runtime_crio_containers_oom_total')
	[[ "$METRIC" == 'container_runtime_crio_containers_oom_total 1' ]]

	METRIC=$(curl -sf "http://localhost:$PORT/metrics" | grep 'crio_containers_oom{')
	[[ "$METRIC" == 'container_runtime_crio_containers_oom{name="k8s_container1_podsandbox1_redhat.test.crio_redhat-test-crio_1"} 1' ]]
}
