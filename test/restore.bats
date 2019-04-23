#!/usr/bin/env bats

load helpers

function teardown() {
	cleanup_test
}

@test "crio restore" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl pods --quiet --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	pod_list_info="$output"

	run crictl inspectp --output=table "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	pod_status_info=`echo "$output" | grep Status`
	pod_ip=`echo "$output" | grep IP`

	run crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl ps --quiet --id "$ctr_id" --all
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_list_info="$output"

	run crictl inspect "$ctr_id" --output table
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_status_info=`echo "$output" | grep State`

	stop_crio

	start_crio
	run crictl pods --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" != "" ]]
	[[ "${output}" == "${pod_id}" ]]

	run crictl pods --quiet --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" == "${pod_list_info}" ]]

	run crictl inspectp --output=table "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	status_output=`echo "$output" | grep Status`
	ip_output=`echo "$output" | grep IP`
	[[ "${status_output}" == "${pod_status_info}" ]]
	[[ "${ip_output}" == "${pod_ip}" ]]

	run crictl ps --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" != "" ]]
	[[ "${output}" == "${ctr_id}" ]]

	run crictl ps --quiet --id "$ctr_id" --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" == "${ctr_list_info}" ]]

	run crictl inspect "$ctr_id" --output table
	echo "$output"
	[ "$status" -eq 0 ]
	output=`echo "$output" | grep State`
	[[ "${output}" == "${ctr_status_info}" ]]

	cleanup_ctrs
	cleanup_pods
	stop_crio
}

@test "crio restore with bad state and pod stopped" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	stop_crio

	# simulate reboot with runc state going away
	"$RUNTIME" delete -f "$pod_id"

	start_crio

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	cleanup_pods
	stop_crio
}

@test "crio restore with bad state and ctr stopped" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl stop "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	stop_crio

	# simulate reboot with runc state going away
	"$RUNTIME" delete -f "$pod_id"
	"$RUNTIME" delete -f "$ctr_id"

	start_crio

	run crictl stop "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	cleanup_ctrs
	cleanup_pods
	stop_crio
}

@test "crio restore with bad state and ctr removed" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl stop "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl rm "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	stop_crio

	# simulate reboot with runc state going away
	"$RUNTIME" delete -f "$pod_id"
	"$RUNTIME" delete -f "$ctr_id"

	start_crio

	run crictl stop "$ctr_id"
	echo "$output"
	[ "$status" -eq 1 ]
	[[ "${output}" =~ "not found" ]]

	cleanup_ctrs
	cleanup_pods
	stop_crio
}

@test "crio restore with bad state and pod removed" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	stop_crio

	# simulate reboot with runc state going away
	"$RUNTIME" delete -f "$pod_id"

	start_crio

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	cleanup_pods
	stop_crio
}

@test "crio restore with bad state" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl inspectp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" =~ "SANDBOX_READY" ]]

	run crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl inspect "$ctr_id" --output table
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" =~ "CONTAINER_CREATED" ]]

	stop_crio

	# simulate reboot with runc state going away
	"$RUNTIME" delete -f "$pod_id"
	"$RUNTIME" delete -f "$ctr_id"

	start_crio
	run crictl pods --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" != "" ]]
	[[ "${output}" =~ "${pod_id}" ]]

	run crictl inspectp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" =~ "SANDBOX_NOTREADY" ]]

	run crictl ps --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" != "" ]]
	[[ "${output}" =~ "${ctr_id}" ]]

	run crictl inspect "$ctr_id" --output table
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" =~ "CONTAINER_EXITED" ]]
	# TODO: may be cri-tool should display Exit Code
	#[[ "${output}" =~ "Exit Code: 255" ]]

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

@test "crio restore with missing config.json" {
	start_crio

	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	stop_crio

	# simulate reboot with runtime state and config.json going away
	"$RUNTIME" delete -f "$pod_id"
	"$RUNTIME" delete -f "$ctr_id"
	find $TESTDIR/ -name config.json -exec rm \{\} \;
	find $TESTDIR/ -name shm -exec umount -l \{\} \;

	start_crio

	run crictl inspect "$ctr_id"
	echo "$output"
	[ "$status" -ne 0 ]

	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

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
