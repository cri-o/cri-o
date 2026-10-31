#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

function streaming_server_cert_contains() {
	openssl s_client -connect "127.0.0.1:$PORT" -showcerts < /dev/null 2>&1 | grep -q "$1"
}

@test "streaming server tls" {
	openssl req -new -newkey rsa:4096 -days 365 -nodes -x509 \
		-subj "/C=US/ST=State/L=City/O=Org/CN=oldName" \
		-keyout "$TESTDIR/key.pem" \
		-out "$TESTDIR/cert.pem"

	PORT=$(free_port)

	CONTAINER_ENABLE_TLS=true \
		CONTAINER_TLS_CERT="$TESTDIR/cert.pem" \
		CONTAINER_TLS_KEY="$TESTDIR/key.pem" \
		CONTAINER_STREAM_PORT="$PORT" \
		start_crio

	wait_for_log "TLS enabled for streaming server"
	run openssl s_client -connect "127.0.0.1:$PORT" -showcerts
	[[ "$output" == *"oldName"* ]]

	openssl req -new -newkey rsa:4096 -days 365 -nodes -x509 \
		-subj "/C=US/ST=State/L=City/O=Org/CN=newName" \
		-keyout "$TESTDIR/key.pem" \
		-out "$TESTDIR/cert.pem"

	# Writing the key and certificate triggers multiple reloads, where the
	# first ones may still see partially written files.
	retry 10 1 streaming_server_cert_contains "newName"
}
