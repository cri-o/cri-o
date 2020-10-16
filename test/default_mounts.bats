#!/usr/bin/env bats

load helpers

function setup() {
	setup_test

	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	MOUNT_PATH="$TESTDIR/secrets"
	mkdir "$MOUNT_PATH"
	MOUNT_FILE="$MOUNT_PATH/test.txt"
	touch "$MOUNT_FILE"
	echo "Testing secrets mounts!" > "$MOUNT_FILE"

	# Setup default secrets mounts
	mkdir "$TESTDIR/containers"
	touch "$TESTDIR/containers/mounts.conf"
	echo "$TESTDIR/rhel/secrets:/run/secrets" > "$TESTDIR/containers/mounts.conf"
	echo "$MOUNT_PATH:/container/path1" >> "$TESTDIR/containers/mounts.conf"
	mkdir -p "$TESTDIR/rhel/secrets"
	touch "$TESTDIR/rhel/secrets/test.txt"
	echo "Testing secrets mounts. I am mounted!" > "$TESTDIR/rhel/secrets/test.txt"
	mkdir -p "$TESTDIR/symlink/target"
	touch "$TESTDIR/symlink/target/key.pem"
	ln -s "$TESTDIR/symlink/target" "$TESTDIR/rhel/secrets/mysymlink"

	start_crio
}

function teardown() {
	cleanup_test
}

@test "bind secrets mounts to container" {
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	crictl exec --sync "$ctr_id" cat /proc/mounts | grep /container/path1

	output=$(crictl exec --sync "$ctr_id" ls /run/secrets)
	[[ "$output" == *"test.txt"* ]]

	output=$(crictl exec --sync "$ctr_id" ls /run/secrets/mysymlink)
	[[ "$output" == *"key.pem"* ]]
}

@test "default mounts correctly sorted with other mounts" {
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	host_path="$TESTDIR"/clash
	mkdir "$host_path"
	echo "clashing..." > "$host_path"/clashing.txt

	config="$TESTDIR"/config.json
	jq --arg host_path "$host_path" --arg ctr_path /run/secrets/clash \
		'  .mounts = [ {
			host_path: $host_path,
			container_path: $ctr_path
		} ]' \
		"$TESTDATA"/container_redis.json > "$config"
	ctr_id=$(crictl create "$pod_id" "$config" "$TESTDATA"/sandbox_config.json)

	crictl exec --sync "$ctr_id" ls -la /run/secrets/clash

	output=$(crictl exec --sync "$ctr_id" cat /run/secrets/clash/clashing.txt)
	[[ "$output" == "clashing..."* ]]

	crictl exec --sync "$ctr_id" ls -la /run/secrets
	output=$(crictl exec --sync "$ctr_id" cat /run/secrets/test.txt)
	[[ "$output" == "Testing secrets mounts. I am mounted!"* ]]
}
