#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	newconfig="$TESTDIR/config.json"
}

function teardown() {
	cleanup_test
}

function create_device_runtime() {
	cat << EOF > "$CRIO_CONFIG_DIR/01-device.conf"
[crio.runtime]
default_runtime = "device"
[crio.runtime.runtimes.device]
runtime_path = "$RUNTIME_BINARY"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "$RUNTIME_TYPE"
allowed_annotations = ["io.kubernetes.cri-o.Devices"]
EOF
}

@test "additional devices support" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	OVERRIDE_OPTIONS="--additional-devices /dev/null:/dev/qifoo:rwm" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" sh -c "ls /dev/qifoo")
	[ "$output" == "/dev/qifoo" ]
}

@test "additional devices permissions" {
	# We need a ubiquitously configured device that isn't in the
	# OCI spec default set.
	declare -r device="/dev/loop-control"
	declare -r timeout=30

	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	if ! test -r $device; then
		skip "$device not readable"
	fi

	if ! test -w $device; then
		skip "$device not writeable"
	fi

	OVERRIDE_OPTIONS="--additional-devices ${device}:${device}:w" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Ensure the device is there.
	crictl exec --timeout=$timeout --sync "$ctr_id" ls $device

	if ! is_cgroup_v2; then
		# Dump the deviced cgroup configuration for debugging.
		output=$(crictl exec --timeout=$timeout --sync "$ctr_id" cat /sys/fs/cgroup/devices/devices.list)
		[[ "$output" == *"c 10:237 w"* ]]
	fi

	# Opening the device in read mode should fail because the device
	# cgroup access only allows writes.
	run crictl exec --timeout=$timeout --sync "$ctr_id" dd if=$device of=/dev/null count=1
	[[ "$output" == *"Operation not permitted"* ]]

	# The write should be allowed by the devices cgroup policy, so we
	# should see an EINVAL from the device when the device fails it.
	# TODO: fix that test, currently fails with "dd: can't open '/dev/loop-control': No such device non-zero exit code"
	# run crictl exec --timeout=$timeout --sync "$ctr_id" dd if=/dev/zero of=$device count=1
	# echo $output
	# [[ "$output" == *"Invalid argument"* ]]
}

@test "annotation devices support" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	create_device_runtime
	start_crio

	jq '      .annotations."io.kubernetes.cri-o.Devices" = "/dev/null:/dev/qifoo:rwm"' \
		"$TESTDATA"/sandbox_config.json > "$newconfig"

	pod_id=$(crictl runp "$newconfig")

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" sh -c "ls /dev/qifoo")
	[ "$output" == "/dev/qifoo" ]
}

@test "annotation should not be processed if not allowed" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	start_crio

	jq '      .annotations."io.kubernetes.cri-o.Devices" = "/dev/null:/dev/qifoo:rwm"' \
		"$TESTDATA"/sandbox_config.json > "$newconfig"

	pod_id=$(crictl runp "$newconfig")

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	run crictl exec --sync "$ctr_id" sh -c "ls /dev/qifoo"
	[ "$status" -ne 0 ]
}

@test "annotation should override configured additional_devices" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	create_device_runtime

	OVERRIDE_OPTIONS="--additional-devices /dev/urandom:/dev/qifoo:rwm" start_crio

	jq '      .annotations."io.kubernetes.cri-o.Devices" = "/dev/null:/dev/qifoo:rwm"' \
		"$TESTDATA"/sandbox_config.json > "$newconfig"

	pod_id=$(crictl runp "$newconfig")

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# if this was /dev/urandom, it would print output
	output=$(crictl exec --sync "$ctr_id" sh -c "head -n1 /dev/qifoo")
	[[ -z "$output" ]]
}

@test "annotation should configure multiple devices" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	create_device_runtime
	start_crio

	jq '      .annotations."io.kubernetes.cri-o.Devices" = "/dev/null:/dev/qifoo:rwm,/dev/urandom:/dev/peterfoo:rwm"' \
		"$TESTDATA"/sandbox_config.json > "$newconfig"

	pod_id=$(crictl runp "$newconfig")

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" sh -c "head -n1 /dev/qifoo")
	[[ -z "$output" ]]

	output=$(crictl exec --sync "$ctr_id" sh -c "head -n1 /dev/peterfoo")
	[[ ! -z "$output" ]]
}

@test "annotation should fail if one device is invalid" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	create_device_runtime
	start_crio

	jq '      .annotations."io.kubernetes.cri-o.Devices" = "/dev/null:/dev/qifoo:rwm,/dove/null"' \
		"$TESTDATA"/sandbox_config.json > "$newconfig"

	pod_id=$(crictl runp "$newconfig")

	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	[ "$status" -ne 0 ]
}
