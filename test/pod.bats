#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

function setup_runtime_with_min_memory() {
	local mem="$1"
	cat << EOF > "$CRIO_CONFIG_DIR/99-mem.conf"
[crio.runtime]
default_runtime = "mem"
[crio.runtime.runtimes.mem]
runtime_path = "$RUNTIME_BINARY_PATH"
container_min_memory = "$mem"
EOF
	unset CONTAINER_DEFAULT_RUNTIME
	unset CONTAINER_RUNTIMES
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

@test "pod remove with timeout from context" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	# the sleep command in the container needs to be killed within the deadline
	# of the context passed down from crictl
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	CRICTL_TIMEOUT=5s crictl rmp -f "$pod_id"
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
	pod_config="$TESTDIR"/sandbox_config.json

	jq '	  .metadata.name = "podsandbox1"
		| .metadata.uid = "redhat-test-crio-1"
		| .labels.group = "test"
		| .labels.name = "podsandbox1"
		| .labels.version = "v1.0.0"' \
		"$TESTDATA"/sandbox_config.json > "$pod_config"
	pod1_id=$(crictl runp "$pod_config")

	jq '	  .metadata.name = "podsandbox2"
		| .metadata.uid = "redhat-test-crio-2"
		| .labels.group = "test"
		| .labels.name = "podsandbox2"
		| .labels.version = "v1.0.0"' \
		"$TESTDATA"/sandbox_config.json > "$pod_config"
	pod2_id=$(crictl runp "$pod_config")

	jq '	  .metadata.name = "podsandbox3"
		| .metadata.uid = "redhat-test-crio-3"
		| .labels.group = "test"
		| .labels.name = "podsandbox3"
		| .labels.version = "v1.1.0"' \
		"$TESTDATA"/sandbox_config.json > "$pod_config"
	pod3_id=$(crictl runp "$pod_config")

	output=$(crictl pods --label "name=podsandbox3" --quiet)
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

	jq '	  .linux.sysctls = {
			"kernel.shm_rmid_forced": "1",
			"net.ipv4.ip_local_port_range": "1024 65000",
			"kernel.msgmax": "8192"
		}' "$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDIR"/sandbox.json)
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

@test "pass pod sysctls to runtime when in userns" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	CONTAINER_DEFAULT_SYSCTLS="net.ipv4.ip_forward=1" start_crio

	# TODO: kernel* ones fail with permission denied.
	jq '	  .linux.sysctls = {
			"net.ipv4.ip_local_port_range": "1024 65000",
		} |
		.linux.security_context.namespace_options.userns_options = {
			"mode": 0,
			"uids": [{
				"host_id": 100000,
				"container_id": 0,
				"length": 65355
			}],
			"gids": [{
				"host_id": 100000,
				"container_id": 0,
				"length": 65355
			}]
		}' "$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDIR"/sandbox.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" sysctl net.ipv4.ip_local_port_range)
	[[ "$output" == *"net.ipv4.ip_local_port_range = 1024	65000"* ]]

	output=$(crictl exec --sync "$ctr_id" sysctl net.ipv4.ip_forward)
	[[ "$output" == *"net.ipv4.ip_forward = 1"* ]]
}

@test "disable crypto.fips_enabled when FIPS_DISABLE is set" {
	# Check if /proc/sys/crypto exists and skip the test if it does not.
	if [ ! -d "/proc/sys/crypto" ]; then
		skip "The directory /proc/sys/crypto does not exist on this host."
	fi
	setup_crio
	create_runtime_with_allowed_annotation logs io.kubernetes.cri-o.DisableFIPS
	start_crio_no_setup

	jq '   .labels["FIPS_DISABLE"] = "true"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sboxconfig.json

	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDIR"/sboxconfig.json)

	output=$(crictl exec --sync "$ctr_id" cat /proc/sys/crypto/fips_enabled)
	[[ "$output" == "0" ]]
}

@test "fail to pass pod sysctls to runtime if invalid spaces" {
	CONTAINER_DEFAULT_SYSCTLS="net.ipv4.ip_forward = 1" crio &
	run ! wait_until_reachable
}

@test "fail to pass pod sysctl to runtime if invalid value" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	start_crio

	jq --arg sysctl "1024 65000'+'net.ipv4.ip_forward=0'" \
		'.linux.sysctls = {
			"net.ipv4.ip_local_port_range": $sysctl,
		}' "$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	run ! crictl runp "$TESTDIR"/sandbox.json

	jq --arg sysctl "net.ipv4.ip_local_port_range=1024 65000'+'net.ipv4.ip_forward" \
		'.linux.sysctls = {
			($sysctl): "0",
		}' "$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	run ! crictl runp "$TESTDIR"/sandbox.json
}

