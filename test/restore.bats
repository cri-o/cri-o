#!/usr/bin/env bats

load helpers

function teardown() {
	cleanup_test
}

@test "crio restore" {
	start_crio
	run crioctl pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crioctl pod list --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	pod_list_info="$output"

	run crioctl pod status --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	pod_status_info="$output"

	run crioctl ctr create --config "$TESTDATA"/container_config.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crioctl ctr list --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_list_info="$output"

	run crioctl ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_status_info="$output"

	stop_crio

	start_crio
	run crioctl pod list
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" != "" ]]
	[[ "${output}" =~ "${pod_id}" ]]

	run crioctl pod list --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" == "${pod_list_info}" ]]

	run crioctl pod status --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" == "${pod_status_info}" ]]

	run crioctl ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" != "" ]]
	[[ "${output}" =~ "${ctr_id}" ]]

	run crioctl ctr list --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" == "${ctr_list_info}" ]]

	run crioctl ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" == "${ctr_status_info}" ]]

	cleanup_ctrs
	cleanup_pods
	stop_crio
}

@test "crio restore with bad state" {
	start_crio
	run crioctl pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crioctl pod status --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" =~ "SANDBOX_READY" ]]

	run crioctl ctr create --config "$TESTDATA"/container_config.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crioctl ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" =~ "CONTAINER_CREATED" ]]

	stop_crio

	# simulate reboot with runc state going away
	for i in $("$RUNTIME" list -q | xargs); do "$RUNTIME" delete -f $i; done

	start_crio
	run crioctl pod list
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" != "" ]]
	[[ "${output}" =~ "${pod_id}" ]]

	run crioctl pod status --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" =~ "SANDBOX_NOTREADY" ]]

	run crioctl ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" != "" ]]
	[[ "${output}" =~ "${ctr_id}" ]]

	run crioctl ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" =~ "CONTAINER_EXITED" ]]
	[[ "${output}" =~ "Exit Code: 255" ]]

	cleanup_ctrs
	cleanup_pods
	stop_crio
}
