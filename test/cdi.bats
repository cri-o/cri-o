#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	pod_config="$TESTDATA/sandbox_config.json"
	ctr_config="$TESTDIR/config.json"
	cdidir=$TESTDIR/cdi
	set_cdi_dir "$cdidir"
}

function set_cdi_dir() {
	cat << EOF > "$CRIO_CONFIG_DIR/zz-cdi.conf"
[crio.runtime]
cdi_spec_dirs = [
    "$1",
]
EOF
}

function teardown() {
	cleanup_test
}

function write_cdi_spec() {
	mkdir -p "$cdidir"
	cat << EOF > "$cdidir/vendor0.yaml"
cdiVersion: "0.3.0"
kind: "vendor0.com/device"
devices:
  - name: loop8
    containerEdits:
      env:
        - LOOP8=present
      deviceNodes:
        - path: /dev/loop8
          type: b
          major: 7
          minor: 8
          fileMode: 0640
  - name: loop9
    containerEdits:
      env:
        - LOOP9=present
      deviceNodes:
        - path: /dev/loop9
          type: b
          major: 7
          minor: 9
          fileMode: 0644
containerEdits:
  env:
    - VENDOR0=injected
EOF
}

function verify_injected_vendor0() {
	# shellcheck disable=SC2016
	output=$(crictl exec --sync "$1" sh -c 'echo $VENDOR0')
	[ "$output" = "injected" ]
}

function verify_injected_loop8() {
	# shellcheck disable=SC2016
	output=$(crictl exec --sync "$1" sh -c 'echo $LOOP8')
	[ "$output" = "present" ]
	output=$(crictl exec --sync "$1" sh -c 'stat -c %t.%T /dev/loop8')
	[ "$output" = "7.8" ]
	output=$(crictl exec --sync "$1" sh -c 'stat -c %a /dev/loop8')
	[ "$output" = "640" ]
}

function verify_injected_loop9() {
	# shellcheck disable=SC2016
	output=$(crictl exec --sync "$1" sh -c 'echo $LOOP9')
	[ "$output" = "present" ]
	output=$(crictl exec --sync "$1" sh -c 'stat -c %t.%T /dev/loop9')
	[ "$output" = "7.9" ]
	output=$(crictl exec --sync "$1" sh -c 'stat -c %a /dev/loop9')
	[ "$output" = "644" ]
}

function write_invalid_cdi_spec() {
	mkdir -p "$cdidir"
	cat << EOF > "$cdidir/vendor1.yaml"
cdiVersion: "0.3.0"
kind: "vendor1.com/device"
devices:
  invalid data
EOF
}

function prepare_ctr_without_cdidev {
	cp "$TESTDATA/container_config.json" "$ctr_config"
}

function annotate_ctr_with_cdidev {
	json_src="${1:-$TESTDATA/container_sleep.json}"
	if [ "$json_src" = "$ctr_config" ]; then
		json_src="$ctr_config.in"
		cp "$ctr_config" "$json_src"
	fi
	jq ".annotations |= . + { \"cdi.k8s.io/test\": \"vendor0.com/device=loop8,vendor0.com/device=loop9\" }" \
		"$json_src" > "$ctr_config"
}

function annotate_ctr_with_unknown_cdidev {
	jq ".annotations |= . + { \"cdi.k8s.io/test\": \"vendor0.com/device=loop10\" }" \
		"$TESTDATA/container_sleep.json" > "$ctr_config"
}

function prepare_ctr_with_cdidev {
	jq ".CDI_Devices |= . + [ { \"Name\": \"vendor0.com/device=loop8\" }, { \"Name\": \"vendor0.com/device=loop9\" } ] | .envs |= . + [ { \"key\": \"VENDOR0\", \"value\": \"unset\" }, { \"key\": \"LOOP8\", \"value\": \"unset\" } ]" \
		"$TESTDATA/container_sleep.json" > "$ctr_config"
}

function prepare_ctr_with_unknown_cdidev {
	jq ".CDI_Devices |= . + { \"Name\": \"vendor0.com/device=loop10\" }" \
		"$TESTDATA/container_sleep.json" > "$ctr_config"
}

@test "no CDI errors, create ctr without CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	start_crio

	pod_id=$(crictl runp "$pod_config")

	prepare_ctr_without_cdidev
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")

	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"
}

@test "no CDI errors, create ctr with CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	start_crio

	pod_id=$(crictl runp "$pod_config")

	prepare_ctr_with_cdidev
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	crictl start "$ctr_id"

	verify_injected_vendor0 "$ctr_id"
	verify_injected_loop8 "$ctr_id"
	verify_injected_loop9 "$ctr_id"
}

