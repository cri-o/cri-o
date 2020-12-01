#!/usr/bin/env bats

load helpers

function setup() {
	if [ -z "$CONTAINER_UID_MAPPINGS" ]; then
		skip "userns testing not enabled"
	fi
	setup_test
	start_crio
}

function teardown() {
	cleanup_test
}

@test "ctr_userns run container" {
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_config_sleep.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	state=$(crictl inspect "$ctr_id")
	pid=$(echo "$state" | jq .info.pid)
	grep 100000 /proc/"$pid"/uid_map
	[ "$status" -eq 0 ]
	grep 200000 /proc/"$pid"/gid_map
	[ "$status" -eq 0 ]

	out=$(echo -e "GET /info HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	echo "$out"
	[[ "$out" == *"100000"* ]]
	[[ "$out" == *"200000"* ]]
}
