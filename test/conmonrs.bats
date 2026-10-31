#!/usr/bin/env bats

load helpers

function setup() {
	if [[ $RUNTIME_TYPE != pod ]]; then
		skip "not using conmonrs"
	fi

	setup_test
}

function teardown() {
	stop_port_forward
	cleanup_test
}

# stop_port_forward stops the crictl port-forward running in the background.
function stop_port_forward() {
	if [[ -z ${PORT_FORWARD_PID:-} ]]; then
		return
	fi
	# The crictl helper runs in a subshell, stop crictl itself as well.
	pkill -P "$PORT_FORWARD_PID" || true
	kill "$PORT_FORWARD_PID" 2> /dev/null || true
	wait "$PORT_FORWARD_PID" || true
	PORT_FORWARD_PID=
}

# redis_ping sends a PING to the redis server forwarded to the local port $1.
function redis_ping() {
	local fd reply=
	exec {fd}<> "/dev/tcp/127.0.0.1/$1" || return 1
	printf 'PING\r\n' >&"$fd"
	read -r -t 5 reply <&"$fd" || true
	exec {fd}>&-
	[[ $reply == "+PONG"* ]]
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

@test "conmonrs streaming server for port forward" {
	start_crio_with_websocket_stream_server
	POD=$(crictl runp "$TESTDATA"/sandbox_config.json)
	CTR=$(crictl create "$POD" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$CTR"

	# The kubelet uses SPDY, crictl can tunnel it through a WebSocket as well.
	for transport in spdy websocket; do
		PORT=$(free_port)
		# The background process must not hold the FD 3 of bats.
		crictl port-forward -r "$transport" "$POD" "$PORT:6379" > "$TESTDIR/port-forward-$transport.log" 2>&1 3>&- &
		PORT_FORWARD_PID=$!

		# Retry until crictl listens and redis accepts connections.
		retry 30 1 redis_ping "$PORT"
		stop_port_forward
	done
	grep -q "Using port forward URL from container monitor" "$CRIO_LOG"
}
