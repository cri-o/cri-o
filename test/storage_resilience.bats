#!/usr/bin/env bats

load helpers

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

	# $BATS_TEST_NUMBER makes the tag unique per test, but that alone is
	# not enough: podman's local image storage (not just its tags) is a
	# single shared, host-wide store, not scoped per TESTDIR. Several of
	# the tests below call this helper under `bats --jobs N`, and since
	# they all build the exact same Dockerfile, concurrent builds produce
	# byte-identical layer content that can race being written into that
	# shared store at the same time, regardless of which tag names it.
	# Serialize just the build/save/rmi section across all parallel jobs
	# with a shared flock so podman's storage never sees two writers at
	# once; each test still runs everything else concurrently.
	local image="localhost/crio-storage-resilience-test-${BATS_TEST_NUMBER}:latest"
	local archive="$TESTDIR/storage-resilience-image.tar"
	local lock="${BATS_RUN_TMPDIR:-/tmp}/crio-storage-resilience-podman-build.lock"

	(
		flock -x 9
		podman build -t "$image" \
			-f "$TESTDATA/Dockerfile.storage-resilience" \
			"$TESTDATA"
		podman save -o "$archive" "$image"
		podman rmi "$image" >/dev/null 2>&1 || true
	) 9>"$lock"

	copyimg \
		--image-name="$image" \
		--import-from="docker-archive:$archive"
}

function find_layer_file_path() {
	local marker="$1"
	find "$TESTDIR/crio/overlay" -path "*/diff/$marker" -print -quit
}

function layer_id_from_diff_path() {
	local diff_path="$1"
	basename "$(dirname "$(dirname "$diff_path")")"
}

function layers_json_path() {
	echo "$TESTDIR/crio/overlay-layers/layers.json"
}

