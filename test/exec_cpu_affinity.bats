#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	skip_if_vm_runtime
	if ! command -v crun; then
		skip "this test is supposed to run with crun"
	fi
	setup_test
	cat << EOF > "$CRIO_CONFIG_DIR/01-high-performance.conf"
[crio.runtime]
shared_cpuset = "2-3"
[crio.runtime.runtimes.high-performance]
runtime_path="$RUNTIME_BINARY_PATH"
exec_cpu_affinity = "first"
allowed_annotations = ["cpu-load-balancing.crio.io", "cpu-shared.crio.io"]
default_annotations = {"run.oci.systemd.subgroup" = ""}
EOF
}

function teardown() {
	cleanup_test
}

# bats test_tags=crio:serial
@test "should not specify the exec cpu affinity" {
	start_crio

	ctr_id=$(crictl run --runtime high-performance "$TESTDATA/container_sleep.json" "$TESTDATA/sandbox_config.json")

	output=$(crictl inspect "$ctr_id" | jq -r .info.runtimeSpec.process.execCPUAffinity)
	[ "$output" = "null" ]
}

#/sys/fs/cgroup/pod_123.slice/pod_123-456.slice/crio-<ctr_id>.scope/
#├── cpuset.cpus:                      0-1
#├── cpuset.cpus.effective:            0-1
#├── cpuset.cpus.exclusive:            (empty)
#├── cpuset.cpus.exclusive.effective:  (empty)
#├── cpuset.cpus.partition:            member
#│
#└── exec/
#    ├── cpuset.cpus:                      0
#    ├── cpuset.cpus.effective:            0
#    ├── cpuset.cpus.exclusive:            (empty)
#    ├── cpuset.cpus.exclusive.effective:  (empty)
#    └── cpuset.cpus.partition:            member
#
# bats test_tags=crio:serial
@test "should run exec with the proper CPU affinity when the container only uses exclusive cpus" {
	start_crio
	jq '
	.linux.resources.cpu_shares = 2048 |
	.linux.resources.cpuset_cpus = "0-1"
	' \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container_config.json"
	ctr_id=$(crictl run --runtime high-performance "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")

	output=$(crictl inspect "$ctr_id" | jq -r .info.runtimeSpec.process.execCPUAffinity.initial)
	echo "$output"
	[ "$output" = "0" ]

	output=$(crictl exec "$ctr_id" grep "Cpus_allowed_list" /proc/self/status | awk '{print $2}')
	echo "$output"
	[ "$output" = "0" ]

	output=$(crictl exec --sync "$ctr_id" grep "Cpus_allowed_list" /proc/self/status | awk '{print $2}')
	echo "$output"
	[ "$output" = "0" ]
}

#/sys/fs/cgroup/pod_123.slice/pod_123-456.slice/crio-<ctr_id>.scope/
#├── cpuset.cpus:                      0-3
#├── cpuset.cpus.effective:            0-3
#├── cpuset.cpus.exclusive:            (empty)
#├── cpuset.cpus.exclusive.effective:  (empty)
#├── cpuset.cpus.partition:            member
#│
#└── cgroup-child/
#    ├── cpuset.cpus:                      0-1
#    ├── cpuset.cpus.effective:            0-1
#    ├── cpuset.cpus.exclusive:            (empty)
#    ├── cpuset.cpus.exclusive.effective:  (empty)
#    └── cpuset.cpus.partition:            member
#
# bats test_tags=crio:serial
@test "should run exec with the proper CPU affinity when the container uses both exclusive cpus and shared cpus" {
	start_crio
	jq '
	.linux.resources.cpu_shares = 2048 |
	.linux.resources.cpuset_cpus = "0-1"
	' "$TESTDATA/container_sleep.json" > "$TESTDIR/container_config.json"
	jq '
	.annotations."cpu-shared.crio.io/podsandbox-sleep" = "enable"
	' "$TESTDATA/sandbox_config.json" > "$TESTDIR/sandbox_config.json"
	ctr_id=$(crictl run --runtime high-performance "$TESTDIR/container_config.json" "$TESTDIR/sandbox_config.json")

	output=$(crictl inspect "$ctr_id" | jq -r .info.runtimeSpec.process.execCPUAffinity.initial)
	echo "$output"
	[ "$output" = "2" ]

	output=$(crictl exec "$ctr_id" grep "Cpus_allowed_list" /proc/self/status | awk '{print $2}')
	echo "$output"
	[ "$output" = "2" ]
	output=$(crictl exec --sync "$ctr_id" grep "Cpus_allowed_list" /proc/self/status | awk '{print $2}')
	[ "$output" = "2" ]
}

