#!/usr/bin/env bats

load helpers

@test "crio commands" {
	run ${CRIO_BINARY} --config /dev/null config > /dev/null
	echo "$output"
	[ "$status" -eq 0 ]
	run ${CRIO_BINARY} badoption > /dev/null
	echo "$output"
	[ "$status" -ne 0 ]
}

@test "invalid ulimits" {
	run ${CRIO_BINARY} --default-ulimits doesntexist=2042
	echo $output
	[ "$status" -ne 0 ]
	[[ "$output" =~ "invalid ulimit type: doesntexist" ]]
	run ${CRIO_BINARY} --default-ulimits nproc=2042:42
	echo $output
	[ "$status" -ne 0 ]
	[[ "$output" =~ "ulimit soft limit must be less than or equal to hard limit: 2042 > 42" ]]
	# can't cover everything here, ulimits parsing is tested in
	# github.com/docker/go-units package
}

@test "invalid devices" {
	run ${CRIO_BINARY} --additional-devices /dev/sda:/dev/foo:123
	echo $output
	[ "$status" -ne 0 ]
	[[ "$output" =~ "invalid device mode:" ]]
	run ${CRIO_BINARY} --additional-devices /dev/sda:/dee/foo:rm
	echo $output
	[ "$status" -ne 0 ]
	[[ "$output" =~ "invalid device mode:" ]]
	run ${CRIO_BINARY} --additional-devices /dee/sda:rmw
	echo $output
	[ "$status" -ne 0 ]
	[[ "$output" =~ "invalid device mode:" ]]
}
