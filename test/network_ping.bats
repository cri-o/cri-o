#!/usr/bin/env bats

load helpers

function setup() {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "FIXME: can't set ping_group_range inside the container"
	fi
	setup_test
	CONTAINER_DEFAULT_SYSCTLS='net.ipv4.ping_group_range=0   2147483647' start_crio
}

function teardown() {
	cleanup_test
}

@test "Ping pod from the host / another pod" {
	pod1_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr1_id=$(crictl create "$pod1_id" "$TESTDATA"/container_config_ping.json "$TESTDATA"/sandbox_config.json)
	ping_pod "$ctr1_id"

	sandbox_config="$TESTDIR"/sandbox_config.json
	jq '	  .metadata.namespace="cni_test"' \
		"$TESTDATA"/sandbox_config.json > "$sandbox_config"

	pod2_id=$(crictl runp "$sandbox_config")
	ctr2_id=$(crictl create "$pod2_id" "$TESTDATA"/container_config_ping.json "$sandbox_config")

	ping_pod_from_pod "$ctr1_id" "$ctr2_id"
	ping_pod_from_pod "$ctr2_id" "$ctr1_id"
}
