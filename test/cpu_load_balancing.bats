#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	if is_cgroup_v2; then
		skip "not yet supported on cgroup2"
	fi
	export activation="cpu-load-balancing.crio.io"
	export prefix="io.openshift.workload.management"
	setup_test
	sboxconfig="$TESTDIR/sbox.json"
	ctrconfig="$TESTDIR/ctr.json"
	shares="1024"
	export cpuset="0-1"
	create_workload "$shares" "$cpuset"
}

function teardown() {
	cleanup_test
}

function create_workload() {
	local cpushares="$1"
	local cpuset="$2"
	cat << EOF > "$CRIO_CONFIG_DIR/01-workload.conf"
[crio.runtime.workloads.management]
activation_annotation = "$activation"
annotation_prefix = "$prefix"
allowed_annotations = ["$activation"]
[crio.runtime.workloads.management.resources]
cpushares =  $cpushares
cpuset = "$cpuset"
EOF
}

function check_sched_load_balance() {
	local ctr_id="$1"
	local is_enabled="$2"

	set_container_pod_cgroup_root "cpuset" "$ctr_id"
	cgroup_file="cpuset.sched_load_balance"

	[[ $(cat "$CTR_CGROUP"/"$cgroup_file") == "$is_enabled" ]]
	if [[ "$CONTAINER_DEFAULT_RUNTIME" == "crun" ]]; then
		[[ $(cat "$CTR_CGROUP"/container/"$cgroup_file") == "$is_enabled" ]]
	fi
}

# Verify the pre start runtime handler hooks run when triggered by annotation and workload.
@test "test cpu load balancing" {
	start_crio

	# first, create a container with load balancing disabled
	jq --arg act "$activation" --arg set "$cpuset" \
		' .annotations[$act] = "true"
		| .linux.resources.cpuset_cpus= $set' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq --arg act "$activation" --arg set "$cpuset" \
		' .annotations[$act] = "true"
		| .linux.resources.cpuset_cpus = $set' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")

	# check for sched_load_balance
	check_sched_load_balance "$ctr_id" 0 # disabled
}

# Verify the post stop runtime handler hooks run when a container is stopped manually.
@test "test cpu load balance disabled on manual stop" {
	start_crio

	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# check for sched_load_balance
	check_sched_load_balance "$ctr_id" 1 # enabled

	# check sched_load_balance is disabled after container stopped
	crictl stop "$ctr_id"
	check_sched_load_balance "$ctr_id" 0 # disabled
}

# Verify the post stop runtime handler hooks run when a container exits on its own.
@test "test cpu load balance disabled on container exit" {
	start_crio

	jq '	  .command = ["/bin/sh", "-c", "sleep 5 && exit 0"]' \
		"$TESTDATA"/container_config.json > "$ctrconfig"
	ctr_id=$(crictl run "$ctrconfig" "$TESTDATA"/sandbox_config.json)

	# wait until container exits naturally
	sleep 10

	# check for sched_load_balance
	check_sched_load_balance "$ctr_id" 0 # disabled
}
