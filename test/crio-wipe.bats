#!/usr/bin/env bats

# this test suite tests crio wipe running with combinations of cri-o and
# podman.

load helpers
PODMAN_BINARY=${PODMAN_BINARY:-$(command -v podman || true)}

function setup() {
	setup_test
	export CONTAINER_VERSION_FILE="$TESTDIR"/version.tmp
	export CONTAINER_VERSION_FILE_PERSIST="$TESTDIR"/version-persist.tmp
	export CONTAINER_CLEAN_SHUTDOWN_FILE="$TESTDIR"/clean-shutdown.tmp
	CONTAINER_NAMESPACES_DIR=$(mktemp -d)
	export CONTAINER_NAMESPACES_DIR
}

function run_podman_with_args() {
	if [ -n "$PODMAN_BINARY" ]; then
		"$PODMAN_BINARY" --root "$TESTDIR/crio" --runroot "$TESTDIR/crio-run" "$@"
	fi
}

function teardown() {
	run_podman_with_args stop -a
	run_podman_with_args rm -fa
	cleanup_test
	cleanup_namespaces_dir
}

# run crio_wipe calls crio_wipe and tests it succeeded
function run_crio_wipe() {
	"$CRIO_BINARY_PATH" --config "$CRIO_CONFIG" -d "$CRIO_CONFIG_DIR" wipe
}

# test_crio_wiped_containers checks if a running crio instance
# has no containers or pods
function test_crio_wiped_containers() {
	output=$(crictl pods -v)
	[ "$output" == "" ]
	output=$(crictl ps -v)
	[ "$output" == "" ]
}

function test_crio_did_not_wipe_containers() {
	output=$(crictl pods -v)
	[ "$output" != "" ]
}

function test_crio_wiped_images() {
	# check that the pause image was removed, as we removed a pod
	# that used it
	output=$(crictl images)
	[[ ! "$output" == *"$IMAGE_USED"* ]]
}

function test_crio_did_not_wipe_images() {
	# check that the pause image was not removed
	output=$(crictl images)
	[[ "$output" == *"$IMAGE_USED"* ]]
}

# simulate a reboot by unmounting and removing the namespaces
function cleanup_namespaces_dir() {
	find "$CONTAINER_NAMESPACES_DIR" -type f -exec umount {} \;
	rm -fr "$CONTAINER_NAMESPACES_DIR"
}

function start_crio_with_stopped_pod() {
	start_crio

	# it must be everything before the tag, because crictl output won't match (the columns for image and tag are separated by space)
	IMAGE_USED=$(jq -r .image.image < "$TESTDATA"/container_config.json | cut -f1 -d ':')

	local pod_id
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl stopp "$pod_id"
}

@test "remove containers and images when remove both" {
	CONTAINER_INTERNAL_WIPE=false start_crio_with_stopped_pod
	stop_crio_no_clean

	rm "$CONTAINER_VERSION_FILE"
	rm "$CONTAINER_VERSION_FILE_PERSIST"
	rm -f "/run/crio/crio-wipe-done"
	run_crio_wipe

	CONTAINER_INTERNAL_WIPE=false start_crio_no_setup
	test_crio_wiped_containers
	test_crio_wiped_images
}

@test "remove containers when remove temporary" {
	CONTAINER_INTERNAL_WIPE=false start_crio_with_stopped_pod
	stop_crio_no_clean

	rm "$CONTAINER_VERSION_FILE"
	rm -f "/run/crio/crio-wipe-done"
	run_crio_wipe

	CONTAINER_INTERNAL_WIPE=false start_crio_no_setup
	test_crio_wiped_containers
	test_crio_did_not_wipe_images
}

@test "clear neither when remove persist" {
	CONTAINER_INTERNAL_WIPE=false start_crio_with_stopped_pod
	stop_crio_no_clean

	rm "$CONTAINER_VERSION_FILE_PERSIST"
	rm -f "/run/crio/crio-wipe-done"
	run_crio_wipe

	CONTAINER_INTERNAL_WIPE=false start_crio_no_setup
	test_crio_did_not_wipe_containers
	test_crio_did_not_wipe_images
}

@test "don't clear podman containers" {
	if [ -z "$PODMAN_BINARY" ]; then
		skip "Podman not installed"
	fi

	CONTAINER_INTERNAL_WIPE=false start_crio_with_stopped_pod
	stop_crio_no_clean
	rm -f "/run/crio/crio-wipe-done"

	run_podman_with_args run --name test -d quay.io/crio/fedora-crio-ci:latest top

	run_crio_wipe

	run_podman_with_args container exists test
}

