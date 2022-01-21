#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "crio restore" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	pod_list_info=$(crictl pods --quiet --id "$pod_id")

	output=$(crictl inspectp -o json "$pod_id")
	[[ "$output" != "" ]]
	pod_status_info=$(echo "$output" | jq ".status.state")
	pod_ip=$(echo "$output" | jq ".status.ip")
	pod_created_at=$(echo "$output" | jq ".status.createdAt")

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	ctr_list_info=$(crictl ps --quiet --id "$ctr_id" --all)
	output=$(crictl inspect -o table "$ctr_id")
	ctr_status_info=$(echo "$output" | grep ^State)

	stop_crio

	start_crio
	output=$(crictl pods --quiet)
	[[ "${output}" == "${pod_id}" ]]

	output=$(crictl pods --quiet --id "$pod_id")
	[[ "${output}" == "${pod_list_info}" ]]

	output=$(crictl inspectp -o json "$pod_id")
	status_output=$(echo "$output" | jq ".status.state")
	ip_output=$(echo "$output" | jq ".status.ip")
	created_at_output=$(echo "$output" | jq ".status.createdAt")
	[[ "${status_output}" == "${pod_status_info}" ]]
	[[ "${ip_output}" == "${pod_ip}" ]]
	[[ "${created_at_output}" == "${pod_created_at}" ]]

	output=$(crictl ps --quiet --all)
	[[ "${output}" == "${ctr_id}" ]]

	output=$(crictl ps --quiet --id "$ctr_id" --all)
	[[ "${output}" == "${ctr_list_info}" ]]

	output=$(crictl inspect -o table "$ctr_id" | grep ^State)
	[[ "${output}" == "${ctr_status_info}" ]]
}

@test "crio restore with bad state and pod stopped" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	crictl stopp "$pod_id"

	stop_crio

	# simulate reboot with runc state going away
	runtime delete -f "$pod_id"

	start_crio

	crictl stopp "$pod_id"
}

@test "crio restore with bad state and ctr stopped" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)

	crictl stop "$ctr_id"

	stop_crio

	# simulate reboot with runc state going away
	runtime delete -f "$pod_id"
	runtime delete -f "$ctr_id"

	start_crio

	crictl stop "$ctr_id"
}

@test "crio restore with bad state and ctr removed" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)

	crictl stop "$ctr_id"
	crictl rm "$ctr_id"

	stop_crio

	# simulate reboot with runc state going away
	runtime delete -f "$pod_id"
	runtime delete -f "$ctr_id"

	start_crio

	run crictl stop "$ctr_id"
	[ "$status" -eq 1 ]
	[[ "${output}" == *"not found"* ]]
}

@test "crio restore with bad state and pod removed" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	stop_crio

	# simulate reboot with runc state going away
	runtime delete -f "$pod_id"

	start_crio

	crictl stopp "$pod_id"
}