@test "no CDI errors, create ctr with annotated CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	start_crio

	pod_id=$(crictl runp "$pod_config")

	annotate_ctr_with_cdidev
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	crictl start "$ctr_id"

	verify_injected_vendor0 "$ctr_id"
	verify_injected_loop8 "$ctr_id"
	verify_injected_loop9 "$ctr_id"
}

@test "no CDI errors, create ctr with duplicate annotated CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	start_crio

	pod_id=$(crictl runp "$pod_config")

	prepare_ctr_with_cdidev
	annotate_ctr_with_cdidev "$ctr_config"

	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	crictl start "$ctr_id"

	verify_injected_vendor0 "$ctr_id"
	verify_injected_loop8 "$ctr_id"
	verify_injected_loop9 "$ctr_id"

	wait_for_log "Skipping duplicate annotated CDI device"
}

@test "no CDI errors, fail to create ctr with unresolvable CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	start_crio

	pod_id=$(crictl runp "$pod_config")

	prepare_ctr_with_unknown_cdidev
	run ! crictl create "$pod_id" "$ctr_config" "$pod_config"
}

@test "no CDI errors, fail to create ctr with unresolvable annotated CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	start_crio

	pod_id=$(crictl runp "$pod_config")

	annotate_ctr_with_unknown_cdidev
	run ! crictl create "$pod_id" "$ctr_config" "$pod_config"
}

@test "CDI registry refresh" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	start_crio

	pod_id=$(crictl runp "$pod_config")

	prepare_ctr_with_cdidev
	run ! crictl create "$pod_id" "$ctr_config" "$pod_config"

	write_cdi_spec
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	crictl start "$ctr_id"

	verify_injected_vendor0 "$ctr_id"
	verify_injected_loop8 "$ctr_id"
	verify_injected_loop9 "$ctr_id"
}

@test "CDI registry refresh, annotated CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	start_crio

	pod_id=$(crictl runp "$pod_config")

	annotate_ctr_with_cdidev
	run ! crictl create "$pod_id" "$ctr_config" "$pod_config"

	write_cdi_spec
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	run -0 crictl start "$ctr_id"

	verify_injected_vendor0 "$ctr_id"
	verify_injected_loop8 "$ctr_id"
	verify_injected_loop9 "$ctr_id"
}

@test "reload CRI-O CDI parameters" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	set_cdi_dir "$cdidir.no-such-dir"
	start_crio

	pod_id=$(crictl runp "$pod_config")

	annotate_ctr_with_cdidev
	run ! crictl create "$pod_id" "$ctr_config" "$pod_config"

	set_cdi_dir "$cdidir"
	reload_crio
	sleep 1

	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	crictl start "$ctr_id"

	verify_injected_vendor0 "$ctr_id"
	verify_injected_loop8 "$ctr_id"
	verify_injected_loop9 "$ctr_id"
}

@test "reload CRI-O CDI parameters, with annotated CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	set_cdi_dir "$cdidir.no-such-dir"
	start_crio

	pod_id=$(crictl runp "$pod_config")

	prepare_ctr_with_cdidev
	run ! crictl create "$pod_id" "$ctr_config" "$pod_config"

	set_cdi_dir "$cdidir"
	reload_crio
	sleep 1

	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	crictl start "$ctr_id"

	verify_injected_vendor0 "$ctr_id"
	verify_injected_loop8 "$ctr_id"
	verify_injected_loop9 "$ctr_id"
}

@test "CDI with errors, create ctr without CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	write_invalid_cdi_spec
	start_crio

	pod_id=$(crictl runp "$pod_config")

	prepare_ctr_without_cdidev
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")

	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"
}

@test "CDI with errors, create ctr with (unaffected) CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	write_invalid_cdi_spec
	start_crio

	pod_id=$(crictl runp "$pod_config")

	prepare_ctr_with_cdidev
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	grep "CDI registry has errors" "$CRIO_LOG"
	crictl start "$ctr_id"

	verify_injected_vendor0 "$ctr_id"
	verify_injected_loop8 "$ctr_id"
	verify_injected_loop9 "$ctr_id"
}

@test "CDI with errors, create ctr with (unaffected) annotated CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	write_invalid_cdi_spec
	start_crio

	pod_id=$(crictl runp "$pod_config")

	annotate_ctr_with_cdidev
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	run -0 grep "CDI registry has errors" "$CRIO_LOG"
	run -0 crictl start "$ctr_id"

	verify_injected_vendor0 "$ctr_id"
	verify_injected_loop8 "$ctr_id"
	verify_injected_loop9 "$ctr_id"
}
