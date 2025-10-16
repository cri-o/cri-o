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
	start_crio
	info_json=$(crictl info -o json)

	# Verify the default timeout is set
	default_runtime=$(jq -r '.config.crio.DefaultRuntime' <<< "$info_json")
	timeout=$(jq -r ".config.crio.Runtimes.[\"$default_runtime\"].ContainerCreateTimeout" <<< "$info_json")
	[[ "$timeout" == "240" ]]
}

@test "container create timeout with custom value" {
	# Test that CRI-O starts with valid container_create_timeout configuration
	cat > "$CRIO_CONFIG_DIR"/01-timeout.conf << EOF
[crio.runtime.runtimes.$CONTAINER_DEFAULT_RUNTIME]
container_create_timeout = 300
EOF

	start_crio
	info_json=$(crictl info -o json)

	# Verify the timeout is set in the runtime handler configuration
	timeout=$(jq -r ".config.crio.Runtimes.[\"$CONTAINER_DEFAULT_RUNTIME\"].ContainerCreateTimeout" <<< "$info_json")
	[[ "$timeout" == "300" ]]
}

@test "container create timeout with minimum value enforced" {
	# Test that CRI-O enforces minimum timeout value (30s)
	cat > "$CRIO_CONFIG_DIR"/01-timeout.conf << EOF
[crio.runtime.runtimes.$CONTAINER_DEFAULT_RUNTIME]
container_create_timeout = 15
EOF

	start_crio
	info_json=$(crictl info -o json)

	# Verify the timeout is set to minimum value
	timeout=$(jq -r ".config.crio.Runtimes.[\"$CONTAINER_DEFAULT_RUNTIME\"].ContainerCreateTimeout" <<< "$info_json")
	[[ "$timeout" == "30" ]]
}

@test "container create timeout with different runtime handlers" {
	# Test that different runtime handlers can have different timeouts
	cat > "$CRIO_CONFIG_DIR"/01-timeout.conf << EOF
[crio.runtime]
default_runtime = "$CONTAINER_DEFAULT_RUNTIME"

[crio.runtime.runtimes.$CONTAINER_DEFAULT_RUNTIME]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "$RUNTIME_TYPE"
container_create_timeout = 300

[crio.runtime.runtimes.kata]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "vm"
container_create_timeout = 600
EOF

	unset CONTAINER_DEFAULT_RUNTIME
	unset CONTAINER_RUNTIMES

	start_crio
	info_json=$(crictl info -o json)

	# Verify different timeouts for different runtimes
	default_timeout=$(jq -r '.config.crio.Runtimes.runc.ContainerCreateTimeout' <<< "$info_json")
	kata_timeout=$(jq -r '.config.crio.Runtimes.kata.ContainerCreateTimeout' <<< "$info_json")

	[[ "$default_timeout" == "300" ]]
	[[ "$kata_timeout" == "600" ]]
}
