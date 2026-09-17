#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test

	REGISTRY_ADDRESS="127.0.0.1:$(free_port)"
	REGISTRY_CERTS="$TESTDIR/registry-certs"
	REGISTRY_LOG="$TESTDIR/registry.log"
	# CRI-O and copyimg read registry certificates from the system certs.d directory.
	CERTS_D="/etc/containers/certs.d/$REGISTRY_ADDRESS"
	TEST_IMAGE="$REGISTRY_ADDRESS/test/pause:latest"
}

function teardown() {
	if [[ -n "${REGISTRY_PID:-}" ]]; then
		kill "$REGISTRY_PID" || true
		wait "$REGISTRY_PID" || true
	fi
	rm -rf "$CERTS_D"
	cleanup_test
}

# Start a registry which uses $1 (ML-DSA-44, ML-DSA-65 or ML-DSA-87) for its
# server certificate and requires client certificates of the same algorithm,
# trust it in certs.d and push a test image to it.
function start_registry() {
	"$REGISTRY_BINARY" generate-certs --algorithm "$1" --dir "$REGISTRY_CERTS"

	"$REGISTRY_BINARY" serve \
		--address "$REGISTRY_ADDRESS" \
		--tls-cert "$REGISTRY_CERTS/server.crt" \
		--tls-key "$REGISTRY_CERTS/server.key" \
		--client-ca "$REGISTRY_CERTS/ca.crt" \
		>> "$REGISTRY_LOG" 2>&1 &
	REGISTRY_PID=$!
	retry 20 1 host_and_port_listens "${REGISTRY_ADDRESS%:*}" "${REGISTRY_ADDRESS##*:}"

	mkdir -p "$CERTS_D"
	cp "$REGISTRY_CERTS"/{ca.crt,client.cert,client.key} "$CERTS_D"

	"$COPYIMG_BINARY" \
		--import-from="dir:$(img2dir registry.k8s.io/pause:3.10.2)" \
		--export-to="docker://$TEST_IMAGE" \
		--signature-policy="$INTEGRATION_ROOT"/policy.json \
		--retry-attempts=0
}

# Verify that all TLS handshakes logged by the registry after line $1 used
# TLS 1.3, hybrid ML-KEM key exchange and a verified $2 client certificate.
function assert_pqc_handshakes() {
	local handshakes
	handshakes=$(tail -n +"$(($1 + 1))" "$REGISTRY_LOG" | grep "TLS handshake")
	[ -n "$handshakes" ]

	while read -r line; do
		[[ "$line" == *"version=TLS 1.3 "* ]]
		[[ "$line" == *"key-exchange=X25519MLKEM768 "* ]]
		[[ "$line" == *"client-certificate=$2 "* ]]
		[[ "$line" == *"verified-client-chains=1"* ]]
	done <<< "$handshakes"
}

@test "pull image from registry with ML-DSA-44 certificates and mTLS" {
	start_registry ML-DSA-44
	start_crio
	log_lines=$(wc -l < "$REGISTRY_LOG")

	crictl_pull "$TEST_IMAGE"

	assert_pqc_handshakes "$log_lines" ML-DSA-44
}

@test "pull image from registry with ML-DSA-65 certificates and mTLS" {
	start_registry ML-DSA-65
	start_crio
	log_lines=$(wc -l < "$REGISTRY_LOG")

	crictl_pull "$TEST_IMAGE"

	assert_pqc_handshakes "$log_lines" ML-DSA-65
}

@test "pull image from registry with ML-DSA-87 certificates and mTLS" {
	start_registry ML-DSA-87
	start_crio
	log_lines=$(wc -l < "$REGISTRY_LOG")

	crictl_pull "$TEST_IMAGE"

	assert_pqc_handshakes "$log_lines" ML-DSA-87
}

@test "deny pull from ML-DSA registry with untrusted CA" {
	start_registry ML-DSA-65
	"$REGISTRY_BINARY" generate-certs --algorithm ML-DSA-65 --dir "$TESTDIR/other-certs"
	cp "$TESTDIR/other-certs/ca.crt" "$CERTS_D/ca.crt"
	start_crio

	run ! crictl pull "$TEST_IMAGE"

	[[ "$output" == *"certificate signed by unknown authority"* ]]
}

@test "deny pull from ML-DSA registry without client certificate" {
	start_registry ML-DSA-65
	rm "$CERTS_D"/client.{cert,key}
	start_crio

	run ! crictl pull "$TEST_IMAGE"

	grep -q "client didn't provide a certificate" "$REGISTRY_LOG"
}
