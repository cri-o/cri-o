#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "container create timeout with default value" {
	# Test that CRI-O uses default timeout (240s) when not specified
	setup_crio
	start_crio_no_setup
	info_json=$(crictl info -o json)

	# Verify the default timeout is set
	default_runtime=$(jq -r '.config.crio.DefaultRuntime' <<< "$info_json")
	timeout=$(jq -r ".config.crio.Runtimes.[\"$default_runtime\"].ContainerCreateTimeout" <<< "$info_json")
	[[ "$timeout" == "240" ]]
}

@test "container create timeout with custom value" {
	# Test that CRI-O starts with valid container_create_timeout configuration
	setup_crio
	cat > "$CRIO_CONFIG_DIR"/01-timeout.conf << EOF
[crio.runtime.runtimes.testruntime]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "$RUNTIME_TYPE"
container_create_timeout = 300
EOF

	start_crio_no_setup
	info_json=$(crictl info -o json)

	# Verify the timeout is set in the runtime handler configuration
	timeout=$(jq -r '.config.crio.Runtimes.testruntime.ContainerCreateTimeout' <<< "$info_json")
	[[ "$timeout" == "300" ]]
}

@test "container create timeout with minimum value enforced" {
	# Test that CRI-O enforces minimum timeout value (30s)
	setup_crio
	cat > "$CRIO_CONFIG_DIR"/01-timeout.conf << EOF
[crio.runtime.runtimes.testruntime]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "$RUNTIME_TYPE"
container_create_timeout = 15
EOF

	start_crio_no_setup
	info_json=$(crictl info -o json)

	# Verify the timeout is set to minimum value
	timeout=$(jq -r '.config.crio.Runtimes.testruntime.ContainerCreateTimeout' <<< "$info_json")
	[[ "$timeout" == "30" ]]
}

@test "container create timeout with different runtime handlers" {
	# Test that different runtime handlers can have different timeouts
	setup_crio
	cat > "$CRIO_CONFIG_DIR"/01-timeout.conf << EOF
[crio.runtime]
default_runtime = "testruntime"

[crio.runtime.runtimes.testruntime]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "$RUNTIME_TYPE"
container_create_timeout = 300

[crio.runtime.runtimes.testruntime2]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "$RUNTIME_TYPE"
container_create_timeout = 600
EOF

	unset CONTAINER_DEFAULT_RUNTIME
	unset CONTAINER_RUNTIMES

	start_crio_no_setup
	info_json=$(crictl info -o json)

	# Verify different timeouts for different runtimes
	default_timeout=$(jq -r '.config.crio.Runtimes.testruntime.ContainerCreateTimeout' <<< "$info_json")
	second_timeout=$(jq -r '.config.crio.Runtimes.testruntime2.ContainerCreateTimeout' <<< "$info_json")

	[[ "$default_timeout" == "300" ]]
	[[ "$second_timeout" == "600" ]]
}

@test "container create timeout actual enforcement with container" {
	if [[ "${RUNTIME_TYPE}" == "pod" ]]; then
		skip "not yet supported by conmonrs"
	elif [[ "${RUNTIME_TYPE}" == "vm" ]]; then
		# With Containerd's runtime shim v2, CreateContainer happens over ttrpc, hence, the call can't be
		# intercepted by a simple wrapper like in the "oci" RUNTIME_TYPE case
		skip "test doesn't support shim v2 API yet"
	else
		export RUNTIME_WRAPPER_PATH="${TESTDIR}/runtime-wrapper.sh"
	fi

	setup_crio

	cat > "$RUNTIME_WRAPPER_PATH" << EOF
#!/bin/bash

[[ " \$* " =~ " create " ]] && sleep 40
exec $RUNTIME_BINARY_PATH "\$@"
EOF

	chmod +x "$RUNTIME_WRAPPER_PATH"

	cat > "$CRIO_CONFIG_DIR"/01-timeout.conf << EOF
[crio.runtime]
default_runtime = "testruntime"

[crio.runtime.runtimes.testruntime]
runtime_path = "$RUNTIME_WRAPPER_PATH"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "$RUNTIME_TYPE"
container_create_timeout = 30
EOF

	start_crio_no_setup

	# Create pod first - infra container creation may also timeout
	pod_id=$(crictl runp --runtime testruntime "$TESTDATA"/sandbox_config.json)

	# Attempt to create container - should fail due to timeout
	run ! crictl create -T 60s "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json

	# Verify the container creation timeout message appears in CRI-O logs
	grep -q "Container creation timeout (30s)" "$CRIO_LOG"
}
