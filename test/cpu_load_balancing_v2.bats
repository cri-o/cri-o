#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	if ! is_cgroup_v2; then
		skip "must be configured with cgroupv2 for this test"
	fi
	setup_test
	cat << EOF > "$CRIO_CONFIG_DIR/01-high-performance.conf"
[crio.runtime.workloads.management]
activation_annotation = "cpu-load-balancing.crio.io"
allowed_annotations = ["cpu-load-balancing.crio.io"]
[crio.runtime.workloads.management.resources]
cpushares = 1024
cpuset = "0-1"
[crio.runtime.runtimes.high-performance]
runtime_path = "$RUNTIME_BINARY_PATH"
allowed_annotations = ["cpu-load-balancing.crio.io"]
default_annotations = {"run.oci.systemd.subgroup" = ""}
EOF
}

function teardown() {
	cleanup_test
}

# Verify the pre start runtime handler hooks run when triggered by annotation and workload.
@test "test cpu load balancing" {
	start_crio

	# first, create a container with load balancing disabled
	jq '.annotations["cpu-load-balancing.crio.io"] = "disable"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR/sandbox_config.json"

	jq '.annotations["cpu-load-balancing.crio.io"] = "disable"' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR/container_config.json"

	ctr_id=$(crictl run --runtime high-performance "$TESTDIR/container_config.json" "$TESTDIR/sandbox_config.json")

	set_container_pod_cgroup_root "cpuset" "$ctr_id"

	cat "$CTR_CGROUP"/"cpuset.cpus.partition"
	[[ $(cat "$CTR_CGROUP"/"cpuset.cpus.partition") == "isolated" ]]

	cat "$CTR_CGROUP"/"cpuset.cpus.exclusive"
	[[ $(cat "$CTR_CGROUP"/"cpuset.cpus.exclusive") == "0-1" ]]
}
