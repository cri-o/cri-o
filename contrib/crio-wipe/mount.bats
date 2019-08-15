#!/usr/bin/env bats

load lib
load helpers

function teardown() {
	# cleanup
	rm -f "$loop_file"
	umount "$TMP_STORAGE"

	cleanup_test
}

@test "don't fail and wipe dir if storage dir is a mount" {
	if [[ $EUID -ne 0 ]]; then
		skip "mount test must be run as root"
	fi

	prepare_test "crio version 1.0.0" "\"0.0.0\""

	# setup lookback mount
	loop_device=$(losetup -f)
	loop_file="/tmp/loop.tmp"
	dd if=/dev/zero of="$loop_file" bs=1M count=1
	losetup "$loop_device" "$loop_file"
	mkfs.ext4 "$loop_device"
	mount -t ext4 "$loop_device" "$TMP_STORAGE"

	run main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ -d "$TMP_STORAGE" ]
	[ ! "$(ls -A $TMP_STORAGE)" ]
}
