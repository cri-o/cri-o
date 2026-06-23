#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

function run_crio_check() {
	local log_level=("-l" "${CRIO_BINARY_LOG_LEVEL:-"info"}")

	"$CRIO_BINARY_PATH" -c "$CRIO_CONFIG" -d "$CRIO_CONFIG_DIR" "${log_level[@]}" check "$@"
}

@test "storage directory check should find no issues" {
	setup_crio

	# Should verify no storage directory errors.
	run_crio_check
}

@test "storage directory check should find errors" {
	setup_crio

	# Remove random layer from the storage directory.
	remove_random_storage_layer

	run ! run_crio_check
}

@test "storage directory check should repair errors" {
	setup_crio

	# Remove random layer from the storage directory.
	remove_random_storage_layer

	# Should repair damaged storage directory.
	run_crio_check --repair

	# Should verify no storage directory errors.
	CRIO_BINARY_LOG_LEVEL="debug" run run_crio_check

	[[ "$output" == *"Storage directory ${TESTDIR}/crio has errors: false"* ]]
}

@test "storage directory check should wipe everything on repair errors" {
	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# This will corrupt the storage directory.
	cp -r "$TESTDIR"/crio/overlay{,.old}
	umount -R -l -f "$TESTDIR"/crio/overlay
	rm -Rf "$TESTDIR"/crio/overlay
	cp -r "$TESTDIR"/crio/overlay{.old,}

	# Should wipe badly damaged storage directory.
	#
	# The output is suppressed, as the c/storage library
	# can generate a large volume of log lines while the
	# repair process runs. A smaller image like "busybux"
	# could help alleviate this issue.
	run_crio_check --repair --force --wipe &> /dev/null

	# Storage directory wipe should leave only the metadata behind.
	size=$(du -sb "$TESTDIR"/crio | cut -f 1)

	# The storage directory wipe did not work if there is more data than 128 KiB left.
	if ((size > 1024 * 128)); then
		echo "The crio check storage directory wipe did not work" >&3
		return 1
	fi

	# Should verify no storage directory errors.
	CRIO_BINARY_LOG_LEVEL="debug" run run_crio_check

	[[ "$output" == *"Storage directory ${TESTDIR}/crio has errors: false"* ]]
}