@test "clear everything when shutdown file not found" {
	CONTAINER_INTERNAL_WIPE=false start_crio_with_stopped_pod
	stop_crio_no_clean

	rm "$CONTAINER_CLEAN_SHUTDOWN_FILE"
	rm "$CONTAINER_VERSION_FILE"
	rm -f "/run/crio/crio-wipe-done"

	run_crio_wipe

	CONTAINER_INTERNAL_WIPE=false start_crio_no_setup

	test_crio_wiped_containers
	test_crio_wiped_images
}

@test "clear podman containers when shutdown file not found" {
	if [[ -z "$PODMAN_BINARY" ]]; then
		skip "Podman not installed"
	fi

	CONTAINER_INTERNAL_WIPE=false start_crio_with_stopped_pod
	stop_crio_no_clean

	run_podman_with_args run --name test quay.io/crio/fedora-crio-ci:latest ls
	# all podman containers would be stopped after a reboot
	run_podman_with_args stop -a

	rm "$CONTAINER_CLEAN_SHUTDOWN_FILE"
	rm "$CONTAINER_VERSION_FILE"
	rm -f "/run/crio/crio-wipe-done"

	run_crio_wipe

	run ! run_podman_with_args container exists test
}

@test "fail to clear podman containers when shutdown file not found but container still running" {
	if [[ -z "$PODMAN_BINARY" ]]; then
		skip "Podman not installed"
	fi

	CONTAINER_INTERNAL_WIPE=false start_crio_with_stopped_pod
	stop_crio_no_clean

	# all podman containers would be stopped after a reboot
	run_podman_with_args run --name test -d quay.io/crio/fedora-crio-ci:latest top

	rm "$CONTAINER_CLEAN_SHUTDOWN_FILE"
	rm "$CONTAINER_VERSION_FILE"
	rm -f "/run/crio/crio-wipe-done"

	run ! "$CRIO_BINARY_PATH" --config "$CRIO_CONFIG" -d "$CRIO_CONFIG_DIR" wipe
}

@test "don't clear containers on a forced restart of crio" {
	CONTAINER_INTERNAL_WIPE=false start_crio_with_stopped_pod
	stop_crio_no_clean "-9" || true

	rm -f "/run/crio/crio-wipe-done"
	run_crio_wipe

	CONTAINER_INTERNAL_WIPE=false start_crio_no_setup

	test_crio_did_not_wipe_containers
	test_crio_did_not_wipe_images
}

@test "don't clear containers if clean shutdown supported file not present" {
	CONTAINER_INTERNAL_WIPE=false start_crio_with_stopped_pod
	stop_crio_no_clean

	rm "$CONTAINER_CLEAN_SHUTDOWN_FILE.supported"

	rm -f "/run/crio/crio-wipe-done"
	run_crio_wipe

	CONTAINER_INTERNAL_WIPE=false start_crio_no_setup

	test_crio_did_not_wipe_containers
	test_crio_did_not_wipe_images
}

@test "internal_wipe remove containers and images when remove both" {
	# simulate a reboot by having a removable namespaces dir
	start_crio_with_stopped_pod
	stop_crio_no_clean

	rm "$CONTAINER_VERSION_FILE"
	rm "$CONTAINER_VERSION_FILE_PERSIST"
	# simulate a reboot by having a removable namespaces dir
	cleanup_namespaces_dir

	CONTAINER_INTERNAL_WIPE=true start_crio_no_setup
	test_crio_wiped_containers
	test_crio_wiped_images
}

@test "internal_wipe remove containers when remove temporary and node reboots" {
	start_crio_with_stopped_pod
	stop_crio_no_clean

	rm "$CONTAINER_VERSION_FILE"
	# simulate a reboot by having a removable namespaces dir
	cleanup_namespaces_dir

	CONTAINER_INTERNAL_WIPE=true start_crio_no_setup
	test_crio_wiped_containers
	test_crio_did_not_wipe_images
}

@test "internal_wipe remove containers when remove temporary" {
	start_crio_with_stopped_pod
	stop_crio_no_clean

	rm "$CONTAINER_VERSION_FILE"

	CONTAINER_INTERNAL_WIPE=true start_crio_no_setup
	test_crio_wiped_containers
	test_crio_did_not_wipe_images
}

@test "internal_wipe clear both when remove persist" {
	start_crio_with_stopped_pod
	stop_crio_no_clean

	rm "$CONTAINER_VERSION_FILE_PERSIST"
	# simulate a reboot by having a removable namespaces dir
	cleanup_namespaces_dir

	CONTAINER_INTERNAL_WIPE=true start_crio_no_setup
	test_crio_wiped_containers
	test_crio_wiped_images
}