#/sys/fs/cgroup/pod_123.slice/pod_123-456.slice/crio-<ctr_id>.scope/
#├── cpuset.cpus:                      0-1
#├── cpuset.cpus.effective:            0-1
#├── cpuset.cpus.exclusive:            0-1
#├── cpuset.cpus.exclusive.effective:  0-1
#├── cpuset.cpus.partition:            isolated
#│
#└── exec/
#    ├── cpuset.cpus:                      0
#    ├── cpuset.cpus.effective:            0
#    ├── cpuset.cpus.exclusive:            (empty)
#    ├── cpuset.cpus.exclusive.effective:  (empty)
#    └── cpuset.cpus.partition:            member
#
# bats test_tags=crio:serial
@test "should run exec with the proper CPU affinity for exclusive cpus with cpu-load-balancing disabled" {
	start_crio

	# Create container config with exclusive CPUs
	jq '
	.linux.resources.cpu_shares = 2048 |
	.linux.resources.cpuset_cpus = "0-1"
	' "$TESTDATA/container_sleep.json" > "$TESTDIR/container_config.json"

	# Create sandbox config with cpu-load-balancing disabled annotation
	jq '
	.annotations."cpu-load-balancing.crio.io" = "disable"
	' "$TESTDATA/sandbox_config.json" > "$TESTDIR/sandbox_config.json"

	ctr_id=$(crictl run --runtime high-performance "$TESTDIR/container_config.json" "$TESTDIR/sandbox_config.json")

	# Verify container was created successfully
	output=$(crictl inspect "$ctr_id" | jq -r .status.state)
	echo "Container state: $output"
	[ "$output" = "CONTAINER_RUNNING" ]

	output=$(crictl inspect "$ctr_id" | jq -r .info.runtimeSpec.process.execCPUAffinity.initial)
	echo "Exec CPU affinity: $output"
	[ "$output" = "0" ]

	output=$(crictl exec "$ctr_id" grep "Cpus_allowed_list" /proc/self/status | awk '{print $2}')
	[ "$output" = "0" ]
	output=$(crictl exec --sync "$ctr_id" grep "Cpus_allowed_list" /proc/self/status | awk '{print $2}')
	[ "$output" = "0" ]
}

#/sys/fs/cgroup/pod_123.slice/pod_123-456.slice/crio-<ctr_id>.scope/
#├── cpuset.cpus:                      0-3
#├── cpuset.cpus.effective:            2-3
#├── cpuset.cpus.exclusive:            0-1
#├── cpuset.cpus.exclusive.effective:  0-1
#├── cpuset.cpus.partition:            member
#│
#└── cgroup-child/
#    ├── cpuset.cpus:                      0-1
#    ├── cpuset.cpus.effective:            0-1
#    ├── cpuset.cpus.exclusive:            0-1
#    ├── cpuset.cpus.exclusive.effective:  0-1
#    └── cpuset.cpus.partition:            isolated
#
# bats test_tags=crio:serial
@test "should run exec with the proper CPU affinity for exclusive cpus and shared cpus with cpu-load-balancing disabled" {
	start_crio

	# Create container config with exclusive CPUs
	jq '
	.linux.resources.cpu_shares = 2048 |
	.linux.resources.cpuset_cpus = "0-1"
	' "$TESTDATA/container_sleep.json" > "$TESTDIR/container_config.json"

	# Create sandbox config with cpu-load-balancing disabled annotation
	jq '
	.annotations."cpu-load-balancing.crio.io" = "disable" |
	.annotations."cpu-shared.crio.io/podsandbox-sleep" = "enable"
	' "$TESTDATA/sandbox_config.json" > "$TESTDIR/sandbox_config.json"

	ctr_id=$(crictl run --runtime high-performance "$TESTDIR/container_config.json" "$TESTDIR/sandbox_config.json")

	# Verify container was created successfully
	output=$(crictl inspect "$ctr_id" | jq -r .status.state)
	echo "Container state: $output"
	[ "$output" = "CONTAINER_RUNNING" ]

	output=$(crictl inspect "$ctr_id" | jq -r .info.runtimeSpec.process.execCPUAffinity.initial)
	echo "Exec CPU affinity: $output"
	[ "$output" = "2" ]

	output=$(crictl exec "$ctr_id" grep "Cpus_allowed_list" /proc/self/status | awk '{print $2}')
	[ "$output" = "2" ]
	output=$(crictl exec --sync "$ctr_id" grep "Cpus_allowed_list" /proc/self/status | awk '{print $2}')
	[ "$output" = "2" ]
}

