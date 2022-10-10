#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "goroutine stability: 100-pod" {
	CONTAINER_PROFILE=true start_crio

	initial_count=$count
	for i in {1..100}; do
		sbxconfig="$TESTDIR/sbx-$i.json"
		jq --arg name "podsandbox$i" '.metadata.name = $name' "$TESTDATA"/sandbox_config.json > "$sbxconfig"
		ctrconfig="$TESTDIR/ctr-$i.json"
		jq --arg name "container$i" '.metadata.name = $name' "$TESTDATA"/container_sleep.json > "$ctrconfig"
		crictl run "$ctrconfig" "$sbxconfig"
		count=$(goroutine_count)
		echo "Count with $i pods running: $count"
		if [[ $i == 1 ]]; then
			initial_count=$count
		fi
	done

	cleanup_ctrs
	cleanup_pods

	count=$(goroutine_count)
	echo "After shutdown: $count"
	[[ $count -le $initial_count ]]
}