@test "skip pod sysctls to runtime if host" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	CONTAINER_DEFAULT_SYSCTLS="net.ipv4.ip_forward=0" start_crio

	jq '  .linux.security_context.namespace_options = {
			network: 2,
			ipc: 2
		} |
		  .linux.sysctls = {
			"kernel.shm_rmid_forced": "1",
			"net.ipv4.ip_local_port_range": "2048 65000",
			"kernel.msgmax": "16384"
		}' "$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDIR"/sandbox.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" sysctl kernel.shm_rmid_forced)
	[[ "$output" != *"kernel.shm_rmid_forced = 1"* ]]

	output=$(crictl exec --sync "$ctr_id" sysctl kernel.msgmax)
	[[ "$output" != *"kernel.msgmax = 16384"* ]]

	output=$(crictl exec --sync "$ctr_id" sysctl net.ipv4.ip_local_port_range)
	[[ "$output" != *"net.ipv4.ip_local_port_range = 2048	65000"* ]]

	output=$(crictl exec --sync "$ctr_id" sysctl net.ipv4.ip_forward)
	[[ "$output" != *"net.ipv4.ip_forward = 0"* ]]
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
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
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

	# set wrong cgroup_parent
	jq '	  .linux.cgroup_parent = "podsandbox1.slice:container:infra"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	# kubelet is technically responsible for creating this cgroup. it is created in cri-o if there's an infra container
	CONTAINER_DROP_INFRA_CTR=false start_crio
	run ! crictl runp "$TESTDIR"/sandbox.json
}

@test "systemd cgroup_parent correctly set" {
	if [[ "$CONTAINER_CGROUP_MANAGER" != "systemd" ]]; then
		skip "need systemd cgroup manager"
	fi

	jq '	  .linux.cgroup_parent = "Burstable-pod_integration_tests-123.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	# kubelet is technically responsible for creating this cgroup. it is created in cri-o if there's an infra container
	CONTAINER_DROP_INFRA_CTR=false start_crio
	crictl runp "$TESTDIR"/sandbox.json
	output=$(systemctl list-units --type=slice)
	[[ "$output" == *"Burstable-pod_integration_tests-123.slice"* ]]
}

@test "kubernetes pod terminationGracePeriod passthru" {
	# There is an assumption in the test to use the system instance of systemd (systemctl show).
	if [[ "$CONTAINER_CGROUP_MANAGER" != "systemd" ]]; then
		skip "need systemd cgroup manager"
	fi
	# Make sure there is no XDG_RUNTIME_DIR set, otherwise the test might end up using the user instance.
	DBUS_SESSION_BUS_ADDRESS="" XDG_RUNTIME_DIR="" start_crio

	jq '	  .annotations += { "io.kubernetes.pod.terminationGracePeriod": "88" }' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/ctr.json

	ctr_id=$(crictl run "$TESTDIR"/ctr.json "$TESTDATA"/sandbox_config.json)

	output=$(systemctl show "crio-${ctr_id}.scope")
	echo "$output" | grep 'TimeoutStopUSec=' || true      # show
	echo "$output" | grep -q '^TimeoutStopUSec=1min 28s$' # check
}

@test "pod pause image matches configured image in crio.conf" {
	CONTAINER_DROP_INFRA_CTR=false start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	output=$(crictl inspectp "$pod_id")

	conf_pause_image=$(grep -oP 'pause_image = \K"[^"]+"' "$CRIO_CONFIG")
	pod_pause_image=$(echo "$output" | jq -e .info.image)
	[[ "$conf_pause_image" == "$pod_pause_image" ]]
}

@test "pod stop cleans up all namespaces" {
	export CONTAINER_NAMESPACES_DIR="$TESTDIR"/namespaces
	start_crio
	id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	crictl stopp "$id"
	[[ $(find "$CONTAINER_NAMESPACES_DIR" -type f) -eq 0 ]]
}

@test "pod with the correct etc folder ownership" {
	start_crio
	etc_perm_config="$TESTDIR"/container_sleep.json
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq '	  .metadata.name = "etc-permission"
	| .image.image = "quay.io/crio/fedora-crio-ci:latest"
	| .annotations.pod = "etc-permission"
	| .linux.security_context.run_as_user = {
		value: 5000
	} |
	  .linux.security_context.run_as_group = {
		value: 5000
	} |
	  .linux.security_context.fs_group = {
		value: 5000
	} |
	  .linux.security_context.capabilities.add_capabilities[0] = "setuid"
	  | .linux.security_context.capabilities.add_capabilities[1] = "setgid"' \
		"$TESTDATA"/container_sleep.json > "$etc_perm_config"
	ctr_id=$(crictl create "$pod_id" "$etc_perm_config" "$TESTDATA"/sandbox_config.json)
	output=$(crictl exec --sync "$ctr_id" ls -ld /etc)
	[[ "$output" == *"test test"* ]]
}

