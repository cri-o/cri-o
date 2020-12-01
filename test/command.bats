#!/usr/bin/env bats

load helpers

@test "crio commands" {
	${CRIO_BINARY_PATH} -c /dev/null config > /dev/null
	! ${CRIO_BINARY_PATH} badoption > /dev/null
}

@test "invalid ulimits" {
	run "${CRIO_BINARY_PATH}" --default-ulimits doesntexist=2042
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" == *"invalid ulimit type: doesntexist"* ]]

	run "${CRIO_BINARY_PATH}" --default-ulimits nproc=2042:42
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" == *"ulimit soft limit must be less than or equal to hard limit: 2042 > 42"* ]]
	# can't cover everything here, ulimits parsing is tested in
	# github.com/docker/go-units package
}

@test "invalid devices" {
	run "${CRIO_BINARY_PATH}" --additional-devices /dev/null:/dev/foo:123
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" == *"is not a valid device"* || "$output" == *"invalid device mode"* ]]

	run "${CRIO_BINARY_PATH}" --additional-devices /dev/null:/dee/foo:rm
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" == *"is not a valid device"* || "$output" == *"invalid device mode"* ]]

	run "${CRIO_BINARY_PATH}" --additional-devices /dee/sda:rmw
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" == *"is not a valid device"* || "$output" == *"invalid device mode"* ]]
}

@test "invalid metrics port" {
	run "$CRIO_BINARY_PATH" --metrics-port foo --enable-metrics
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" == *'invalid value "foo" for flag'* ]]

	run "$CRIO_BINARY_PATH" --metrics-port 18446744073709551616 --enable-metrics
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" == *"value out of range"* ]]
}

@test "invalid log max" {
	run "$CRIO_BINARY_PATH" --log-size-max foo
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" == *'invalid value "foo" for flag'* ]]
}

@test "log max boundary testing" {
	# log size max is special zero value
	run "$CRIO_BINARY_PATH" --log-size-max 0
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" == *"log size max should be negative or >= 8192"* ]]

	# log size max is less than 8192 and more than 0
	run "$CRIO_BINARY_PATH" --log-size-max 8191
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" == *"log size max should be negative or >= 8192"* ]]

	# log size max is out of the range of 64-bit signed integers
	run "$CRIO_BINARY_PATH" --log-size-max 18446744073709551616
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" == *"value out of range"* ]]
}
