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
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	state=$(crictl inspect "$ctr_id")
	pid=$(echo "$state" | jq .info.pid)
	grep 100000 /proc/"$pid"/uid_map
	grep 200000 /proc/"$pid"/gid_map

	out=$(echo -e "GET /info HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	[[ "$out" == *"100000"* ]]
	[[ "$out" == *"200000"* ]]
}

# This test follows a similar pattern to the one above but adds
# an adds extra step of crio restart.
@test "ctr_userns mappings persist after crio restart" {
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	restart_crio
	crictl inspectp "$pod_id" | jq -e '.status.state == "SANDBOX_READY"'
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	state=$(crictl inspect "$ctr_id")
	pid=$(echo "$state" | jq .info.pid)
	grep 100000 /proc/"$pid"/uid_map
	grep 200000 /proc/"$pid"/gid_map
}

@test "ctr_userns mappings persist after container kill and restart" {
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	restart_crio

	runtime delete -f "$ctr_id"
	crictl rm "$ctr_id"

	new_ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$new_ctr_id"

	state=$(crictl inspect "$new_ctr_id")
	pid=$(echo "$state" | jq .info.pid)
	grep 100000 /proc/"$pid"/uid_map
	grep 200000 /proc/"$pid"/gid_map
}
