#!/usr/bin/env bats

load helpers

@test "crio commands" {
	${CRIO_BINARY_PATH} -c /dev/null config > /dev/null
	run ! "${CRIO_BINARY_PATH}" badoption
}

@test "invalid ulimits" {
	run ! "${CRIO_BINARY_PATH}" --default-ulimits doesntexist=2042
	[[ "$output" == *"invalid ulimit type: doesntexist"* ]]

	run ! "${CRIO_BINARY_PATH}" --default-ulimits nproc=2042:42
	[[ "$output" == *"ulimit soft limit must be less than or equal to hard limit: 2042 > 42"* ]]
	# can't cover everything here, ulimits parsing is tested in
	# github.com/docker/go-units package
}

@test "invalid devices" {
	run ! "${CRIO_BINARY_PATH}" --additional-devices /dev/null:/dev/foo:123
	[[ "$output" == *"is not a valid device"* || "$output" == *"invalid device mode"* ]]

	run ! "${CRIO_BINARY_PATH}" --additional-devices /dev/null:/dee/foo:rm
	[[ "$output" == *"is not a valid device"* || "$output" == *"invalid device mode"* ]]

	run ! "${CRIO_BINARY_PATH}" --additional-devices /dee/sda:rmw
	[[ "$output" == *"is not a valid device"* || "$output" == *"invalid device mode"* ]]
}

@test "invalid metrics port" {
	run ! "$CRIO_BINARY_PATH" --metrics-port foo --enable-metrics
	[[ "$output" == *'invalid value "foo" for flag'* ]]

	run ! "$CRIO_BINARY_PATH" --metrics-port 18446744073709551616 --enable-metrics
	[[ "$output" == *"value out of range"* ]]
}
