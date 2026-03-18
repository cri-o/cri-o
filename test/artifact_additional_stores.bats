#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

ARTIFACT_REPO=quay.io/crio/artifact
ARTIFACT_IMAGE="$ARTIFACT_REPO:singlefile"

# Helper: populate an additional store by pulling an artifact into the main
# store, copying the OCI layout, and then removing it from the main store.
# After this, the artifact only exists in the additional store.
function populate_additional_store() {
	local additional_store="$1"

	# Pull artifact into the main store
	crictl pull "$ARTIFACT_IMAGE"

	# Copy the OCI artifact layout to the additional store
	local main_store="$TESTDIR/crio/artifacts"
	cp -a "$main_store" "$additional_store/"

	# Remove from main store so it only exists in the additional store
	crictl rmi "$ARTIFACT_IMAGE"
}

@test "should support valid additional_artifact_stores" {
	ADDITIONAL_STORE="$TESTDIR/additional-store"
	mkdir -p "$ADDITIONAL_STORE/artifacts"

	cat << EOF > "$CRIO_CONFIG_DIR/99-artifact.conf"
[crio.runtime]
additional_artifact_stores = [
    "$ADDITIONAL_STORE"
]
EOF

	start_crio

	run -0 "${CRIO_BINARY_PATH}" status --socket="${CRIO_SOCKET}" config
	[[ "$output" == *'additional_artifact_stores = ['* ]]
	[[ "$output" == *"$ADDITIONAL_STORE"* ]]
}

@test "should fail if additional_artifact_stores path is not absolute" {
	cat << EOF > "$CRIO_CONFIG_DIR/99-artifact-invalid.conf"
[crio.runtime]
additional_artifact_stores = [
    "./relative/path/store"
]
EOF

	run -1 "$CRIO_BINARY_PATH" --config-dir "$CRIO_CONFIG_DIR" config
	[[ "$output" == *'additional_artifact_stores entry must be absolute'* ]]
	[[ "$output" == *'./relative/path/store'* ]]
}

@test "should list artifact from additional store" {
	ADDITIONAL_STORE="$TESTDIR/additional-store"
	mkdir -p "$ADDITIONAL_STORE"

	cat << EOF > "$CRIO_CONFIG_DIR/99-artifact.conf"
[crio.runtime]
additional_artifact_stores = [
    "$ADDITIONAL_STORE"
]
EOF

	start_crio
	populate_additional_store "$ADDITIONAL_STORE"

	# The artifact should be visible from the additional store
	crictl images | grep -qE "$ARTIFACT_REPO.*singlefile"
}

@test "should inspect artifact from additional store" {
	ADDITIONAL_STORE="$TESTDIR/additional-store"
	mkdir -p "$ADDITIONAL_STORE"

	cat << EOF > "$CRIO_CONFIG_DIR/99-artifact.conf"
[crio.runtime]
additional_artifact_stores = [
    "$ADDITIONAL_STORE"
]
EOF

	start_crio
	populate_additional_store "$ADDITIONAL_STORE"

	crictl inspecti "$ARTIFACT_IMAGE" |
		jq -e '
		(.status.pinned == true) and
		(.status.repoDigests | length == 1) and
		(.status.repoTags | length == 1) and
		(.status.size != "0")'
}

@test "should skip pull when artifact exists in additional store" {
	ADDITIONAL_STORE="$TESTDIR/additional-store"
	mkdir -p "$ADDITIONAL_STORE"

	cat << EOF > "$CRIO_CONFIG_DIR/99-artifact.conf"
[crio.runtime]
additional_artifact_stores = [
    "$ADDITIONAL_STORE"
]
EOF

	start_crio
	populate_additional_store "$ADDITIONAL_STORE"

	# Pull again; should be skipped because it exists in the additional store
	crictl pull "$ARTIFACT_IMAGE"

	# Verify the artifact is NOT in the main store (pull was skipped)
	local main_index="$TESTDIR/crio/artifacts/index.json"
	run jq -r '.manifests[].annotations["org.opencontainers.image.ref.name"] // empty' "$main_index"
	[[ "$output" != *"$ARTIFACT_IMAGE"* ]]

	# Verify the log shows the skip message
	grep -q "already exists in additional store" "$CRIO_LOG"
}

@test "should mount artifact from additional store" {
	ADDITIONAL_STORE="$TESTDIR/additional-store"
	mkdir -p "$ADDITIONAL_STORE"

	cat << EOF > "$CRIO_CONFIG_DIR/99-artifact.conf"
[crio.runtime]
additional_artifact_stores = [
    "$ADDITIONAL_STORE"
]
EOF

	start_crio
	populate_additional_store "$ADDITIONAL_STORE"

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE "$ARTIFACT_IMAGE" \
		'.mounts = [ {
      container_path: "/root/artifact",
      image: { image: $ARTIFACT_IMAGE },
    } ] |
    .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")
	crictl start "$ctr_id"

	run crictl exec --sync "$ctr_id" cat /root/artifact/artifact.txt
	[[ "$output" == "hello artifact" ]]
}
