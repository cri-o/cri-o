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

@test "crio restore with pod stopped" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	crictl stopp "$pod_id"

	output1=$(crictl pods -o json)

	stop_crio

	start_crio
	output2=$(crictl pods -o json)

	[[ "$output1" == "$output2" ]]
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

	crictl stop "$ctr_id"
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
	[[ "${output}" == *"Exit Code: 137"* ]]

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "crio restore with vm runtime and dead shim" {
	if [[ "$RUNTIME_TYPE" != "vm" ]]; then
		skip "only applicable to vm runtime type"
	fi

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_RUNNING"* ]]

	stop_crio_no_clean ""

	# simulate dead shim by removing address files from bundle directories
	find "$TESTDIR"/ -name address -exec rm \{\} \;

	start_crio_no_setup

	# container should be marked exited after restore
	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_EXITED"* ]]
	[[ "${output}" == *"Exit Code: 255"* ]]

	# stop and remove should succeed without panic
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "crio restore with vm runtime and stale shim address" {
	if [[ "$RUNTIME_TYPE" != "vm" ]]; then
		skip "only applicable to vm runtime type"
	fi

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_RUNNING"* ]]

	stop_crio_no_clean ""

	# simulate dead shim with stale address file: replace address contents
	# with a socket path that will get connection refused
	find "$TESTDIR"/ -name address -exec sh -c 'echo "unix:///nonexistent/dead-shim.sock" > "$1"' _ {} \;

	start_crio_no_setup

	# container should be marked exited after restore
	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_EXITED"* ]]
	[[ "${output}" == *"Exit Code: 255"* ]]

	# stop and remove should succeed without panic
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "crio restore with vm runtime and dead shim already stopped" {
	if [[ "$RUNTIME_TYPE" != "vm" ]]; then
		skip "only applicable to vm runtime type"
	fi

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl stop "$ctr_id"

	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_EXITED"* ]]

	stop_crio_no_clean ""

	# simulate dead shim by removing address files
	find "$TESTDIR"/ -name address -exec rm \{\} \;

	start_crio_no_setup

	# already stopped container should remain exited
	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_EXITED"* ]]

	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "crio stop vm runtime container with signal trapping" {
	if [[ "$RUNTIME_TYPE" != "vm" ]]; then
		skip "only applicable to vm runtime type"
	fi

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Container traps TERM and kills its process group. In production
	# (through kubelet), this triggers a race where the kata shim exits
	# before the Kill RPC response reaches CRI-O, causing ttrpc.ErrClosed.
	# This test does not reliably reproduce the race (crictl talks directly
	# to CRI-O without kubelet's concurrency), but validates the stop/remove
	# lifecycle works for signal-trapping kata containers.
	jq '.command = ["/bin/sh", "-c", "trap '\''kill 0'\'' TERM; sleep 6000"]
		| .args = []' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR/container_trap.json"

	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_trap.json" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_RUNNING"* ]]

	# Without kill() ttrpc.ErrClosed fix: fails with ttrpc: closed
	crictl stop "$ctr_id"

	# Without StopContainer status fix: container stays RUNNING (CRI violation)
	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_EXITED"* ]]

	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "crio vm pod with signal trapping container deletes gracefully" {
	if [[ "$RUNTIME_TYPE" != "vm" ]]; then
		skip "only applicable to vm runtime type"
	fi

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# See note in "crio stop vm runtime container with signal trapping"
	# about the ttrpc.ErrClosed race. This test validates the full pod
	# teardown lifecycle completes for signal-trapping kata containers.
	jq '.command = ["/bin/sh", "-c", "trap '\''kill 0'\'' TERM; sleep 6000"]
		| .args = []' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR/container_trap.json"

	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_trap.json" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Full pod teardown: stop container, remove container, stop pod, remove pod
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# Verify pod is gone
	output=$(crictl pods --quiet)
	[[ -z "${output}" ]]
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

	run ! crictl inspect "$ctr_id"

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "crio restore first not managing then managing" {
	CONTAINER_DROP_INFRA_CTR=false start_crio
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

	start_crio
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
	CONTAINER_DROP_INFRA_CTR=true start_crio
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

	CONTAINER_DROP_INFRA_CTR=false start_crio
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
	CONTAINER_NAMESPACE_DIR="$TESTDIR/ns1" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	pod_list_info=$(crictl pods --quiet --id "$pod_id")

	output=$(crictl inspectp -o table "$pod_id")
	pod_status_info=$(echo "$output" | grep ^Status)
	pod_ip=$(echo "$output" | grep ^IP)

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	ctr_list_info=$(crictl ps --quiet --id "$ctr_id" --all)

	ctr_status_info=$(crictl inspect -o table "$ctr_id" | grep ^State)

	stop_crio

	CONTAINER_NAMESPACE_DIR="$TESTDIR/ns2" start_crio
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

@test "crio restore upon entering KUBENSMNT" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	pod_list_info=$(crictl pods --quiet --id "$pod_id")

	output=$(crictl inspectp -o json "$pod_id")
	[[ -n "$output" ]]
	pod_status_info=$(jq ".status.state" <<< "$output")
	pod_ip=$(jq ".status.ip" <<< "$output")
	pod_created_at=$(jq ".status.createdAt" <<< "$output")

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	ctr_list_info=$(crictl ps --quiet --id "$ctr_id" --all)
	output=$(crictl inspect -o table "$ctr_id")
	ctr_status_info=$(grep ^State <<< "$output")

	stop_crio

	setup_kubensmnt
	start_crio
	output=$(crictl pods --quiet)
	[[ "${output}" == "${pod_id}" ]]

	output=$(crictl pods --quiet --id "$pod_id")
	[[ "${output}" == "${pod_list_info}" ]]

	output=$(crictl inspectp -o json "$pod_id")
	status_output=$(jq ".status.state" <<< "$output")
	ip_output=$(jq ".status.ip" <<< "$output")
	created_at_output=$(jq ".status.createdAt" <<< "$output")
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

@test "crio restore upon exiting KUBENSMNT" {
	setup_kubensmnt
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	pod_list_info=$(crictl pods --quiet --id "$pod_id")

	output=$(crictl inspectp -o json "$pod_id")
	[[ -n "$output" ]]
	pod_status_info=$(jq ".status.state" <<< "$output")
	pod_ip=$(jq ".status.ip" <<< "$output")
	pod_created_at=$(jq ".status.createdAt" <<< "$output")

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	ctr_list_info=$(crictl ps --quiet --id "$ctr_id" --all)
	output=$(crictl inspect -o table "$ctr_id")
	ctr_status_info=$(grep ^State <<< "$output")

	stop_crio

	unset KUBENSMNT
	start_crio
	output=$(crictl pods --quiet)
	[[ "${output}" == "${pod_id}" ]]

	output=$(crictl pods --quiet --id "$pod_id")
	[[ "${output}" == "${pod_list_info}" ]]

	output=$(crictl inspectp -o json "$pod_id")
	status_output=$(jq ".status.state" <<< "$output")
	ip_output=$(jq ".status.ip" <<< "$output")
	created_at_output=$(jq ".status.createdAt" <<< "$output")
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

@test "crio restore imageRef for containers" {
	start_crio
	[[ -n "$REDIS_IMAGEREF" ]]

	ctr_id=$(crictl run "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)

	imageRef=$(crictl inspect -o json "$ctr_id" | jq -r '.status.imageRef')
	[[ "$imageRef" == "$REDIS_IMAGEREF" ]]

	stop_crio
	start_crio

	imageRef=$(crictl inspect -o json "$ctr_id" | jq -r '.status.imageRef')
	[[ "$imageRef" == "$REDIS_IMAGEREF" ]]
}

@test "crio restore volumes for containers" {
	start_crio

	jq --arg path "$TESTDIR" \
		'.mounts = [{
			host_path: $path,
			container_path: "/host"
		}]' \
		"$TESTDATA/container_redis.json" > "$TESTDIR/container.json"
	ctr_id=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")
	crictl inspect "$ctr_id" | jq -e '.status.mounts != []'

	stop_crio
	start_crio
	crictl inspect "$ctr_id" | jq -e '.status.mounts != []'
}
