#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
}

function setup_cgroupv1_test() {
	if is_cgroup_v2; then
		skip "not yet supported on cgroup2"
	fi
	export activation="cpu-load-balancing.crio.io"
	export prefix="io.openshift.workload.management"
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
	setup_cgroupv1_test
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
	setup_cgroupv1_test
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
	setup_cgroupv1_test
	start_crio

	jq '	  .command = ["/bin/sh", "-c", "sleep 5 && exit 0"]' \
		"$TESTDATA"/container_config.json > "$ctrconfig"
	ctr_id=$(crictl run "$ctrconfig" "$TESTDATA"/sandbox_config.json)

	# wait until container exits naturally
	sleep 10

	# check for sched_load_balance
	check_sched_load_balance "$ctr_id" 0 # disabled
}

function setup_per_container_test() {
	if ! is_cgroup_v2; then
		skip "node must be configured with cgroupv2 for this test"
	fi
	if [[ "$(basename "$RUNTIME_BINARY_PATH")" != "crun" ]]; then
		skip "this test requires crun"
	fi
	if [[ $(nproc) -lt 4 ]]; then
		skip "this test requires at least 4 CPUs"
	fi
	cat << EOF > "$CRIO_CONFIG_DIR/01-high-performance.conf"
[crio.runtime]
infra_ctr_cpuset = "0-1"
[crio.runtime.runtimes.high-performance]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "$RUNTIME_TYPE"
allowed_annotations = ["cpu-load-balancing.crio.io"]
default_annotations = {"run.oci.systemd.subgroup" = ""}
EOF
}

function check_partition() {
	local ctr_id="$1"
	local expected="$2"
	set_container_pod_cgroup_root "" "$ctr_id"
	[[ $(cat "$CTR_CGROUP"/cpuset.cpus.partition) == "$expected" ]]
}

function create_container_config() {
	local name="$1"
	local outfile="$2"
	local cpuset="$3"
	jq --arg name "$name" --arg cpuset "$cpuset" \
		'.metadata.name = $name
		| .linux.resources.cpu_shares = 1024
		| .linux.resources.cpuset_cpus = $cpuset' \
		"$TESTDATA"/container_sleep.json > "$outfile"
}

# Verify per-container annotation only applies to the targeted container.
# bats test_tags=crio:serial
@test "test cpu load balancing per-container annotation targets specific container" {
	setup_per_container_test
	start_crio

	# Apply annotation to container-a
	jq '.annotations."cpu-load-balancing.crio.io/container-a" = "disable"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sbox.json

	create_container_config "container-a" "$TESTDIR"/ctr_a.json "2"
	create_container_config "container-b" "$TESTDIR"/ctr_b.json "3"

	pod_id=$(crictl runp --runtime high-performance "$TESTDIR"/sbox.json)
	ctr_a=$(crictl create "$pod_id" "$TESTDIR"/ctr_a.json "$TESTDIR"/sbox.json)
	crictl start "$ctr_a"
	ctr_b=$(crictl create "$pod_id" "$TESTDIR"/ctr_b.json "$TESTDIR"/sbox.json)
	crictl start "$ctr_b"

	# Verify only container-a is isolated
	check_partition "$ctr_a" "isolated"
	check_partition "$ctr_b" "member"
}

# Verify pod-level annotation applies to all containers.
# bats test_tags=crio:serial
@test "test cpu load balancing pod-level annotation applies to all containers" {
	setup_per_container_test
	start_crio

	# Apply annotation to pod
	jq '.annotations."cpu-load-balancing.crio.io" = "disable"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sbox.json

	create_container_config "container-a" "$TESTDIR"/ctr_a.json "2"
	create_container_config "container-b" "$TESTDIR"/ctr_b.json "3"

	pod_id=$(crictl runp --runtime high-performance "$TESTDIR"/sbox.json)
	ctr_a=$(crictl create "$pod_id" "$TESTDIR"/ctr_a.json "$TESTDIR"/sbox.json)
	crictl start "$ctr_a"

	ctr_b=$(crictl create "$pod_id" "$TESTDIR"/ctr_b.json "$TESTDIR"/sbox.json)
	crictl start "$ctr_b"

	# Verify both containers are isolated
	check_partition "$ctr_a" "isolated"
	check_partition "$ctr_b" "isolated"
}

# Verify container-specific annotation overrides pod-level.
# bats test_tags=crio:serial
@test "test cpu load balancing container annotation overrides pod-level" {
	setup_per_container_test
	start_crio

	# Apply annotation to pod, but undo it for container-b
	jq '.annotations."cpu-load-balancing.crio.io" = "disable"
		| .annotations."cpu-load-balancing.crio.io/container-b" = "enable"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sbox.json

	create_container_config "container-a" "$TESTDIR"/ctr_a.json "2"
	create_container_config "container-b" "$TESTDIR"/ctr_b.json "3"

	pod_id=$(crictl runp --runtime high-performance "$TESTDIR"/sbox.json)
	ctr_a=$(crictl create "$pod_id" "$TESTDIR"/ctr_a.json "$TESTDIR"/sbox.json)
	crictl start "$ctr_a"
	ctr_b=$(crictl create "$pod_id" "$TESTDIR"/ctr_b.json "$TESTDIR"/sbox.json)
	crictl start "$ctr_b"

	# Verify only container-a is isolated
	check_partition "$ctr_a" "isolated"
	check_partition "$ctr_b" "member"
}
