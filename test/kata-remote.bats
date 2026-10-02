#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

# kata_remote_conf writes the kata-remote runtime drop-in config and adjusts
# the kata container timeout. Call after setup_crio but before start_crio_no_setup.
function kata_remote_conf() {
	cat > "$CRIO_CONFIG_DIR/50-kata.conf" <<- EOF
		[crio.runtime.runtimes.kata-remote]
		  runtime_path = "/opt/kata/bin/containerd-shim-kata-v2"
		  runtime_root = "/run/vc"
		  runtime_type = "vm"
		  privileged_without_host_devices = true
		  runtime_config_path = "/opt/kata/share/defaults/kata-containers/configuration-remote.toml"
		  runtime_pull_image = true
		  container_create_timeout = 600
	EOF

	local kata_remote_cfg="/opt/kata/share/defaults/kata-containers/configuration-remote.toml"
	if [[ -f "$kata_remote_cfg" ]]; then
		sudo sed -i 's/^create_container_timeout\s*=.*/create_container_timeout = 300/' "$kata_remote_cfg"
	fi
}

# require_caa asserts that the Cloud API Adaptor peer pod backend is running.
# Tests that call this will fail when the CAA container is absent.
function require_caa() {
	local caa_status
	caa_status=$(sudo podman inspect --format '{{.State.Status}}' caa 2> /dev/null || true)
	[[ "$caa_status" == "running" ]]
}

function cleanup() {
	local ctr_id=$1
	local pod_id=$2

	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

}

