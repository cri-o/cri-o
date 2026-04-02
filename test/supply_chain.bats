#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

SANDBOX_CONFIG="$TESTDATA/sandbox_config.json"
TEST_IMAGE="quay.io/crio/fedora-crio-ci:latest"

# Helper: write a supply chain drop-in config.
function write_supply_chain_config() {
	cat << EOF > "$CRIO_CONFIG_DIR/99-supply-chain.conf"
[crio.image.supply_chain]
$1
EOF
}

# Helper: write a supply chain policy JSON file.
function write_supply_chain_policy() {
	local dir="$1"
	local name="$2"
	local content="$3"
	echo "$content" > "$dir/$name.json"
}

# Helper: create a container config JSON for the test image.
# Usage: create_container_config [name]
# Sets CTR_CONFIG to the output path.
function create_container_config() {
	local name="${1:-}"
	CTR_CONFIG="$TESTDIR/ctr-config.json"
	if [ -n "$name" ]; then
		jq --arg img "$TEST_IMAGE" --arg name "$name" \
			'.image.image = $img | .image.user_specified_image = $img | .metadata.name = $name' \
			"$TESTDATA/container_config.json" > "$CTR_CONFIG"
	else
		jq --arg img "$TEST_IMAGE" \
			'.image.image = $img | .image.user_specified_image = $img' \
			"$TESTDATA/container_config.json" > "$CTR_CONFIG"
	fi
}

@test "supply chain: disabled by default" {
	start_crio

	crictl pull "$TEST_IMAGE"

	POD_ID=$(crictl runp "$SANDBOX_CONFIG")
	create_container_config
	crictl create "$POD_ID" "$CTR_CONFIG" "$SANDBOX_CONFIG"

	run ! grep -q "Supply chain verification" "$CRIO_LOG"
	run ! grep -q "Supply chain audit" "$CRIO_LOG"
}

@test "supply chain: config validation rejects invalid verification mode" {
	write_supply_chain_config 'verification = "bogus"'

	run -1 "$CRIO_BINARY_PATH" --config-dir "$CRIO_CONFIG_DIR" config
	[[ "$output" == *"invalid supply chain verification mode"* ]]
}

@test "supply chain: config validation rejects relative policy_dir" {
	write_supply_chain_config '
verification = "warn"
policy_dir = "relative/path"
'

	run -1 "$CRIO_BINARY_PATH" --config-dir "$CRIO_CONFIG_DIR" config
	[[ "$output" == *"not absolute"* ]]
}

@test "supply chain: config validation rejects invalid fetch_failure_policy" {
	write_supply_chain_config '
verification = "warn"
fetch_failure_policy = "bogus"
'

	run -1 "$CRIO_BINARY_PATH" --config-dir "$CRIO_CONFIG_DIR" config
	[[ "$output" == *"fetch_failure_policy"* ]]
}

@test "supply chain: config validation accepts valid warn config" {
	POLICY_DIR="$TESTDIR/supply-chain-policies"
	mkdir -p "$POLICY_DIR"

	write_supply_chain_config "
verification = \"warn\"
policy_dir = \"$POLICY_DIR\"
"

	run -0 "$CRIO_BINARY_PATH" --config-dir "$CRIO_CONFIG_DIR" config
}

@test "supply chain: warn mode allows container creation and logs audit" {
	POLICY_DIR="$TESTDIR/supply-chain-policies"
	mkdir -p "$POLICY_DIR"
	write_supply_chain_policy "$POLICY_DIR" "default" \
		'{"trust": {"builders": [{"id": "https://github.com/actions/runner", "max_level": 3}]}, "provenance": {"missing_policy": "deny"}}'

	write_supply_chain_config "
verification = \"warn\"
policy_dir = \"$POLICY_DIR\"
"

	start_crio

	POD_ID=$(crictl runp "$SANDBOX_CONFIG")
	create_container_config

	# Container creation should succeed in warn mode even with deny provenance policy.
	crictl create "$POD_ID" "$CTR_CONFIG" "$SANDBOX_CONFIG"

	# Verify audit log was written.
	wait_for_log "Supply chain audit"
	wait_for_log "warn mode, allowing"
}

