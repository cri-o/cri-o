#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	CONTAINER_INFRA_CTR_CPUSET="0" start_crio
}

function teardown() {
	cleanup_test
}

@test "test infra ctr cpuset" {
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	output=$(crictl inspectp -o yaml "$pod_id")
	[[ "$output" = *"cpus: \"0\""* ]]
}
