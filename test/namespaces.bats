#!/usr/bin/env bats

load helpers

function teardown() {
	cleanup_test
}

@test "pid namespace mode pod test" {
	start_crio

	newconfigsandbox=$(mktemp --tmpdir crio-config.XXXXXX.json)
	cp "$TESTDATA"/sandbox_pidnamespacemode_config.json "$newconfigsandbox"
	sed -i 's|"%pidmode%"|0|' "$newconfigsandbox"

	run crictl runp "$newconfigsandbox"
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis_namespace.json "$newconfigsandbox"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" cat /proc/1/cmdline
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ pause ]]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_crio
}

@test "pid namespace mode container with init" {
	start_crio_init

	newconfigsandbox=$(mktemp --tmpdir crio-config.XXXXXX.json)
	cp "$TESTDATA"/sandbox_pidnamespacemode_config.json "$newconfigsandbox"
	sed -i 's|"%pidmode%"|1|' "$newconfigsandbox"

	run crictl runp "$newconfigsandbox"
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_redis_namespace.json "$newconfigsandbox"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" ps | grep -v 'init' | grep ps | awk '{print $1}'
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ 1 ]]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_crio
}
