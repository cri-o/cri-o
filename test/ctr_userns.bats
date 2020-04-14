#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "ctr_userns run container" {
	if test \! -e /proc/self/uid_map; then
		skip "userNS not available"
	fi
	export CONTAINER_UID_MAPPINGS="0:100000:100000"
	export CONTAINER_GID_MAPPINGS="0:200000:100000"
	export CONTAINER_MANAGE_NS_LIFECYCLE=false

	# Workaround for https://github.com/opencontainers/runc/pull/1562
	# Remove once the fix hits the CI
	export OVERRIDE_OPTIONS="--selinux=false"

	# Needed for RHEL
	if test -e /proc/sys/user/max_user_namespaces; then
		echo 15000 > /proc/sys/user/max_user_namespaces
	fi

	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_config_sleep.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	state=$(crictl inspect "$ctr_id")
	pid=$(echo $state | jq .info.pid)
	grep 100000 /proc/$pid/uid_map
	[ "$status" -eq 0 ]
	grep 200000 /proc/$pid/gid_map
	[ "$status" -eq 0 ]

	out=`echo -e "GET /info HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:$CRIO_SOCKET`
	echo "$out"
	[[ "$out" =~ "100000" ]]
	[[ "$out" =~ "200000" ]]
}
