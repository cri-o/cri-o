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
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	id="$output"
	run crictl stopp "$id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	id="$output"
	run crictl stopp "$id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "pod remove" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "pod stop ignores not found sandboxes" {
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

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "pod list filtering" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox1_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod1_id="$output"
	run crictl runp "$TESTDATA"/sandbox2_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod2_id="$output"
	run crictl runp "$TESTDATA"/sandbox3_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod3_id="$output"
	run crictl pods --label "name=podsandbox3" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	#[[ "$output" != "" ]]
	[[ "$output" == "$pod3_id" ]]
	run crictl pods --label "label=not-exist" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "" ]]
	run crictl pods --label "group=test" --label "version=v1.0.0" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$pod1_id" ]]
	[[ "$output" =~ "$pod2_id" ]]
	[[ "$output" != "$pod3_id" ]]
	run crictl pods --label "group=test" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$pod1_id" ]]
	[[ "$output" =~ "$pod2_id" ]]
	[[ "$output" =~ "$pod3_id" ]]
	run crictl pods --id "$pod1_id" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$pod1_id" ]]
	# filter by truncated id should work as well
	run crictl pods --id "${pod1_id:0:4}" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$pod1_id" ]]
	run crictl pods --id "$pod2_id" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$pod2_id" ]]
	run crictl pods --id "$pod3_id" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$pod3_id" ]]
	run crictl pods --id "$pod1_id" --label "group=test" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$pod1_id" ]]
	run crictl pods --id "$pod2_id" --label "group=test" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$pod2_id" ]]
	run crictl pods --id "$pod3_id" --label "group=test" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$pod3_id" ]]
	run crictl pods --id "$pod3_id" --label "group=production" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "" ]]
	run crictl stopp "$pod1_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod1_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stopp "$pod2_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod2_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stopp "$pod3_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod3_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "pod metadata in list & status" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl pods --id "$pod_id" --verbose
	echo "$output"
	[ "$status" -eq 0 ]
	# TODO: expected value should not hard coded here
	[[ "$output" =~ "Name: podsandbox1" ]]
	[[ "$output" =~ "UID: redhat-test-crio" ]]
	[[ "$output" =~ "Namespace: redhat.test.crio" ]]
	[[ "$output" =~ "Attempt: 1" ]]

	run crictl inspectp --output=table "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	# TODO: expected value should not hard coded here
	[[ "$output" =~ "Name: podsandbox1" ]]
	[[ "$output" =~ "UID: redhat-test-crio" ]]
	[[ "$output" =~ "Namespace: redhat.test.crio" ]]
	[[ "$output" =~ "Attempt: 1" ]]
}

@test "pass pod sysctls to runtime" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	CONTAINER_DEFAULT_SYSCTLS="net.ipv4.ip_forward=1" start_crio
	run crictl runp "$TESTDATA"/sandbox_config_sysctl.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config_sysctl.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" sysctl kernel.shm_rmid_forced
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "kernel.shm_rmid_forced = 1" ]]

	run crictl exec --sync "$ctr_id" sysctl kernel.msgmax
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "kernel.msgmax = 8192" ]]

	run crictl exec --sync "$ctr_id" sysctl net.ipv4.ip_local_port_range
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "net.ipv4.ip_local_port_range = 1024	65000" ]]

	run crictl exec --sync "$ctr_id" sysctl net.ipv4.ip_forward
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "net.ipv4.ip_forward = 1" ]]
}

@test "pod stop idempotent" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "pod remove idempotent" {
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
}

@test "pod stop idempotent with ctrs already stopped" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stop "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "restart crio and still get pod status" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	restart_crio
	run crictl inspectp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" != "" ]
}

@test "invalid systemd cgroup_parent fail" {
	if [[ "$CONTAINER_CGROUP_MANAGER" != "systemd" ]]; then
		skip "need systemd cgroup manager"
	fi

	wrong_cgroup_parent_config=$(cat "$TESTDATA"/sandbox_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["cgroup_parent"] = "podsandbox1.slice:container:infra"; json.dump(obj, sys.stdout)')
	echo "$wrong_cgroup_parent_config" > "$TESTDIR"/sandbox_wrong_cgroup_parent.json

	start_crio
	run crictl runp "$TESTDIR"/sandbox_wrong_cgroup_parent.json
	echo "$output"
	[ "$status" -eq 1 ]

	stop_crio
}

@test "systemd cgroup_parent correctly set" {
	if [[ "$CONTAINER_CGROUP_MANAGER" != "systemd" ]]; then
		skip "need systemd cgroup manager"
	fi

	cgroup_parent_config=$(cat "$TESTDATA"/sandbox_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["cgroup_parent"] = "Burstable-pod_integration_tests-123.slice"; json.dump(obj, sys.stdout)')
	echo "$cgroup_parent_config" > "$TESTDIR"/sandbox_systemd_cgroup_parent.json

	start_crio
	run crictl runp "$TESTDIR"/sandbox_systemd_cgroup_parent.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run systemctl list-units --type=slice
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "Burstable-pod_integration_tests-123.slice" ]]
}

@test "kubernetes pod terminationGracePeriod passthru" {
	[ -v CIRCLECI ] && skip "runc v1.0.0-rc11 required" # TODO remove this
        # Make sure there is no XDG_RUNTIME_DIR set, otherwise the test might end up using the user instance.
        # There is an assumption in the test to use the system instance of systemd (systemctl show).
	CONTAINER_CGROUP_MANAGER="systemd" DBUS_SESSION_BUS_ADDRESS="" XDG_RUNTIME_DIR="" start_crio

	config=$(cat "$TESTDATA"/sandbox_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);del obj["linux"]["cgroup_parent"]; json.dump(obj, sys.stdout)')
	echo "$config" > "$TESTDIR"/sandbox_config-systemd.json
	run crictl runp "$TESTDIR"/sandbox_config-systemd.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_config_sleep.json "$TESTDIR"/sandbox_config-systemd.json
	echo $output
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl start "$ctr_id"
	echo $output
	[ "$status" -eq 0 ]

	run systemctl show "crio-${ctr_id}.scope"
	[ "$status" -eq 0 ]
	echo "$output" | grep 'TimeoutStopUSec=' || true	# show
	echo "$output" | grep -q '^TimeoutStopUSec=1min 28s$'	# check

	stop_crio
}

@test "pod pause image matches configured image in crio.conf" {
	start_crio

	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"


	run crictl inspectp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	conf_pause_image=$(grep -oP 'pause_image = \K"[^"]+"' $CRIO_CONFIG)
	pod_pause_image=$(echo $output | jq -e .info.image)
	[[ "$conf_pause_image" == "$pod_pause_image" ]]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	stop_crio
}
