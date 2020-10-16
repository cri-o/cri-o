#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "bind secrets mounts to container" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	crictl exec --sync "$ctr_id" cat /proc/mounts | grep /container/path1

	output=$(crictl exec --sync "$ctr_id" ls /run/secrets)
	[[ "$output" == *"test.txt"* ]]

	output=$(crictl exec --sync "$ctr_id" ls /run/secrets/mysymlink)
	[[ "$output" == *"key.pem"* ]]
}

@test "default mounts correctly sorted with other mounts" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	host_path="$TESTDIR"/clash
	mkdir "$host_path"
	echo "clashing..." > "$host_path"/clashing.txt
	sed -e "s,%HPATH%,$host_path,g" "$TESTDATA"/container_redis_default_mounts.json > "$TESTDIR"/defmounts_pre.json
	sed -e 's,%CPATH%,\/run\/secrets\/clash,g' "$TESTDIR"/defmounts_pre.json > "$TESTDIR"/defmounts.json
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/defmounts.json "$TESTDATA"/sandbox_config.json)

	crictl exec --sync "$ctr_id" ls -la /run/secrets/clash

	output=$(crictl exec --sync "$ctr_id" cat /run/secrets/clash/clashing.txt)
	[[ "$output" == *"clashing..."* ]]

	crictl exec --sync "$ctr_id" ls -la /run/secrets
	output=$(crictl exec --sync "$ctr_id" cat /run/secrets/test.txt)
	[[ "$output" == *"Testing secrets mounts. I am mounted!"* ]]
}