@test "internal_wipe don't clear podman containers" {
	if [ -z "$PODMAN_BINARY" ]; then
		skip "Podman not installed"
	fi

	start_crio_with_stopped_pod
	stop_crio_no_clean

	run_podman_with_args run --name test -d quay.io/crio/fedora-crio-ci:latest top

	CONTAINER_INTERNAL_WIPE=true start_crio_no_setup

	run_podman_with_args ps -a | grep test
}

@test "internal_wipe don't clear containers on a forced restart of crio" {
	start_crio_with_stopped_pod
	stop_crio_no_clean "-9" || true

	CONTAINER_INTERNAL_WIPE=true start_crio_no_setup

	test_crio_did_not_wipe_containers
	test_crio_did_not_wipe_images
}

@test "internal_wipe eventually cleans network on forced restart of crio if network is slow to come up" {
	CNI_RESULTS_DIR=/var/lib/cni/results

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	stop_crio_no_clean

	runtime kill "$ctr_id" || true
	runtime kill "$pod_id" || true
	# simulate a reboot by having a removable namespaces dir
	cleanup_namespaces_dir

	# pretend like the CNI plugin is waiting for a container to start
	mv "$CRIO_CNI_PLUGIN"/"$CNI_TYPE" "$CRIO_CNI_PLUGIN"/"$CNI_TYPE"-hidden
	rm "$CONTAINER_VERSION_FILE"

	CONTAINER_INTERNAL_WIPE=true start_crio_no_setup

	# allow cri-o to catchup
	sleep 5s

	# pretend like the CNI container has started
	mv "$CRIO_CNI_PLUGIN"/"$CNI_TYPE"-hidden "$CRIO_CNI_PLUGIN"/"$CNI_TYPE"

	# allow cri-o to catch up
	sleep 5s

	# make sure network resources were cleaned up
	run ! ls "$CNI_RESULTS_DIR"/*"$pod_id"*
}

@test "clean up image if corrupted on server restore" {
	setup_crio
	touch "$CONTAINER_CLEAN_SHUTDOWN_FILE.supported"

	# Remove random layer from the storage directory.
	remove_random_storage_layer

	# Since the clean shutdown supported file is created,
	# but the clean shutdown file is absent, we will do the
	# c/storage check/repair.
	CONTAINER_INTERNAL_REPAIR=true start_crio_no_setup

	# Since one of the layers was removed, the image would be corrupted, so we expect
	# one to have been removed.
	num_images=${#IMAGES[@]}

	# We start with $num_images images, and remove one with the layer removal above.
	# `crictl images` adds one additional row for the table header.
	# Thus, this is really $(crictl images | wc -l) - 1 (for the removed image) + 1 (for the header).
	[[ $(crictl images | wc -l) == "$num_images" ]]
}

@test "recover from badly corrupted storage directory" {
	setup_crio
	touch "$CONTAINER_CLEAN_SHUTDOWN_FILE".supported

	start_crio_no_setup

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# This will corrupt the storage directory.
	cp -r "$TESTDIR"/crio/overlay{,.old}
	umount -R -l -f "$TESTDIR"/crio/overlay
	rm -Rf "$TESTDIR"/crio/overlay
	cp -r "$TESTDIR"/crio/overlay{.old,}

	stop_crio_no_clean

	# Remove to trigger internal repair on unclean shutdown.
	rm -Rf "$CONTAINER_CLEAN_SHUTDOWN_FILE"

	# Should recovery from badly corrupted storage directory gracefully.
	CONTAINER_INTERNAL_REPAIR=true start_crio_no_setup

	# Storage directory wipe should leave only the metadata behind.
	size=$(du -sb "$TESTDIR"/crio | cut -f 1)

	# The storage directory wipe did not work if there is more data than 128 KiB left.
	if ((size > 1024 * 128)); then
		echo "The CRI-O internal repair storage directory wipe did not work" >&3
		return 1
	fi
}

@test "crio-wipe should create /run/crio/crio-wipe-done and not wipe again" {
	CONTAINER_INTERNAL_WIPE=false start_crio_with_stopped_pod
	stop_crio_no_clean

	rm "$CONTAINER_CLEAN_SHUTDOWN_FILE"
	rm "$CONTAINER_VERSION_FILE"
	rm -f "/run/crio/crio-wipe-done"

	run_crio_wipe

	ls -l /run/crio/crio-wipe-done

	run cat /run/crio/crio-wipe-done
	[[ "$output" == "done" ]]

	run_crio_wipe
	[[ ! "$output" == *"Wiping storage directory"* ]]

	ls -l /run/crio/crio-wipe-done
}
