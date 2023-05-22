#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "pause/unpause ctr with right ctr id" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	out=$(echo -e "GET /pause/$ctr_id HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	if [[ ! "$out" == *"200 OK"* ]]; then
		echo "$out"
		exit 1
	fi

	#in order to stop container when finish, it should not be in paused state
	out=$(echo -e "GET /unpause/$ctr_id HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	if [[ ! "$out" == *"200 OK"* ]]; then
		echo "$out"
		exit 1
	fi

	crictl rm -f "$ctr_id"
}

@test "pause ctr with invalid ctr id" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	out=$(echo -e "GET /pause/123 HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	if [[ ! "$out" == *"404 Not Found"* ]]; then
		echo "$out"
		exit 1
	fi

	crictl rm -f "$ctr_id"
}

@test "pause ctr with already paused ctr" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	out=$(echo -e "GET /pause/$ctr_id HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	if [[ ! "$out" == *"200 OK"* ]]; then
		echo "$out"
		exit 1
	fi

	out=$(echo -e "GET /pause/$ctr_id HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	if [[ ! "$out" == *"409 Conflict"* ]]; then
		echo "$out"
		exit 1
	fi

	#in order to stop container when finish, it should not be in paused state
	out=$(echo -e "GET /unpause/$ctr_id HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	if [[ ! "$out" == *"200 OK"* ]]; then
		echo "$out"
		exit 1
	fi

	crictl rm -f "$ctr_id"
}

@test "unpause ctr with right ctr id with running ctr" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	out=$(echo -e "GET /unpause/$ctr_id HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	if [[ ! "$out" == *"409 Conflict"* ]]; then
		echo "$out"
		exit 1
	fi

	crictl rm -f "$ctr_id"
}

@test "unpause ctr with invalid ctr id" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	out=$(echo -e "GET /unpause/123 HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	if [[ ! "$out" == *"404 Not Found"* ]]; then
		echo "$out"
		exit 1
	fi

	crictl rm -f "$ctr_id"
}

@test "remove paused ctr" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	out=$(echo -e "GET /pause/$ctr_id HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	if [[ ! "$out" == *"200 OK"* ]]; then
		echo "$out"
		exit 1
	fi

	crictl rm -f "$ctr_id"
}
