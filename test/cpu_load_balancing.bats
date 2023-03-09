#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	export activation="cpu-load-balancing.crio.io"
	setup_test
	sboxconfig="$TESTDIR/sbox.json"
	ctrconfig="$TESTDIR/ctr.json"
}

function teardown() {
	cleanup_test
}

function check_sched_load_balance() {
	local is_enabled="$1"

	if is_cgroup_v2; then
		return
	else
		loadbalance_path="/sys/fs/cgroup/cpuset"
		loadbalance_filename="cpuset.sched_load_balance"
	fi

	loadbalance=$(cat "$loadbalance_path/pod_123.slice/$loadbalance_filename")
	[[ "$is_enabled" == *"$loadbalance"* ]]
}

@test "test cpu load balancing" {
	cpuset="0-1"

	start_crio

	# setup container with annotation
	jq --arg act "$activation" --arg set "$cpuset" \
		' .annotations[$act] = "true"
		| .linux.resources.cpuset_cpus= $set' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"


	jq --arg act "$activation" --arg set "$cpuset" \
		' .annotations[$act] = "true"
		| .linux.resources.cpuset_cpus = $set' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	# run container
	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")

	# get pid of the container process
	ctr_pid=$(crictl inspect "$ctr_id" | jq .info.pid)

	# get process affinity (cpu) list
	affinity_list=$(taskset -pc $ctr_pid | cut -d ':' -f 2 | sed -e 's/^[[:space:]]*//' | sed  's/,/-/g')
	[[ "$affinity_list" == *"$cpuset"* ]]

	# check for sched_load_balance
	check_sched_load_balance 1 # enabled
}

