#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	if ! command -v crun; then
		skip "this test is supposed to run with crun"
	fi
	setup_test

}

function teardown() {
	cleanup_test
}

@test "should not specify the exec cpu affinity" {
	crun_path=$(which crun)
	cat << EOF > "$CRIO_CONFIG_DIR/01-workload.conf"
[crio.runtime.runtimes.high-performance]
runtime_path="$crun_path"
EOF
	start_crio

	ctr_id=$(crictl run --runtime high-performance "$TESTDATA/container_sleep.json" "$TESTDATA/sandbox_config.json")

	output=$(crictl inspect "$ctr_id" | jq -r .info.runtimeSpec.process.execCPUAffinity)
	[ "$output" = "null" ]
}

@test "should specify the exec cpu affinity when the container only uses exclusive cpus" {
	crun_path=$(which crun)
	cat << EOF > "$CRIO_CONFIG_DIR/01-workload.conf"
[crio.runtime.runtimes.high-performance]
runtime_path="$crun_path"
exec_cpu_affinity = "first"
EOF
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
}

@test "should specify shared cpu as the exec cpu affinity when the container uses both exclusive cpus and shared cpus" {
	crun_path=$(which crun)
	cat << EOF > "$CRIO_CONFIG_DIR/01-workload.conf"
[crio.runtime]
shared_cpuset = "2-3"
[crio.runtime.runtimes.high-performance]
runtime_path="$crun_path"
exec_cpu_affinity = "first"
EOF
	start_crio
	jq '
	.linux.resources.cpu_shares = 2048 |
	.linux.resources.cpuset_cpus = "0-1"
	' \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container_config.json"
	ctr_id=$(crictl run --runtime high-performance "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")

	output=$(crictl inspect "$ctr_id" | jq -r .info.runtimeSpec.process.execCPUAffinity.initial)
	echo "$output"
	[ "$output" = "2" ]
}
