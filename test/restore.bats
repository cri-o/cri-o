#!/usr/bin/env bats

load helpers

function teardown() {
	cleanup_test
}

@test "ocid restore" {
	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run ocic pod list --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	pod_list_info="$output"

	run ocic pod status --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	pod_status_info="$output"

	run ocic ctr create --config "$TESTDATA"/container_config.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run ocic ctr list --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_list_info="$output"

	run ocic ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_status_info="$output"

	stop_ocid

	start_ocid
	run ocic pod list
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" != "" ]]
	[[ "${output}" =~ "${pod_id}" ]]

	run ocic pod list --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" == "${pod_list_info}" ]]

	run ocic pod status --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" == "${pod_status_info}" ]]

	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" != "" ]]
	[[ "${output}" =~ "${pod_id}" ]]

	run ocic ctr list --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" == "${ctr_list_info}" ]]

	run ocic ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" == "${ctr_status_info}" ]]

	cleanup_ctrs
	cleanup_pods
	stop_ocid
}