# Returns success (0) if layers.json records the given layer id as complete,
# i.e. without an "incomplete" flag set to true.
function layer_is_marked_complete() {
	local layer_id="$1"
	jq -e --arg id "$layer_id" \
		'any(.[]; .id == $id and ((.flags.incomplete // false) != true))' \
		"$(layers_json_path)" >/dev/null
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

# Configures a dedicated storage.conf that forces pulls through the
# zstd:chunked staging path (ApplyDiffFromStagingDirectory), which is the
# only path that currently calls SyncDirectoryContents() before the atomic
# rename. Without enable_partial_images/convert_images, ordinary pulls use
# ApplyDiff and never exercise the fsync fix at all.
function setup_storage_resilience_convert_config() {
	CONTAINERS_STORAGE_CONF="$TESTDIR/storage.conf"
	export CONTAINERS_STORAGE_CONF

	cat <<EOF >"$CONTAINERS_STORAGE_CONF"
[storage]
driver = "overlay"

[storage.options.pull_options]
enable_partial_images = "true"
convert_images = "true"
EOF

	setup_crio
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

@test "layer stays marked complete in layers.json after content is corrupted under default sync mode" {
	# This directly tests the claim that layers.json's incompleteFlag plus
	# the store lock are sufficient on their own: that "nothing should
	# observe the unsynced layer contents" because an incomplete layer
	# would be caught and deleted on the next load.
	#
	# It does not hold under the default storage sync_mode ("none"):
	# saveLayers() always fsyncs the layers.json write itself (metadata
	# durability is unconditional), but only calls Syncfs() on the actual
	# layer content when sync_mode="filesystem" is explicitly configured.
	# So layers.json can durably record a layer as complete before its
	# content ever reaches disk. A crash in that window leaves a layer
	# that metadata says is perfectly fine, with corrupted content
	# underneath -- exactly the "readlink: invalid argument" signature
	# from the field reports, not a missing/incomplete layer.
	setup_storage_resilience_config
	import_two_layer_test_image

	start_crio_no_setup

	layer_two_path=$(find_layer_file_path "layer-two.txt")
	layer_two_id=$(layer_id_from_diff_path "$layer_two_path")

	# Before corruption: layers.json already durably says this layer is
	# complete (the write that cleared incompleteFlag is always fsynced).
	layer_is_marked_complete "$layer_two_id"

	# Simulate what an unclean shutdown can leave behind under the default
	# sync_mode: content written but never fsynced to physical storage,
	# even though layers.json's own "complete" record was.
	dd if=/dev/zero of="$layer_two_path" bs=1 count=1 conv=notrunc status=none

	stop_crio_unclean
	start_crio_no_setup

	# The incompleteFlag mechanism never sees this: the layer is still
	# recorded as complete after the restart, so the load-time cleanup for
	# incomplete layers does not fire for it.
	layer_is_marked_complete "$layer_two_id"

	# And the corrupted content is served back as if the layer were
	# healthy -- this is what "nothing should observe the unsynced layer
	# contents" would need to prevent, and does not.
	[[ "$(cat "$layer_two_path")" != "layer-two" ]]
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

@test "partial pull through the fsync-protected staging path succeeds against a real image" {
	setup_storage_resilience_convert_config
	start_crio

	# quay.io/crio/nginx, pinned by digest (same image used to reproduce
	# OCPNODE-4463 manually). docker:// sources implement GetBlobAt, so with
	# enable_partial_images/convert_images enabled this pull is routed
	# through PutBlobPartial -> ApplyDiffFromStagingDirectory, the exact
	# function the fsync fix patches. Without those two options (the
	# default), this same pull would use plain ApplyDiff instead, which has
	# no staging directory, no atomic rename, and nothing to fsync.
	#
	# This is an integration check, not proof of fsync-before-rename
	# ordering: it confirms crio really drives a real image's layers
	# (including symlinks, like Debian's /etc/alternatives/*, and small
	# metadata-only layers) through SyncDirectoryContents() without
	# erroring. The syscall-ordering guarantee itself is verified
	# separately and more reliably in the "syscall trace" test below,
	# using a single-threaded reproducer instead of tracing crio live:
	# crio is a heavily multi-threaded Go binary, and strace -f has a
	# well-known attach race on newly created threads where a thread's
	# first few syscalls can execute before the tracer finishes
	# attaching, silently dropping them from a live trace.
	local nginx_image="quay.io/crio/nginx@sha256:960355a671fb88ef18a85f92ccf2ccf8e12186216c86337ad808c204d69d512d"
	crictl pull "$nginx_image"
}

# Builds test/synctrace, a minimal single-threaded reproducer that calls the
# exact same two statements ApplyDiffFromStagingDirectory does --
# ioutils.SyncDirectoryContents() followed by os.Rename() -- against a
# populated staging directory, then traces it with strace. Unlike tracing
# crio live during a real pull, a single-threaded program with no
# concurrent I/O has no new-thread ptrace attach race for strace to lose
# syscalls to, so this reliably proves the ordering guarantee that the
# fsync fix depends on.
@test "fsync on staging directory happens before atomic rename (syscall trace)" {
	if ! command -v strace >/dev/null; then
		skip "strace required to verify fsync-before-rename syscall ordering"
	fi

	local synctrace_bin="$CRIO_ROOT/test/synctrace/synctrace"
	[[ -x "$synctrace_bin" ]]

	local staging_dir="$TESTDIR/staging"
	local trace_log="$TESTDIR/synctrace-strace.log"

	strace -y -tt -e trace=fsync,fdatasync,rename,renameat,renameat2 \
		-o "$trace_log" \
		"$synctrace_bin" "$staging_dir"

	[[ -s "$trace_log" ]]

	local rename_line rename_lineno
	rename_line=$(grep -nE '(rename|renameat|renameat2)\(' "$trace_log" | tail -1)
	[[ -n "$rename_line" ]]
	rename_lineno=${rename_line%%:*}

	# At least one fsync/fdatasync on the staging directory (or a file
	# under it) must appear strictly before the rename line, and none of
	# the dangling symlink created by the reproducer may have caused
	# SyncDirectoryContents to touch it via a dereferencing open() -- if
	# it had, synctrace itself would have failed above.
	local syncs_before_rename
	syncs_before_rename=$(head -n "$rename_lineno" "$trace_log" | grep -F "$staging_dir" | grep -Ec 'f(data)?sync\(')
	((syncs_before_rename > 0))
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