@test "Container creation with runtime_pull_image=true" {
	# Skip in non-kata environments where kata binaries aren't installed.
	# In CI, RUNTIME_TYPE=vm is only set in kata matrix jobs.
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "Not running with kata"
	fi

	require_caa

	setup_crio
	kata_remote_conf
	start_crio_no_setup
	check_images

	# Verify kata-remote runtime is registered
	wait_for_log "kata-remote"

	# Remove the container image from local storage so we can verify
	# that runtime_pull_image=true pulls it inside the peer pod VM,
	# not into the host's local storage.
	local test_image="quay.io/crio/fedora-crio-ci:latest"
	crictl rmi "$test_image"
	output=$(crictl images)
	[[ "$output" != *"$test_image"* ]]

	# Create pod — use a longer crictl timeout because the remote
	# hypervisor needs to boot a peer pod VM via the CAA.
	local runtimeclass=kata-remote
	pod_id=$(CRICTL_TIMEOUT=5m crictl runp --runtime="$runtimeclass" "$TESTDATA"/sandbox_config.json)
	[[ -n "$pod_id" ]]

	# Verify pod is ready
	output=$(crictl inspectp "$pod_id" | jq -r '.status.state')
	[[ "$output" == "SANDBOX_READY" ]]

	# Create container with image pull
	ctr_id=$(crictl create --with-pull "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	[[ -n "$ctr_id" ]]

	# Verify image is still NOT in local storage — it was pulled inside the VM
	output=$(crictl images)
	[[ "$output" != *"$test_image"* ]]

	# Verify container is created
	output=$(crictl inspect "$ctr_id" | jq -r '.status.state')
	[[ "$output" == "CONTAINER_CREATED" ]]

	# Start container
	crictl start "$ctr_id"

	# Verify container is running
	output=$(crictl inspect "$ctr_id" | jq -r '.status.state')
	[[ "$output" == "CONTAINER_RUNNING" ]]

	# Verify workload is functional
	retry 10 1 crictl exec "$ctr_id" echo ok

	cleanup "$ctr_id" "$pod_id"

	# Verify pod is gone
	run ! crictl inspectp "$pod_id"
}

@test "runtime_pull_image: image cache is restored after CRI-O restart" {
	# Skip in non-kata environments where kata binaries aren't installed.
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "Not running with kata"
	fi

	require_caa

	setup_crio
	kata_remote_conf
	start_crio_no_setup
	wait_for_log "kata-remote"

	local runtimeclass=kata-remote
	local test_image="quay.io/crio/fedora-crio-ci:latest"

	# Create a pod using the kata-remote runtime (which has runtime_pull_image=true).
	pod_id=$(CRICTL_TIMEOUT=5m crictl runp --runtime="$runtimeclass" "$TESTDATA"/sandbox_config.json)
	[[ -n "$pod_id" ]]

	# Create container with image pull
	ctr_id=$(crictl create --with-pull "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	[[ -n "$ctr_id" ]]

	# Start container and stop it
	# This is a workaround for a kata-side problem with deleting a container
	# that was not started.
	# To be fixed on the kata side.
	crictl start "$ctr_id"
	crictl stop "$ctr_id"

	# Remove the container but keep the pod so its run directory (which holds the
	# artifact store) is preserved across the restart.
	crictl rm "$ctr_id"

	# Restart CRI-O. The pod state and artifact store on disk survive the restart,
	# but the known images cache is gone.
	restart_crio

	# After restart, attempt to create a container WITHOUT --with-pull.
	# The image information should be loaded from the artifact store, and the
	# container creation should succeed.
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Verify via the CRI-O log that we actually restored the previously pulled image.
	wait_for_log "runtimePulledImageService: restored 1"
}

@test "runtime_pull_image: pull without sandbox context uses main store" {
	# Requires a kata environment so that the handler binary exists and CRI-O
	# registers the handler. Does not require the Cloud API Adaptor.
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "Not running with kata"
	fi

	setup_crio
	kata_remote_conf
	start_crio_no_setup
	check_images
	wait_for_log "kata-remote"

	local test_image="quay.io/crio/fedora-crio-ci:latest"
	# Ensure a clean slate in the main store.
	crictl rmi "$test_image" || true

	# Pull with no sandbox config — PullImage routes to the default store
	# regardless of any handler's runtime_pull_image setting.
	crictl pull "$test_image"

	# The image must now be visible in the main store.
	[[ -n "$(crictl images --quiet "$test_image")" ]]
}

@test "runtime_pull_image: crictl images excludes runtime-pulled images" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "Not running with kata"
	fi
	require_caa

	setup_crio
	kata_remote_conf
	start_crio_no_setup
	check_images
	wait_for_log "kata-remote"

	local test_image="quay.io/crio/fedora-crio-ci:latest"
	# Remove from main store so it can only be found if routing is wrong.
	crictl rmi "$test_image" || true

	local pod_id
	pod_id=$(CRICTL_TIMEOUT=5m crictl runp --runtime=kata-remote "$TESTDATA"/sandbox_config.json)
	[[ -n "$pod_id" ]]

	# Pull happens inside the VM; the manifest is stored in the per-sandbox artifact store.
	local ctr_id
	ctr_id=$(crictl create --with-pull "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	[[ -n "$ctr_id" ]]

	# ListImages uses the default (local) store only.
	# A runtime-pulled image must not appear there.
	output=$(crictl images)
	[[ "$output" != *"$test_image"* ]]

	crictl start "$ctr_id"
	cleanup "$ctr_id" "$pod_id"
}

@test "runtime_pull_image: crictl image status is empty for runtime-pulled image" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "Not running with kata"
	fi
	require_caa

	setup_crio
	kata_remote_conf
	start_crio_no_setup
	check_images
	wait_for_log "kata-remote"

	local test_image="quay.io/crio/fedora-crio-ci:latest"
	crictl rmi "$test_image" || true

	local pod_id
	pod_id=$(CRICTL_TIMEOUT=5m crictl runp --runtime=kata-remote "$TESTDATA"/sandbox_config.json)
	[[ -n "$pod_id" ]]

	local ctr_id
	ctr_id=$(crictl create --with-pull "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	[[ -n "$ctr_id" ]]

	# Extract the image ID that CRI-O assigned to the runtime-pulled image.
	# IDStringForOutOfProcessConsumptionOnly() returns bare 64-char hex (no sha256: prefix).
	local image_id
	image_id=$(grep "Pulled image:" "$CRIO_LOG" | tail -1 | grep -oE '[0-9a-f]{64}' | tail -1)
	[[ -n "$image_id" ]]

	# ImageStatus uses the default (local) store only.
	# The runtime-pulled image must not appear, so the response has no image ID.
	output=$(crictl image status "$test_image" 2>&1)
	[[ "$output" != *"$image_id"* ]]

	crictl start "$ctr_id"
	cleanup "$ctr_id" "$pod_id"
}

@test "runtime_pull_image: sandbox removal evicts the per-sandbox image service cache" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "Not running with kata"
	fi
	require_caa

	setup_crio
	kata_remote_conf
	start_crio_no_setup
	check_images
	wait_for_log "kata-remote"

	local test_image="quay.io/crio/fedora-crio-ci:latest"
	crictl rmi "$test_image" || true

	# Pull the image into pod A's per-sandbox artifact store.
	local pod_a
	pod_a=$(CRICTL_TIMEOUT=5m crictl runp --runtime=kata-remote "$TESTDATA"/sandbox_config.json)
	[[ -n "$pod_a" ]]

	local ctr_id
	ctr_id=$(crictl create --with-pull "$pod_a" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	[[ -n "$ctr_id" ]]

	crictl start "$ctr_id"
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"

	# Remove pod A — RemoveImageService evicts the per-sandbox cache and
	# removes the on-disk artifact store under the pod's run directory.
	crictl stopp "$pod_a"
	crictl rmp "$pod_a"

	# Pod B is a fresh sandbox with its own empty artifact store.
	local pod_b
	pod_b=$(CRICTL_TIMEOUT=5m crictl runp --runtime=kata-remote "$TESTDATA"/sandbox_config.json)
	[[ -n "$pod_b" ]]

	# Without --with-pull the image is not available for pod B.
	run ! crictl create "$pod_b" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json

	crictl stopp "$pod_b"
	crictl rmp "$pod_b"
}

@test "runtime_pull_image: same image stored in artifact store for kata and containers/storage for default" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "Not running with kata"
	fi
	require_caa

	setup_crio
	kata_remote_conf

	start_crio_no_setup
	wait_for_log "kata-remote"

	local test_image="quay.io/crio/fedora-crio-ci:latest"
	crictl rmi "$test_image" || true

	# Create the kata pod (runtime_pull_image=true) and pull the image.
	# Only a manifest is written into the per-sandbox artifact store;
	# no layers land in the host containers/storage.
	local kata_pod_id
	kata_pod_id=$(CRICTL_TIMEOUT=5m crictl runp --runtime=kata-remote "$TESTDATA"/sandbox_config.json)
	[[ -n "$kata_pod_id" ]]

	local kata_ctr_id
	kata_ctr_id=$(crictl create --with-pull "$kata_pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	[[ -n "$kata_ctr_id" ]]
	crictl start "$kata_ctr_id"

	# The image must be absent from the host-side image list: it was stored
	# in the per-sandbox artifact store, not in containers/storage.
	[[ -z "$(crictl images --quiet "$test_image")" ]]

	# The per-sandbox artifact store must exist and contain an OCI layout
	# index, confirming that only the manifest was persisted on the host.
	local kata_artifact_dir
	kata_artifact_dir=$(sudo find "$TESTDIR/crio-run" -type d -name "artifacts" 2> /dev/null | grep "/$kata_pod_id/" | head -1)
	[[ -n "$kata_artifact_dir" ]]
	[[ -f "$kata_artifact_dir/index.json" ]]

	# Create the default (crun) pod and pull the same image with --with-pull.
	# It must land in containers/storage as a regular image because crun does
	# not have runtime_pull_image=true.
	# Use a distinct UID so CRI-O doesn't mistake this for the still-running
	# kata pod (same name/namespace/attempt would cause a name-index collision
	# and return the kata pod ID instead of creating a fresh sandbox).
	local default_sandbox_config
	default_sandbox_config=$(mktemp "$TESTDIR/sandbox-XXXXXX.json")
	jq '.metadata.uid = "redhat-test-crio-default"' "$TESTDATA"/sandbox_config.json > "$default_sandbox_config"

	local default_pod_id
	default_pod_id=$(crictl runp --runtime=crun "$default_sandbox_config")
	[[ -n "$default_pod_id" ]]

	local default_ctr_id
	default_ctr_id=$(crictl create --with-pull "$default_pod_id" "$TESTDATA"/container_sleep.json "$default_sandbox_config")
	[[ -n "$default_ctr_id" ]]

	# The image must now appear in the host-side image list: it was pulled
	# into containers/storage by the default handler.
	[[ -n "$(crictl images --quiet "$test_image")" ]]

	cleanup "$kata_ctr_id" "$kata_pod_id"
	cleanup "$default_ctr_id" "$default_pod_id"
}

@test "runtime_pull_image: runtimePulledImageService pulls own manifest even when image is already in containers/storage" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "Not running with kata"
	fi
	require_caa

	setup_crio
	kata_remote_conf
	start_crio_no_setup
	wait_for_log "kata-remote"

	local test_image="quay.io/crio/fedora-crio-ci:latest"

	# Pull the image into the host containers/storage directly.
	crictl pull "$test_image"
	[[ -n "$(crictl images --quiet "$test_image")" ]]

	# Record the log position right after this pull so we can detect a
	# separate pull event later.
	wait_for_log "Pulled image:"
	local after_first_pull="$LAST_TIMESTAMP"

	# Create the kata pod and pull the same image with --with-pull.
	# Even though the image is already in containers/storage, the
	# runtimePulledImageService must do its own pull into the per-sandbox
	# artifact store rather than silently reusing the containers/storage copy.
	local kata_pod_id
	kata_pod_id=$(CRICTL_TIMEOUT=5m crictl runp --runtime=kata-remote "$TESTDATA"/sandbox_config.json)
	[[ -n "$kata_pod_id" ]]

	ctr_id=$(crictl create --with-pull "$kata_pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# A second "Pulled image:" entry must appear after the first pull,
	# confirming the runtimePulledImageService performed an independent pull.
	wait_for_log "Pulled image:" "$after_first_pull"

	# The per-sandbox artifact store must also carry an independent OCI manifest.
	local kata_artifact_dir
	kata_artifact_dir=$(sudo find "$TESTDIR/crio-run" -type d -name "artifacts" 2> /dev/null | grep "/$kata_pod_id/" | head -1)
	[[ -n "$kata_artifact_dir" ]]
	[[ -f "$kata_artifact_dir/index.json" ]]

	crictl start "$ctr_id"
	cleanup "$ctr_id" "$kata_pod_id"
}

@test "runtime_pull_image: pulling the same image twice is idempotent" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "Not running with kata"
	fi
	require_caa

	setup_crio
	kata_remote_conf
	start_crio_no_setup
	check_images
	wait_for_log "kata-remote"

	local test_image="quay.io/crio/fedora-crio-ci:latest"
	crictl rmi "$test_image" || true

	local pod_id
	pod_id=$(CRICTL_TIMEOUT=5m crictl runp --runtime=kata-remote "$TESTDATA"/sandbox_config.json)
	[[ -n "$pod_id" ]]

	# First pull.
	local ctr1_id
	ctr1_id=$(crictl create --with-pull "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	[[ -n "$ctr1_id" ]]

	crictl start "$ctr1_id"
	crictl stop "$ctr1_id"
	crictl rm "$ctr1_id"

	# Second pull of the same image for the same sandbox — must succeed and
	# leave the artifact store in a valid state.
	local ctr2_id
	ctr2_id=$(crictl create --with-pull "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	[[ -n "$ctr2_id" ]]

	crictl start "$ctr2_id"
	cleanup "$ctr2_id" "$pod_id"
}
