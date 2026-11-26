#!/usr/bin/env bats

load helpers

function setup() {
	# TODO: drop this skip once userns works with pinned namespaces
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	setup_test
}

function teardown() {
	cleanup_test
}

# Configure a default runtime handler which overrides the global
# drop_infra_ctr setting.
function configure_handler_drop_infra_ctr() {
	local global="$1" handler="$2"

	cat << EOF > "$CRIO_CONFIG_DIR/99-drop-infra.conf"
[crio.runtime]
drop_infra_ctr = $global
default_runtime = "drop-infra"
[crio.runtime.runtimes.drop-infra]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "$RUNTIME_TYPE"
drop_infra_ctr = $handler
EOF
	if [[ -n "$RUNTIME_CONFIG_PATH" ]]; then
		echo "runtime_config_path = \"$RUNTIME_CONFIG_PATH\"" >> "$CRIO_CONFIG_DIR/99-drop-infra.conf"
	fi
}

function start_crio_with_handler_drop_infra_ctr() {
	setup_crio
	configure_handler_drop_infra_ctr "$@"
	unset CONTAINER_DEFAULT_RUNTIME
	unset CONTAINER_RUNTIMES
	unset CONTAINER_DROP_INFRA_CTR
	start_crio_no_setup
}

@test "test infra ctr dropped" {
	CONTAINER_DROP_INFRA_CTR=true start_crio
	jq '.linux.security_context.namespace_options.pid = 1' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)

	output=$(runtime list || true)
	[[ ! "$output" = *"$pod_id"* ]]
}

@test "test infra ctr not dropped" {
	CONTAINER_DROP_INFRA_CTR=true start_crio
	jq '.linux.security_context.namespace_options.pid = 0' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)

	output=$(runtime list)
	[[ "$output" = *"$pod_id"* ]]
}

@test "test infra ctr dropped status" {
	CONTAINER_DROP_INFRA_CTR=true start_crio
	jq '.linux.security_context.namespace_options.pid = 1' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)
	output=$(crictl inspectp "$pod_id" | jq .info)
	[[ "$output" != "{}" ]]
}

@test "runtime handler drop_infra_ctr true overrides the global setting" {
	if [[ "$RUNTIME_TYPE" == vm ]]; then
		skip "requires a shim implementing the Sandbox API"
	fi

	start_crio_with_handler_drop_infra_ctr false true
	jq '.linux.security_context.namespace_options.pid = 1' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)

	grep -q "Dropping infra container for pod $pod_id" "$CRIO_LOG"
	output=$(runtime list || true)
	[[ ! "$output" = *"$pod_id"* ]]
}

@test "runtime handler drop_infra_ctr false overrides the global setting" {
	start_crio_with_handler_drop_infra_ctr true false
	jq '.linux.security_context.namespace_options.pid = 1' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)

	grep -q "Keeping infra container for pod $pod_id" "$CRIO_LOG"
}

@test "vm runtime handler without infra ctr uses the Sandbox API" {
	if [[ "$RUNTIME_TYPE" != vm || -z "$RUNTIME_SUPPORTS_SANDBOX_API" ]]; then
		skip "requires a vm runtime whose shim implements the Sandbox API"
	fi

	start_crio_with_handler_drop_infra_ctr true true
	jq '.linux.security_context.namespace_options.pid = 1' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)

	grep -q "Dropping infra container for pod $pod_id" "$CRIO_LOG"
	find "$TESTDIR" -path "*$pod_id*" -name cri-sandbox-config.pb | grep -q .
	[ "$(ps --no-headers -C containerd-shim-kata-v2 | wc -l)" == "1" ]

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDIR"/sandbox_no_infra.json)
	crictl start "$ctr_id"

	# Removing the only workload leaves the shim to the sandbox.
	crictl rm -f "$ctr_id"
	[ "$(ps --no-headers -C containerd-shim-kata-v2 | wc -l)" == "1" ]

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDIR"/sandbox_no_infra.json)
	crictl start "$ctr_id"

	# The sandbox keeps its mode after a restart with the override reverted.
	stop_crio_no_clean ""
	configure_handler_drop_infra_ctr true false
	start_crio_no_setup
	output=$(crictl inspect -o table "$ctr_id")
	[[ "$output" == *"CONTAINER_RUNNING"* ]]

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
	[ "$(ps --no-headers -C containerd-shim-kata-v2 | wc -l)" == "0" ]
}
