#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

# PR#59
@test "pod release name on remove" {
	start_crio
	id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	crictl stopp "$id"
	crictl rmp "$id"
	crictl runp "$TESTDATA"/sandbox_config.json
}

@test "pod remove" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "pod stop ignores not found sandboxes" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
	crictl stopp "$pod_id"
}

@test "pod list filtering" {
	start_crio
	pod1_id=$(crictl runp "$TESTDATA"/sandbox1_config.json)
	pod2_id=$(crictl runp "$TESTDATA"/sandbox2_config.json)
	pod3_id=$(crictl runp "$TESTDATA"/sandbox3_config.json)
	output=$(crictl pods --label "name=podsandbox3" --quiet)
	#[[ "$output" != "" ]]
	[[ "$output" == "$pod3_id" ]]
	output=$(crictl pods --label "label=not-exist" --quiet)
	[[ "$output" == "" ]]
	output=$(crictl pods --label "group=test" --label "version=v1.0.0" --quiet)
	[[ "$output" != "" ]]
	[[ "$output" == *"$pod1_id"* ]]
	[[ "$output" == *"$pod2_id"* ]]
	[[ "$output" != *"$pod3_id"* ]]
	output=$(crictl pods --label "group=test" --quiet)
	[[ "$output" != "" ]]
	[[ "$output" == *"$pod1_id"* ]]
	[[ "$output" == *"$pod2_id"* ]]
	[[ "$output" == *"$pod3_id"* ]]
	output=$(crictl pods --id "$pod1_id" --quiet)
	[[ "$output" == "$pod1_id" ]]
	# filter by truncated id should work as well
	output=$(crictl pods --id "${pod1_id:0:4}" --quiet)
	[[ "$output" == "$pod1_id" ]]
	output=$(crictl pods --id "$pod2_id" --quiet)
	[[ "$output" == "$pod2_id" ]]
	output=$(crictl pods --id "$pod3_id" --quiet)
	[[ "$output" == "$pod3_id" ]]
	output=$(crictl pods --id "$pod1_id" --label "group=test" --quiet)
	[[ "$output" == "$pod1_id" ]]
	output=$(crictl pods --id "$pod2_id" --label "group=test" --quiet)
	[[ "$output" == "$pod2_id" ]]
	output=$(crictl pods --id "$pod3_id" --label "group=test" --quiet)
	[[ "$output" == "$pod3_id" ]]
	output=$(crictl pods --id "$pod3_id" --label "group=production" --quiet)
	[[ "$output" == "" ]]
}

@test "pod metadata in list & status" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	output=$(crictl pods --id "$pod_id" --verbose)
	# TODO: expected value should not hard coded here
	[[ "$output" == *"Name: podsandbox1"* ]]
	[[ "$output" == *"UID: redhat-test-crio"* ]]
	[[ "$output" == *"Namespace: redhat.test.crio"* ]]
	[[ "$output" == *"Attempt: 1"* ]]

	output=$(crictl inspectp --output=table "$pod_id")
	# TODO: expected value should not hard coded here
	[[ "$output" == *"Name: podsandbox1"* ]]
	[[ "$output" == *"UID: redhat-test-crio"* ]]
	[[ "$output" == *"Namespace: redhat.test.crio"* ]]
	[[ "$output" == *"Attempt: 1"* ]]
}

@test "pass pod sysctls to runtime" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	CONTAINER_DEFAULT_SYSCTLS="net.ipv4.ip_forward=1" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config_sysctl.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config_sysctl.json)

	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" sysctl kernel.shm_rmid_forced)
	[[ "$output" == *"kernel.shm_rmid_forced = 1"* ]]

	output=$(crictl exec --sync "$ctr_id" sysctl kernel.msgmax)
	[[ "$output" == *"kernel.msgmax = 8192"* ]]

	output=$(crictl exec --sync "$ctr_id" sysctl net.ipv4.ip_local_port_range)
	[[ "$output" == *"net.ipv4.ip_local_port_range = 1024	65000"* ]]

	output=$(crictl exec --sync "$ctr_id" sysctl net.ipv4.ip_forward)
	[[ "$output" == *"net.ipv4.ip_forward = 1"* ]]
}

