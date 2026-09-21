#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	newconfig="$TESTDIR/config.json"
}

function teardown() {
	cleanup_test
}

@test "bind_mount_prefix regular file mount works" {
	PREFIX="$TESTDIR/prefix"
	mkdir -p "$PREFIX/data"
	echo "SAFE_CONTENT" > "$PREFIX/data/testfile"

	CONTAINER_BIND_MOUNT_PREFIX="$PREFIX" start_crio

	jq --arg host_path "/data/testfile" \
		'  .command = ["/bin/cat", "/mnt/testfile"]
		| .mounts = [ {
			host_path: $host_path,
			container_path: "/mnt/testfile"
		} ]' \
		"$TESTDATA"/container_config.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)
	wait_until_exit "$ctr_id"
	output=$(crictl logs "$ctr_id")
	[[ "$output" == *"SAFE_CONTENT"* ]]
}

@test "bind_mount_prefix symlink within scope resolves correctly" {
	PREFIX="$TESTDIR/prefix"
	mkdir -p "$PREFIX"
	echo "REAL_DATA" > "$PREFIX/realfile"
	# Relative symlink: target stays within PREFIX, so SecureJoin should resolve it.
	ln -s "realfile" "$PREFIX/linkfile"

	CONTAINER_BIND_MOUNT_PREFIX="$PREFIX" start_crio

	jq --arg host_path "/linkfile" \
		'  .command = ["/bin/cat", "/mnt/linkfile"]
		| .mounts = [ {
			host_path: $host_path,
			container_path: "/mnt/linkfile"
		} ]' \
		"$TESTDATA"/container_config.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)
	wait_until_exit "$ctr_id"
	output=$(crictl logs "$ctr_id")
	[[ "$output" == *"REAL_DATA"* ]]
}

@test "bind_mount_prefix confines symlink that escapes scope" {
	PREFIX="$TESTDIR/prefix"
	mkdir -p "$PREFIX/etc"
	echo "SCOPED_HOSTNAME" > "$PREFIX/etc/hostname"

	# Absolute symlink: target escapes PREFIX, so SecureJoin should confine it.
	ln -s "/etc" "$PREFIX/escape"

	CONTAINER_BIND_MOUNT_PREFIX="$PREFIX" start_crio

	jq --arg host_path "/escape/hostname" \
		'  .command = ["/bin/cat", "/mnt/hostname"]
		| .mounts = [ {
			host_path: $host_path,
			container_path: "/mnt/hostname"
		} ]' \
		"$TESTDATA"/container_config.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)
	wait_until_exit "$ctr_id"
	output=$(crictl logs "$ctr_id")
	[[ "$output" == *"SCOPED_HOSTNAME"* ]]
}

@test "bind_mount_prefix confines symlink with dotdot escape" {
	PREFIX="$TESTDIR/prefix"
	mkdir -p "$PREFIX/subdir" "$PREFIX/etc"
	echo "SCOPED_SHADOW" > "$PREFIX/etc/shadow"

	# Relative symlink using ".." to reach outside PREFIX — SecureJoin should confine it.
	ln -s "../../etc/shadow" "$PREFIX/subdir/escape"

	CONTAINER_BIND_MOUNT_PREFIX="$PREFIX" start_crio

	jq --arg host_path "/subdir/escape" \
		'  .command = ["/bin/cat", "/mnt/shadow"]
		| .mounts = [ {
			host_path: $host_path,
			container_path: "/mnt/shadow"
		} ]' \
		"$TESTDATA"/container_config.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)
	wait_until_exit "$ctr_id"
	output=$(crictl logs "$ctr_id")
	[[ "$output" == *"SCOPED_SHADOW"* ]]
}

@test "bind_mount_prefix confines dotdot traversal in host_path" {
	PREFIX="$TESTDIR/prefix"
	mkdir -p "$PREFIX/etc"
	echo "INSIDE_PREFIX" > "$PREFIX/etc/shadow"

	CONTAINER_BIND_MOUNT_PREFIX="$PREFIX" start_crio

	jq '  .command = ["/bin/cat", "/mnt/shadow"]
		| .mounts = [ {
			host_path: "../../etc/shadow",
			container_path: "/mnt/shadow"
		} ]' \
		"$TESTDATA"/container_config.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)
	wait_until_exit "$ctr_id"
	output=$(crictl logs "$ctr_id")
	[[ "$output" == *"INSIDE_PREFIX"* ]]
}
