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

function run_cmd() {
	while [[ $1 == -* || $1 == '!' ]]; do
		case "$1" in
		'!')
			expected_status=-1
			;;
		-[0-9]*)
			expected_status=${1#-}
			;;
		esac
		shift
	done

	run "$@"

	if [[ -n "$expected_status" ]]; then
		if [[ "$expected_status" = "-1" ]]; then
			if [[ "$status" -eq 0 ]]; then
				BATS_ERROR_SUFFIX=", expected nonzero exit code, got $status"
				return 1
			fi
		elif [[ "$status" -ne "$expected_status" ]]; then
			# shellcheck disable=SC2034
			BATS_ERROR_SUFFIX=", expected exit status $expected_status, got $status"
			return 1
		fi
	fi
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
	run_cmd -0 crictl exec --sync "$1" sh -c 'echo $VENDOR0'
	[ "$output" = "injected" ]
}

function verify_injected_loop8() {
	# shellcheck disable=SC2016
	run_cmd -0 crictl exec --sync "$1" sh -c 'echo $LOOP8'
	[ "$output" = "present" ]
	run_cmd -0 crictl exec --sync "$1" sh -c 'stat -c %t.%T /dev/loop8'
	[ "$output" = "7.8" ]
	run_cmd -0 crictl exec --sync "$1" sh -c 'stat -c %a /dev/loop8'
	[ "$output" = "640" ]
}

function verify_injected_loop9() {
	# shellcheck disable=SC2016
	run_cmd -0 crictl exec --sync "$1" sh -c 'echo $LOOP9'
	[ "$output" = "present" ]
	run_cmd -0 crictl exec --sync "$1" sh -c 'stat -c %t.%T /dev/loop9'
	[ "$output" = "7.9" ]
	run_cmd -0 crictl exec --sync "$1" sh -c 'stat -c %a /dev/loop9'
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

function prepare_ctr_with_cdidev {
	jq ".annotations |= . + { \"cdi.k8s.io/test\": \"vendor0.com/device=loop8,vendor0.com/device=loop9\" }" \
		"$TESTDATA/container_sleep.json" > "$ctr_config"
}

function prepare_ctr_with_unknown_cdidev {
	jq ".annotations |= . + { \"cdi.k8s.io/test\": \"vendor0.com/device=loop10\" }" \
		"$TESTDATA/container_sleep.json" > "$ctr_config"
}

@test "no CDI errors, create ctr without CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	start_crio

	run_cmd -0 crictl runp "$pod_config"
	pod_id="$output"

	prepare_ctr_without_cdidev
	run_cmd -0 crictl create "$pod_id" "$ctr_config" "$pod_config"
	ctr_id="$output"

	run_cmd -0 crictl start "$ctr_id"
	run_cmd -0 wait_until_exit "$ctr_id"
}

@test "no CDI errors, create ctr with CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	start_crio

	run_cmd -0 crictl runp "$pod_config"
	pod_id="$output"

	prepare_ctr_with_cdidev
	run_cmd -0 crictl create "$pod_id" "$ctr_config" "$pod_config"
	ctr_id="$output"
	crictl start "$ctr_id"

	verify_injected_vendor0 "$ctr_id"
	verify_injected_loop8 "$ctr_id"
	verify_injected_loop9 "$ctr_id"
}

@test "no CDI errors, fail to create ctr with unresolvable CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	start_crio

	run_cmd -0 crictl runp "$pod_config"
	pod_id="$output"

	prepare_ctr_with_unknown_cdidev
	run_cmd ! crictl create "$pod_id" "$ctr_config" "$pod_config"
}

@test "CDI registry refresh" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	start_crio

	run_cmd -0 crictl runp "$pod_config"
	pod_id="$output"

	prepare_ctr_with_cdidev
	run_cmd ! crictl create "$pod_id" "$ctr_config" "$pod_config"

	write_cdi_spec
	run_cmd -0 crictl create "$pod_id" "$ctr_config" "$pod_config"
	ctr_id="$output"
	run_cmd -0 crictl start "$ctr_id"

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

	run_cmd -0 crictl runp "$pod_config"
	pod_id="$output"

	prepare_ctr_with_cdidev
	run_cmd ! crictl create "$pod_id" "$ctr_config" "$pod_config"

	set_cdi_dir "$cdidir"
	reload_crio
	sleep 1

	run_cmd -0 crictl create "$pod_id" "$ctr_config" "$pod_config"
	ctr_id="$output"
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

	run_cmd -0 crictl runp "$pod_config"
	pod_id="$output"

	prepare_ctr_without_cdidev
	run_cmd -0 crictl create "$pod_id" "$ctr_config" "$pod_config"
	ctr_id="$output"

	run_cmd -0 crictl start "$ctr_id"
	run_cmd -0 wait_until_exit "$ctr_id"
}

@test "CDI with errors, create ctr with (unaffected) CDI devices" {
	if [[ -n "$CONTAINER_UID_MAPPINGS" ]]; then
		skip "CDI tests for user namespace"
	fi
	write_cdi_spec
	write_invalid_cdi_spec
	start_crio

	run_cmd -0 crictl runp "$pod_config"
	pod_id="$output"

	prepare_ctr_with_cdidev
	run_cmd -0 crictl create "$pod_id" "$ctr_config" "$pod_config"
	ctr_id="$output"
	run_cmd -0 grep "CDI registry has errors" "$CRIO_LOG"
	run_cmd -0 crictl start "$ctr_id"

	verify_injected_vendor0 "$ctr_id"
	verify_injected_loop8 "$ctr_id"
	verify_injected_loop9 "$ctr_id"
}