@test "pod stop idempotent" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	crictl stopp "$pod_id"
	crictl stopp "$pod_id"
}

@test "pod remove idempotent" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "pod stop idempotent with ctrs already stopped" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl stop "$ctr_id"
	crictl stopp "$pod_id"
}

@test "restart crio and still get pod status" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	crictl stopp "$pod_id"
	restart_crio
	output=$(crictl inspectp "$pod_id")
	[ "$output" != "" ]
}

@test "invalid systemd cgroup_parent fail" {
	if [[ "$CONTAINER_CGROUP_MANAGER" != "systemd" ]]; then
		skip "need systemd cgroup manager"
	fi

	wrong_cgroup_parent_config=$(cat "$TESTDATA"/sandbox_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["cgroup_parent"] = "podsandbox1.slice:container:infra"; json.dump(obj, sys.stdout)')
	echo "$wrong_cgroup_parent_config" > "$TESTDIR"/sandbox_wrong_cgroup_parent.json

	# kubelet is technically responsible for creating this cgroup. it is created in cri-o if there's an infra container
	CONTAINER_DROP_INFRA_CTR=false start_crio
	! crictl runp "$TESTDIR"/sandbox_wrong_cgroup_parent.json

	stop_crio
}

@test "systemd cgroup_parent correctly set" {
	if [[ "$CONTAINER_CGROUP_MANAGER" != "systemd" ]]; then
		skip "need systemd cgroup manager"
	fi

	cgroup_parent_config=$(cat "$TESTDATA"/sandbox_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["cgroup_parent"] = "Burstable-pod_integration_tests-123.slice"; json.dump(obj, sys.stdout)')
	echo "$cgroup_parent_config" > "$TESTDIR"/sandbox_systemd_cgroup_parent.json

	# kubelet is technically responsible for creating this cgroup. it is created in cri-o if there's an infra container
	CONTAINER_DROP_INFRA_CTR=false start_crio
	crictl runp "$TESTDIR"/sandbox_systemd_cgroup_parent.json
	output=$(systemctl list-units --type=slice)
	[[ "$output" == *"Burstable-pod_integration_tests-123.slice"* ]]
}

@test "kubernetes pod terminationGracePeriod passthru" {
	[ -v CIRCLECI ] && skip "runc v1.0.0-rc11 required" # TODO remove this
	# Make sure there is no XDG_RUNTIME_DIR set, otherwise the test might end up using the user instance.
	# There is an assumption in the test to use the system instance of systemd (systemctl show).
	CONTAINER_CGROUP_MANAGER="systemd" DBUS_SESSION_BUS_ADDRESS="" XDG_RUNTIME_DIR="" start_crio

	config=$(cat "$TESTDATA"/sandbox_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);del obj["linux"]["cgroup_parent"]; json.dump(obj, sys.stdout)')
	echo "$config" > "$TESTDIR"/sandbox_config-systemd.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_config-systemd.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config_sleep.json "$TESTDIR"/sandbox_config-systemd.json)

	crictl start "$ctr_id"

	output=$(systemctl show "crio-${ctr_id}.scope")
	echo "$output" | grep 'TimeoutStopUSec=' || true      # show
	echo "$output" | grep -q '^TimeoutStopUSec=1min 28s$' # check

	stop_crio
}

@test "pod pause image matches configured image in crio.conf" {
	CONTAINER_DROP_INFRA_CTR=false start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	output=$(crictl inspectp "$pod_id")

	conf_pause_image=$(grep -oP 'pause_image = \K"[^"]+"' "$CRIO_CONFIG")
	pod_pause_image=$(echo "$output" | jq -e .info.image)
	[[ "$conf_pause_image" == "$pod_pause_image" ]]
}
