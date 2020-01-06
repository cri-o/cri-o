#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	start_crio
}

function teardown() {
	cleanup_test
}

function kill_conmon() {
	ctr_id="$1"

	conmon_pid=$(pgrep -f "conmon .* -c $ctr_id ")
	if [ -z "$conmon_pid" ]; then
		skip "conmon pid found empty; probably kata containers"
	fi

	run sudo kill -9 $conmon_pid
	echo "$output"
	[ "$status" -eq 0 ]
}

function wait_and_check_for_oom() {
	attempt=0
	while [ $attempt -le 100 ]; do
		attempt=$((attempt+1))
		run crictl inspect --output yaml "$ctr_id"
		echo "$output"
		[ "$status" -eq 0 ]
		if [[ "$output" =~ "OOMKilled" ]]; then
			break
		fi
		sleep 10
	done
	[[ "$output" =~ "OOMKilled" ]]
}

function wait_and_check_for_sandbox_oom() {
	attempt=0
	while [ $attempt -le 100 ]; do
		attempt=$((attempt+1))
		run crictl inspectp --output yaml "$pod_id"
		echo "$output"
		[ "$status" -eq 0 ]
		if [[ "$output" =~ "SANDBOX_NOTREADY" ]]; then
			break
		fi
		sleep 10
	done
	[[ "$output" =~ "SANDBOX_NOTREADY" ]]
}

@test "conmonmon cleans up running conmon" {
	if [[ "$CI" == "true" ]]; then
		skip "CI container tests don't support conmonmon"
	fi
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_config_sleep.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	kill_conmon "$ctr_id"

	wait_and_check_for_oom "$ctr_id"
}

@test "conmonmon cleans up created conmon once it runs" {
	if [[ "$CI" == "true" ]]; then
		skip "CI container tests don't support conmonmon"
	fi
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_config_sleep.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	kill_conmon "$ctr_id"

	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	wait_and_check_for_oom "$ctr_id"
}

@test "conmonmon cleans up created conmon after restart" {
	if [[ "$CI" == "true" ]]; then
		skip "CI container tests don't support conmonmon"
	fi
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_config_sleep.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	restart_crio

	kill_conmon "$ctr_id"

	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	wait_and_check_for_oom "$ctr_id"
}

@test "conmonmon cleans up started conmon after restart" {
	if [[ "$CI" == "true" ]]; then
		skip "CI container tests don't support conmonmon"
	fi
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_config_sleep.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	restart_crio

	kill_conmon "$ctr_id"

	wait_and_check_for_oom "$ctr_id"
}

@test "conmonmon cleans up running sandbox conmon" {
	if [[ "$CI" == "true" ]]; then
		skip "CI container tests don't support conmonmon"
	fi
	export CONTAINER_MANAGE_NS_LIFECYCLE=false
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	kill_conmon "$pod_id"

	wait_and_check_for_sandbox_oom "$pod_id"
}

@test "conmonmon cleans up started sandbox conmon after restart" {
	if [[ "$CI" == "true" ]]; then
		skip "CI container tests don't support conmonmon"
	fi
	export CONTAINER_MANAGE_NS_LIFECYCLE=false
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	restart_crio

	kill_conmon "$pod_id"

	wait_and_check_for_sandbox_oom "$pod_id"
}
