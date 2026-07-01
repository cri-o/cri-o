#!/usr/bin/env bats

load helpers

STORAGE_RESILIENCE_IMAGE="localhost/crio-storage-resilience:latest"

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

function setup_storage_resilience_config() {
	export CONTAINER_CLEAN_SHUTDOWN_FILE="$TESTDIR/clean-shutdown"
	setup_crio

	cat <<EOF >"$CRIO_CONFIG_DIR/01-storage-resilience.conf"
[crio]
clean_shutdown_file = "$CONTAINER_CLEAN_SHUTDOWN_FILE"
internal_repair = true
EOF
	touch "${CONTAINER_CLEAN_SHUTDOWN_FILE}.supported"
}

function import_two_layer_test_image() {
	if ! command -v podman >/dev/null; then
		skip "podman required to build the two-layer test image"
	fi

	podman build -t "$STORAGE_RESILIENCE_IMAGE" \
		-f "$TESTDATA/Dockerfile.storage-resilience" \
		"$TESTDATA"

	local archive="$TESTDIR/storage-resilience-image.tar"
	podman save -o "$archive" "$STORAGE_RESILIENCE_IMAGE"

	copyimg \
		--image-name="$STORAGE_RESILIENCE_IMAGE" \
		--import-from="docker-archive:$archive"
}

function find_layer_file_path() {
	local marker="$1"
	find "$TESTDIR/crio/overlay" -path "*/diff/$marker" -print -quit
}

function checksum_file() {
	sha256sum "$1" | awk '{print $1}'
}

function storage_overlay_bytes() {
	du -sb "$TESTDIR/crio/overlay" | awk '{print $1}'
}

function stop_crio_unclean() {
	if [ -n "${CRIO_PID+x}" ]; then
		kill -9 "$CRIO_PID" >/dev/null 2>&1 || true
		# wait exits 137 (128 + SIGKILL) after kill -9; that is expected here
		wait "$CRIO_PID" || true
		unset CRIO_PID
	fi
}

@test "two-layer image import succeeds with durable staging sync enabled" {
	setup_storage_resilience_config
	import_two_layer_test_image

	start_crio_no_setup

	layer_one_path=$(find_layer_file_path "layer-one.txt")
	layer_two_path=$(find_layer_file_path "layer-two.txt")

	[[ -n "$layer_one_path" ]]
	[[ -n "$layer_two_path" ]]
	[[ "$(cat "$layer_one_path")" == "layer-one" ]]
	[[ "$(cat "$layer_two_path")" == "layer-two" ]]
}

@test "corrupt upper layer leaves lower layer content intact on disk" {
	setup_storage_resilience_config
	import_two_layer_test_image

	start_crio_no_setup

	layer_one_path=$(find_layer_file_path "layer-one.txt")
	layer_two_path=$(find_layer_file_path "layer-two.txt")
	layer_one_checksum=$(checksum_file "$layer_one_path")

	# Simulate corruption in the upper layer only.
	dd if=/dev/zero of="$layer_two_path" bs=1 count=1 conv=notrunc status=none

	[[ "$(checksum_file "$layer_one_path")" == "$layer_one_checksum" ]]
	[[ "$(cat "$layer_one_path")" == "layer-one" ]]
}

@test "crio check repair preserves healthy lower layer after upper layer corruption" {
	setup_storage_resilience_config
	import_two_layer_test_image

	start_crio_no_setup

	layer_one_path=$(find_layer_file_path "layer-one.txt")
	layer_two_path=$(find_layer_file_path "layer-two.txt")
	layer_one_checksum=$(checksum_file "$layer_one_path")
	storage_before=$(storage_overlay_bytes)

	dd if=/dev/zero of="$layer_two_path" bs=1 count=1 conv=notrunc status=none

	stop_crio_unclean

	"$CRIO_BINARY_PATH" -c "$CRIO_CONFIG" -d "$CRIO_CONFIG_DIR" check --repair

	[[ "$(checksum_file "$layer_one_path")" == "$layer_one_checksum" ]]
	[[ "$(cat "$layer_one_path")" == "layer-one" ]]

	storage_after=$(storage_overlay_bytes)
	((storage_after > 128000))
	((storage_after >= storage_before / 2))
}

@test "unclean restart runs internal repair without wiping healthy lower layer" {
	setup_storage_resilience_config
	import_two_layer_test_image

	start_crio_no_setup

	layer_one_path=$(find_layer_file_path "layer-one.txt")
	layer_two_path=$(find_layer_file_path "layer-two.txt")
	layer_one_checksum=$(checksum_file "$layer_one_path")
	storage_before=$(storage_overlay_bytes)

	dd if=/dev/zero of="$layer_two_path" bs=1 count=1 conv=notrunc status=none

	stop_crio_unclean
	start_crio_no_setup

	[[ "$(checksum_file "$layer_one_path")" == "$layer_one_checksum" ]]
	[[ "$(cat "$layer_one_path")" == "layer-one" ]]

	storage_after=$(storage_overlay_bytes)
	((storage_after > 128000))
	((storage_after >= storage_before / 2))
}