@test "crio restore with bad state" {
	# this test makes no sense with no infra container
	CONTAINER_DROP_INFRA_CTR=false start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	output=$(crictl inspectp "$pod_id")
	[[ "${output}" == *"SANDBOX_READY"* ]]

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_CREATED"* ]]

	stop_crio

	# simulate reboot with runc state going away
	runtime delete -f "$pod_id"
	runtime delete -f "$ctr_id"

	start_crio
	output=$(crictl pods --quiet)
	[[ "${output}" == *"${pod_id}"* ]]

	output=$(crictl inspectp "$pod_id")
	[[ "${output}" == *"SANDBOX_NOTREADY"* ]]

	output=$(crictl ps --quiet --all)
	[[ "${output}" == *"${ctr_id}"* ]]

	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_EXITED"* ]]
	# TODO: may be cri-tool should display Exit Code
	#[[ "${output}" == *"Exit Code: 255"* ]]

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "crio restore with missing config.json" {
	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	stop_crio

	# simulate reboot with runtime state and config.json going away
	runtime delete -f "$pod_id"
	runtime delete -f "$ctr_id"
	find "$TESTDIR"/ -name config.json -exec rm \{\} \;
	find "$TESTDIR"/ -name shm -exec umount -l \{\} \;

	start_crio

	! crictl inspect "$ctr_id"

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "crio restore first not managing then managing" {
	CONTAINER_MANAGE_NS_LIFECYCLE=false CONTAINER_DROP_INFRA_CTR=false start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	pod_list_info=$(crictl pods --quiet --id "$pod_id")

	output=$(crictl inspectp -o table "$pod_id")
	pod_status_info=$(echo "$output" | grep ^Status)
	pod_ip=$(echo "$output" | grep ^IP)

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)

	ctr_list_info=$(crictl ps --quiet --id "$ctr_id" --all)

	output=$(crictl inspect -o table "$ctr_id")
	ctr_status_info=$(echo "$output" | grep ^State)

	stop_crio

	CONTAINER_MANAGE_NS_LIFECYCLE=true start_crio
	output=$(crictl pods --quiet)
	[[ "${output}" == "${pod_id}" ]]

	output=$(crictl pods --quiet --id "$pod_id")
	[[ "${output}" == "${pod_list_info}" ]]

	output=$(crictl inspectp -o table "$pod_id")
	status_output=$(echo "$output" | grep ^Status)
	ip_output=$(echo "$output" | grep ^IP)
	[[ "${status_output}" == "${pod_status_info}" ]]
	[[ "${ip_output}" == "${pod_ip}" ]]

	output=$(crictl ps --quiet --all)
	[[ "${output}" == "${ctr_id}" ]]

	output=$(crictl ps --quiet --id "$ctr_id" --all)
	[[ "${output}" == "${ctr_list_info}" ]]

	output=$(crictl inspect -o table "$ctr_id")
	output=$(echo "$output" | grep ^State)
	[[ "${output}" == "${ctr_status_info}" ]]
}

@test "crio restore first managing then not managing" {
	CONTAINER_MANAGE_NS_LIFECYCLE=true CONTAINER_DROP_INFRA_CTR=true start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	pod_list_info=$(crictl pods --quiet --id "$pod_id")

	output=$(crictl inspectp -o table "$pod_id")
	pod_status_info=$(echo "$output" | grep ^Status)
	pod_ip=$(echo "$output" | grep ^IP)

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	ctr_list_info=$(crictl ps --quiet --id "$ctr_id" --all)

	output=$(crictl inspect -o table "$ctr_id")
	ctr_status_info=$(echo "$output" | grep ^State)

	stop_crio

	CONTAINER_MANAGE_NS_LIFECYCLE=false CONTAINER_DROP_INFRA_CTR=false start_crio
	output=$(crictl pods --quiet)
	[[ "${output}" == "${pod_id}" ]]

	output=$(crictl pods --quiet --id "$pod_id")
	[[ "${output}" == "${pod_list_info}" ]]

	output=$(crictl inspectp -o table "$pod_id")
	status_output=$(echo "$output" | grep ^Status)
	ip_output=$(echo "$output" | grep ^IP)
	[[ "${status_output}" == "${pod_status_info}" ]]
	[[ "${ip_output}" == "${pod_ip}" ]]

	output=$(crictl ps --quiet --all)
	[[ "${output}" == "${ctr_id}" ]]

	output=$(crictl ps --quiet --id "$ctr_id" --all)
	[[ "${output}" == "${ctr_list_info}" ]]

	output=$(crictl inspect -o table "$ctr_id")
	output=$(echo "$output" | grep ^State)
	[[ "${output}" == "${ctr_status_info}" ]]
}

@test "crio restore changing managing dir" {
	CONTAINER_MANAGE_NS_LIFECYCLE=true CONTAINER_NAMESPACE_DIR="$TESTDIR/ns1" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	pod_list_info=$(crictl pods --quiet --id "$pod_id")

	output=$(crictl inspectp -o table "$pod_id")
	pod_status_info=$(echo "$output" | grep ^Status)
	pod_ip=$(echo "$output" | grep ^IP)

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	ctr_list_info=$(crictl ps --quiet --id "$ctr_id" --all)

	ctr_status_info=$(crictl inspect -o table "$ctr_id" | grep ^State)

	stop_crio

	CONTAINER_MANAGE_NS_LIFECYCLE=true CONTAINER_NAMESPACE_DIR="$TESTDIR/ns2" start_crio
	output=$(crictl pods --quiet)
	[[ "${output}" == "${pod_id}" ]]

	output=$(crictl pods --quiet --id "$pod_id")
	[[ "${output}" == "${pod_list_info}" ]]

	output=$(crictl inspectp -o table "$pod_id")
	status_output=$(echo "$output" | grep ^Status)
	ip_output=$(echo "$output" | grep ^IP)
	[[ "${status_output}" == "${pod_status_info}" ]]
	[[ "${ip_output}" == "${pod_ip}" ]]

	output=$(crictl ps --quiet --all)
	[[ "${output}" == "${ctr_id}" ]]

	output=$(crictl ps --quiet --id "$ctr_id" --all)
	[[ "${output}" == "${ctr_list_info}" ]]

	output=$(crictl inspect -o table "$ctr_id" | grep ^State)
	[[ "${output}" == "${ctr_status_info}" ]]
}