@test "supply chain: enforce mode rejects container creation with deny provenance policy" {
	POLICY_DIR="$TESTDIR/supply-chain-policies"
	mkdir -p "$POLICY_DIR"
	write_supply_chain_policy "$POLICY_DIR" "default" \
		'{"trust": {"builders": [{"id": "https://github.com/actions/runner", "max_level": 3}]}, "provenance": {"missing_policy": "deny"}}'

	write_supply_chain_config "
verification = \"enforce\"
policy_dir = \"$POLICY_DIR\"
"

	start_crio

	POD_ID=$(crictl runp "$SANDBOX_CONFIG")
	create_container_config

	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$SANDBOX_CONFIG"
	[[ "$output" == *"SignatureValidationFailed"* ]]
}

@test "supply chain: enforce mode allows exempt images" {
	POLICY_DIR="$TESTDIR/supply-chain-policies"
	mkdir -p "$POLICY_DIR"
	write_supply_chain_policy "$POLICY_DIR" "default" \
		"{\"trust\": {\"builders\": [{\"id\": \"https://github.com/actions/runner\", \"max_level\": 3}]}, \"provenance\": {\"missing_policy\": \"deny\"}, \"exclude\": [\"$TEST_IMAGE\"]}"

	write_supply_chain_config "
verification = \"enforce\"
policy_dir = \"$POLICY_DIR\"
"

	start_crio

	POD_ID=$(crictl runp "$SANDBOX_CONFIG")
	create_container_config

	# Exempt image should be allowed even in enforce mode.
	crictl create "$POD_ID" "$CTR_CONFIG" "$SANDBOX_CONFIG"

	wait_for_log "excluded from supply chain verification"
}

@test "supply chain: enforce mode allows with provenance_missing_policy=allow" {
	POLICY_DIR="$TESTDIR/supply-chain-policies"
	mkdir -p "$POLICY_DIR"
	write_supply_chain_policy "$POLICY_DIR" "default" \
		'{"trust": {"builders": [{"id": "https://github.com/actions/runner", "max_level": 3}]}, "provenance": {"missing_policy": "allow"}}'

	write_supply_chain_config "
verification = \"enforce\"
policy_dir = \"$POLICY_DIR\"
"

	start_crio

	POD_ID=$(crictl runp "$SANDBOX_CONFIG")
	create_container_config

	# Should succeed when provenance is missing but policy is allow.
	crictl create "$POD_ID" "$CTR_CONFIG" "$SANDBOX_CONFIG"

	wait_for_log "Supply chain audit"
}

@test "supply chain: enforce mode allows when no trusted builders configured" {
	POLICY_DIR="$TESTDIR/supply-chain-policies"
	mkdir -p "$POLICY_DIR"
	# Policy with deny but no trusted builders.
	write_supply_chain_policy "$POLICY_DIR" "default" '{"provenance": {"missing_policy": "deny"}}'

	write_supply_chain_config "
verification = \"enforce\"
policy_dir = \"$POLICY_DIR\"
"

	start_crio

	POD_ID=$(crictl runp "$SANDBOX_CONFIG")
	create_container_config

	# No trusted builders means provenance check is a no-op, even with deny policy.
	crictl create "$POD_ID" "$CTR_CONFIG" "$SANDBOX_CONFIG"

	wait_for_log "no trusted builders configured"
}

@test "supply chain: namespace-specific policy is used" {
	POLICY_DIR="$TESTDIR/supply-chain-policies"
	mkdir -p "$POLICY_DIR"
	# Default policy: no builders (passes).
	write_supply_chain_policy "$POLICY_DIR" "default" '{}'
	# Namespace "testns": has builders configured + deny, provenance will be missing.
	write_supply_chain_policy "$POLICY_DIR" "testns" \
		'{"trust": {"builders": [{"id": "https://github.com/actions/runner", "max_level": 3}]}, "provenance": {"missing_policy": "deny"}}'

	write_supply_chain_config "
verification = \"enforce\"
policy_dir = \"$POLICY_DIR\"
"

	start_crio

	# Create sandbox in the "testns" namespace.
	NS_SANDBOX_CONFIG="$TESTDIR/sandbox-testns.json"
	jq '.metadata.namespace = "testns"' "$SANDBOX_CONFIG" > "$NS_SANDBOX_CONFIG"
	POD_ID=$(crictl runp "$NS_SANDBOX_CONFIG")

	create_container_config

	# testns namespace has builders configured + deny policy, so should fail.
	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$NS_SANDBOX_CONFIG"
	[[ "$output" == *"SignatureValidationFailed"* ]]
}

