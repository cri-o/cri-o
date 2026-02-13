#!/usr/bin/env bats
# vim:set ft=bash :
# Integration tests for TLS minimum version configuration
# Tests the tls_min_version option in [crio.api] section

load helpers

function setup() {
	setup_test
}

function teardown() {
	rm -f "$CRIO_CONFIG_DIR/01-tls-config.conf"
	cleanup_test
}

function generate_test_certs() {
	openssl req -new -newkey rsa:4096 -days 365 -nodes -x509 \
		-subj "/C=US/ST=State/L=City/O=Org/CN=localhost" \
		-keyout "$TESTDIR/key.pem" \
		-out "$TESTDIR/cert.pem"
}

function start_crio_with_tls() {
	local tls_min_version="$1"
	local port="$2"

	# Configure TLS via drop-in config
	cat << EOF > "$CRIO_CONFIG_DIR/01-tls-config.conf"
[crio.api]
tls_min_version = "$tls_min_version"
EOF

	CONTAINER_ENABLE_TLS=true \
		CONTAINER_TLS_CERT="$TESTDIR/cert.pem" \
		CONTAINER_TLS_KEY="$TESTDIR/key.pem" \
		CONTAINER_STREAM_PORT="$port" \
		start_crio

	wait_for_log "TLS enabled for streaming server"
}

@test "tls_min_version: TLS 1.3 server should accept TLS 1.3 and reject TLS 1.2 connections" {
	generate_test_certs
	PORT=$(free_port)

	start_crio_with_tls "VersionTLS13" "$PORT"

	# TLS 1.3 should be accepted
	run openssl s_client -connect "127.0.0.1:$PORT" -tls1_3
	[[ "$output" == *"TLSv1.3"* ]]

	# TLS 1.2 should be rejected
	run openssl s_client -connect "127.0.0.1:$PORT" -tls1_2
	[[ "$output" == *"alert protocol version"* ]] ||
		[[ "$output" == *"wrong version"* ]] ||
		[[ "$output" == *"unsupported protocol"* ]] ||
		[[ "$output" == *"no protocols available"* ]]
}

@test "tls_min_version: TLS 1.2 server should accept both TLS 1.2 and TLS 1.3 connections" {
	generate_test_certs
	PORT=$(free_port)

	start_crio_with_tls "VersionTLS12" "$PORT"

	# TLS 1.3 should be accepted
	run openssl s_client -connect "127.0.0.1:$PORT" -tls1_3
	[[ "$output" == *"TLSv1.3"* ]]

	# TLS 1.2 should be accepted
	run openssl s_client -connect "127.0.0.1:$PORT" -tls1_2
	[[ "$output" == *"TLSv1.2"* ]]
}

@test "tls_min_version: default (empty) should behave as TLS 1.2" {
	generate_test_certs
	PORT=$(free_port)

	CONTAINER_ENABLE_TLS=true \
		CONTAINER_TLS_CERT="$TESTDIR/cert.pem" \
		CONTAINER_TLS_KEY="$TESTDIR/key.pem" \
		CONTAINER_STREAM_PORT="$PORT" \
		start_crio

	wait_for_log "TLS enabled for streaming server"

	# Both TLS 1.2 and TLS 1.3 should work with default config
	run openssl s_client -connect "127.0.0.1:$PORT" -tls1_2
	[[ "$output" == *"TLSv1.2"* ]]

	run openssl s_client -connect "127.0.0.1:$PORT" -tls1_3
	[[ "$output" == *"TLSv1.3"* ]]
}

@test "tls_min_version: invalid version should fail validation" {
	generate_test_certs
	PORT=$(free_port)

	cat << EOF > "$CRIO_CONFIG_DIR/01-tls-config.conf"
[crio.api]
tls_min_version = "InvalidVersion"
EOF

	# CRI-O should fail to start with invalid TLS version
	CONTAINER_ENABLE_TLS=true \
		CONTAINER_TLS_CERT="$TESTDIR/cert.pem" \
		CONTAINER_TLS_KEY="$TESTDIR/key.pem" \
		CONTAINER_STREAM_PORT="$PORT" \
		run ! start_crio_no_setup
}