@test "verify RunAsGroup in container" {
	start_crio

	jq '
    .linux.security_context.run_as_user = { value: 1000 }
    | .linux.security_context.run_as_group = { value: 1001 }
  ' "$TESTDATA"/sandbox_config.json > "$TESTDIR/modified_sandbox_config.json"

	jq '
    .linux.security_context.run_as_user = { value: 1000 }
    | .linux.security_context.run_as_group = { value: 1002 }
  ' "$TESTDATA"/container_sleep.json > "$TESTDIR/modified_container_sleep_config"

	# Create a new pod using the modified sandbox configuration
	pod_id=$(crictl runp "$TESTDIR/modified_sandbox_config.json")

	# Create a new container within the pod using the modified container configuration
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/modified_container_sleep_config" "$TESTDIR/modified_sandbox_config.json")
	crictl start "$ctr_id"

	# Verify that the gid is present in the /etc/group file
	exec_output=$(crictl exec "$ctr_id" cat /etc/group)
	echo "$exec_output" | grep "x:1002" || fail "RunAsGroup ID 1002 not found in /etc/group"

	# Clean up the pod and container
	crictl stop "$ctr_id"
	crictl stopp "$pod_id"
}

@test "run container with memory_limit_in_bytes -1" {
	setup_crio
	setup_runtime_with_min_memory ""
	start_crio_no_setup

	jq --arg image "$IMAGE" '.metadata.name = "memory"
		| .command = ["/bin/sh", "-c", "sleep 600"]
		| .linux.resources.memory_limit_in_bytes = -1' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/memory.json

	run ! crictl run "$TESTDIR"/memory.json "$TESTDATA"/sandbox_config.json
}

@test "run container with memory_limit_in_bytes 12.5MiB" {
	setup_crio
	setup_runtime_with_min_memory "7.5MiB"
	start_crio_no_setup

	jq --arg image "$IMAGE" '.metadata.name = "memory"
		| .command = ["/bin/sh", "-c", "sleep 600"]
		| .linux.resources.memory_limit_in_bytes = 12582912' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/memory.json

	crictl run "$TESTDIR"/memory.json "$TESTDATA"/sandbox_config.json
}

@test "run container with container_min_memory 17.5MiB" {
	setup_crio
	setup_runtime_with_min_memory "17.5MiB"
	start_crio_no_setup

	jq --arg image "$IMAGE" '.metadata.name = "memory"
		| .command = ["/bin/sh", "-c", "sleep 600"]
		| .linux.resources.memory_limit_in_bytes = 12582912' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/memory.json

	run ! crictl run "$TESTDIR"/memory.json "$TESTDATA"/sandbox_config.json
}

@test "run container with container_min_memory 5.5MiB" {
	setup_crio
	setup_runtime_with_min_memory "5.5MiB"
	start_crio_no_setup

	jq --arg image "$IMAGE" '.metadata.name = "memory"
		| .command = ["/bin/sh", "-c", "sleep 600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/memory.json

	crictl run "$TESTDIR"/memory.json "$TESTDATA"/sandbox_config.json
}

@test "run container with empty container_min_memory" {
	setup_crio
	setup_runtime_with_min_memory ""
	start_crio_no_setup

	jq --arg image "$IMAGE" '.metadata.name = "memory"
		| .command = ["/bin/sh", "-c", "sleep 600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/memory.json

	wait_for_log 'Runtime handler \\"mem\\" container minimum memory set to 12582912 bytes'
	crictl run "$TESTDIR"/memory.json "$TESTDATA"/sandbox_config.json
}

@test "run container with default crun memory_limit_in_bytes" {
	if [[ "$CONTAINER_DEFAULT_RUNTIME" != "crun" ]]; then
		skip "must use crun"
	fi
	setup_crio

	# make sure the crun entry is defaulted so we can verify the one crio makes has the correct limit
	sed -i '/\[crio.runtime.runtimes.crun\]/,/monitor_exec_cgroup = \"\"/d' "$CRIO_CUSTOM_CONFIG"
	cat << EOF > "$CRIO_CONFIG_DIR/99-mem.conf"
[crio.runtime]
default_runtime = ""
EOF
	unset CONTAINER_RUNTIMES
	unset CONTAINER_DEFAULT_RUNTIME

	start_crio_no_setup

	jq --arg image "$IMAGE" '.metadata.name = "memory"
		| .command = ["/bin/sh", "-c", "sleep 600"]
		| .linux.resources.memory_limit_in_bytes = 512000' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/memory.json

	wait_for_log 'Runtime handler \\"crun\\" container minimum memory set to 512000 bytes'

	crictl run "$TESTDIR"/memory.json "$TESTDATA"/sandbox_config.json
}
