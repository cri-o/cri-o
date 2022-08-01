#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
	CONTAINER_CONMON_CGROUP="pod" CONTAINER_INFRA_CTR_CPUSET="0" CONTAINER_DROP_INFRA_CTR=false start_crio
}

function teardown() {
	cleanup_test
}

@test "test infra ctr cpuset" {
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "not yet supported by conmonrs"
	fi

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	output=$(crictl inspectp -o yaml "$pod_id")
	[[ "$output" = *"cpus: \"0\""* ]]
	check_conmon_cpuset "$pod_id" '0'

	# Ensure the container gets the appropriate taskset
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	ctr_output=$(crictl inspect -o json "$ctr_id")
	ctr_pid=$(jq -r '.info.pid' <<< "$ctr_output")
	ctr_taskset=$(taskset -p "$ctr_pid")
	[[ ${ctr_taskset#*current affinity mask: } = 1 ]]
}