# bats test_tags=crio:serial
@test "should run exec with the proper CPU affinity when the container only uses exclusive cpus with infra_ctr_cpuset" {
	cat << EOF > "$CRIO_CONFIG_DIR/01-high-performance.conf"
[crio.runtime]
infra_ctr_cpuset = "1"
shared_cpuset = "2-3"
[crio.runtime.runtimes.high-performance]
runtime_path="$RUNTIME_BINARY_PATH"
exec_cpu_affinity = "first"
allowed_annotations = ["cpu-load-balancing.crio.io", "cpu-shared.crio.io"]
default_annotations = {"run.oci.systemd.subgroup" = ""}
EOF
	start_crio
	jq '
	.linux.resources.cpu_shares = 2048 |
	.linux.resources.cpuset_cpus = "0"
	' \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container_config.json"
	ctr_id=$(crictl run --runtime high-performance "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")

	output=$(crictl inspect "$ctr_id" | jq -r .info.runtimeSpec.process.execCPUAffinity.initial)
	echo "$output"
	[ "$output" = "0" ]

	output=$(crictl exec "$ctr_id" grep "Cpus_allowed_list" /proc/self/status | awk '{print $2}')
	echo "$output"
	[ "$output" = "0" ]

	output=$(crictl exec --sync "$ctr_id" grep "Cpus_allowed_list" /proc/self/status | awk '{print $2}')
	echo "$output"
	[ "$output" = "0" ]
}

# bats test_tags=crio:serial
@test "should run exec with the proper CPU affinity when the container uses both exclusive cpus and shared cpus with infra_ctr_cpuset" {
	cat << EOF > "$CRIO_CONFIG_DIR/01-high-performance.conf"
[crio.runtime]
infra_ctr_cpuset = "1"
shared_cpuset = "2-3"
[crio.runtime.runtimes.high-performance]
runtime_path="$RUNTIME_BINARY_PATH"
exec_cpu_affinity = "first"
allowed_annotations = ["cpu-load-balancing.crio.io", "cpu-shared.crio.io"]
default_annotations = {"run.oci.systemd.subgroup" = ""}
EOF
	start_crio
	jq '
	.linux.resources.cpu_shares = 2048 |
	.linux.resources.cpuset_cpus = "0"
	' "$TESTDATA/container_sleep.json" > "$TESTDIR/container_config.json"
	jq '
	.annotations."cpu-shared.crio.io/podsandbox-sleep" = "enable"
	' "$TESTDATA/sandbox_config.json" > "$TESTDIR/sandbox_config.json"
	ctr_id=$(crictl run --runtime high-performance "$TESTDIR/container_config.json" "$TESTDIR/sandbox_config.json")

	output=$(crictl inspect "$ctr_id" | jq -r .info.runtimeSpec.process.execCPUAffinity.initial)
	echo "$output"
	[ "$output" = "2" ]

	output=$(crictl exec "$ctr_id" grep "Cpus_allowed_list" /proc/self/status | awk '{print $2}')
	echo "$output"
	[ "$output" = "2" ]
	output=$(crictl exec --sync "$ctr_id" grep "Cpus_allowed_list" /proc/self/status | awk '{print $2}')
	[ "$output" = "2" ]
}