@test "supply chain: reload enables verification via SIGHUP" {
	POLICY_DIR="$TESTDIR/supply-chain-policies"
	mkdir -p "$POLICY_DIR"
	write_supply_chain_policy "$POLICY_DIR" "default" \
		'{"trust": {"builders": [{"id": "https://github.com/actions/runner", "max_level": 3}]}, "provenance": {"missing_policy": "deny"}}'

	# Start with verification disabled.
	write_supply_chain_config 'verification = "disabled"'

	start_crio

	# Container creation should work with disabled verification.
	POD_ID=$(crictl runp "$SANDBOX_CONFIG")
	create_container_config
	crictl create "$POD_ID" "$CTR_CONFIG" "$SANDBOX_CONFIG"

	run ! grep -q "Supply chain verification" "$CRIO_LOG"

	# Now switch to enforce mode.
	write_supply_chain_config "
verification = \"enforce\"
policy_dir = \"$POLICY_DIR\"
"

	reload_crio
	wait_for_log "Configuration reload completed"

	# New container creation should fail after reload.
	# Use a different container name to avoid duplicate name optimization.
	POD_ID2=$(crictl runp "$SANDBOX_CONFIG")
	create_container_config "container2"
	run ! crictl create "$POD_ID2" "$CTR_CONFIG" "$SANDBOX_CONFIG"
	[[ "$output" == *"SignatureValidationFailed"* ]]
}

@test "supply chain: reload disables verification via SIGHUP" {
	POLICY_DIR="$TESTDIR/supply-chain-policies"
	mkdir -p "$POLICY_DIR"
	write_supply_chain_policy "$POLICY_DIR" "default" \
		'{"trust": {"builders": [{"id": "https://github.com/actions/runner", "max_level": 3}]}, "provenance": {"missing_policy": "deny"}}'

	# Start with enforce mode.
	write_supply_chain_config "
verification = \"enforce\"
policy_dir = \"$POLICY_DIR\"
"

	start_crio

	POD_ID=$(crictl runp "$SANDBOX_CONFIG")
	create_container_config

	# Should fail with enforce + deny.
	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$SANDBOX_CONFIG"
	[[ "$output" == *"SignatureValidationFailed"* ]]

	# Now disable verification.
	write_supply_chain_config 'verification = "disabled"'

	reload_crio
	wait_for_log "Configuration reload completed"

	# New container should succeed after disabling.
	POD_ID2=$(crictl runp "$SANDBOX_CONFIG")
	crictl create "$POD_ID2" "$CTR_CONFIG" "$SANDBOX_CONFIG"
}

@test "supply chain: malformed policy file prevents startup" {
	POLICY_DIR="$TESTDIR/supply-chain-policies"
	mkdir -p "$POLICY_DIR"
	echo "not json" > "$POLICY_DIR/default.json"

	write_supply_chain_config "
verification = \"warn\"
policy_dir = \"$POLICY_DIR\"
"

	run ! start_crio
	grep -q "parsing policy file" "$CRIO_LOG"
}

@test "supply chain: invalid policy file (bad builder) prevents startup" {
	POLICY_DIR="$TESTDIR/supply-chain-policies"
	mkdir -p "$POLICY_DIR"
	write_supply_chain_policy "$POLICY_DIR" "default" \
		'{"trust": {"builders": [{"id": "", "max_level": 1}]}}'

	write_supply_chain_config "
verification = \"warn\"
policy_dir = \"$POLICY_DIR\"
"

	run ! start_crio
	grep -q "id is required" "$CRIO_LOG"
}
