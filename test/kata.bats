#!/usr/bin/env bats
# vim: set syntax=sh:

# this is a canary test for the CI job for kata: if this one fails, all the others
# are dubious, as it means they probably run without kata

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "container run with kata should have containerd-shim-kata-v2 process running" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip Not running with kata
	fi
	start_crio

	# make sure no kata process is running before we start a container
	output="$(ps --no-headers -C containerd-shim-kata-v2 | wc -l)"
	echo "$output"
	[[ "$output" == "0" ]]

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# verify that the shim is running
	[ "$(ps --no-headers -C containerd-shim-kata-v2 | wc -l)" == "1" ]

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# verify that the shim goes away with the pod
	[ "$(ps --no-headers -C containerd-shim-kata-v2 | wc -l)" == "0" ]
}
