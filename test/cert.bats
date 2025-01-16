#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
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
	wait_for_log "reloading certificates"
	run openssl s_client -connect "127.0.0.1:$PORT" -showcerts
	[[ "$output" == *"newName"* ]]
}

@test "streaming server min TLS version is TLS1.2 by default" {
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
	run openssl s_client -connect "127.0.0.1:$PORT" -tls1_3
	[[ "$output" == *"Certificate chain"* ]]

	run openssl s_client -connect "127.0.0.1:$PORT" -tls1_2
	[[ "$output" == *"Certificate chain"* ]]

	run openssl s_client -connect "127.0.0.1:$PORT" -tls1_1
	[[ "$output" == *"no peer certificate available"* ]]
}

@test "streaming server with min TLS version 1.3 rejects request less than TLS 1.3" {
	openssl req -new -newkey rsa:4096 -days 365 -nodes -x509 \
		-subj "/C=US/ST=State/L=City/O=Org/CN=oldName" \
		-keyout "$TESTDIR/key.pem" \
		-out "$TESTDIR/cert.pem"

	PORT=$(free_port)

	CONTAINER_ENABLE_TLS=true \
		CONTAINER_TLS_CERT="$TESTDIR/cert.pem" \
		CONTAINER_TLS_KEY="$TESTDIR/key.pem" \
		CONTAINER_STREAM_PORT="$PORT" \
		CONTAINER_STREAM_MIN_TLS_VERSION="VersionTLS13" \
		start_crio

	wait_for_log "TLS enabled for streaming server"
	run openssl s_client -connect "127.0.0.1:$PORT" -tls1_3
	[[ "$output" != *":error:"* ]]
	[[ "$output" == *"Certificate chain"* ]]

	run openssl s_client -connect "127.0.0.1:$PORT" -tls1_2
	[[ "$output" == *":error:"* ]]

	run openssl s_client -connect "127.0.0.1:$PORT" -tls1_1
	[[ "$output" == *":error:"* ]]
}

@test "streaming server with cipher suites rejects request with other cipher suite" {
	openssl req -new -newkey rsa:4096 -days 365 -nodes -x509 \
		-subj "/C=US/ST=State/L=City/O=Org/CN=oldName" \
		-keyout "$TESTDIR/key.pem" \
		-out "$TESTDIR/cert.pem"

	PORT=$(free_port)

	CONTAINER_ENABLE_TLS=true \
		CONTAINER_TLS_CERT="$TESTDIR/cert.pem" \
		CONTAINER_TLS_KEY="$TESTDIR/key.pem" \
		CONTAINER_STREAM_PORT="$PORT" \
		CONTAINER_STREAM_MIN_TLS_VERSION="VersionTLS12" \
		CONTAINER_STREAM_CIPHER_SUITES="TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384" \
		start_crio

	run openssl s_client -connect "127.0.0.1:$PORT" -cipher "ECDHE-RSA-AES128-GCM-SHA256" -tls1_3
	[[ "$output" != *":error:"* ]]
	[[ "$output" == *"Certificate chain"* ]]

	run openssl s_client -connect "127.0.0.1:$PORT" -cipher "ECDHE-RSA-AES256-GCM-SHA384" -tls1_2
	[[ "$output" != *":error:"* ]]
	[[ "$output" == *"Certificate chain"* ]]

	run openssl s_client -connect "127.0.0.1:$PORT" -cipher "ECDHE-RSA-CHACHA20-POLY1305" -tls1_1
	[[ "$output" == *":error:"* ]]
}
