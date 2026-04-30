#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "Container creation with runtime_pull_image=true" {
	# Skip in non-kata environments where kata binaries aren't installed.
	# In CI, RUNTIME_TYPE=vm is only set in kata matrix jobs.
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "Not running with kata"
	fi

	# Verify the cloud API adaptor (peerpod backend) is running
	output=$(sudo podman inspect --format '{{.State.Status}}' caa 2> /dev/null || true)
	[[ "$output" == "running" ]]

	# setup_crio uses RUNTIME_TYPE to configure the default crun runtime.
	# With RUNTIME_TYPE=vm, crun gets runtime_type="vm" and CRI-O rejects it
	# because /usr/bin/crun doesn't match the containerd-shim-*-v2 naming pattern.
	# kata-remote gets its own runtime_type="vm" from the drop-in config below.
	RUNTIME_TYPE="oci"
	setup_crio

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

	# Increase the kata create_container_timeout — the default (60s) is too
	# short for peer pod VM boot via the Cloud API Adaptor.
	local kata_remote_cfg="/opt/kata/share/defaults/kata-containers/configuration-remote.toml"
	if [[ -f "$kata_remote_cfg" ]]; then
		sudo sed -i 's/^create_container_timeout\s*=.*/create_container_timeout = 300/' "$kata_remote_cfg"
	fi

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
	ctr_id=$(crictl create --with-pull "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
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

	# Cleanup
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# Verify pod is gone
	run ! crictl inspectp "$pod_id"
}
