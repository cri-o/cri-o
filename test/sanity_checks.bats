#!/usr/bin/env bats

# Sanity checks for test environment. If any of the below tests fail,
# the test environment is not to be trusted (i.e. bogus failures are
# quite possible), and the test environment need to be fixed.

# RHEL7 kernels should have this sysctl enabled, otherwise
# we can see EBUSY on mount point removal, container removal
# etc. It is set in production via runc rpm %postin script,
# and in CI via contrib/test/integration/main.yml.
# For more details, see
#  - https://bugzilla.redhat.com/show_bug.cgi?id=1823374#c17
#  - https://github.com/cri-o/cri-o/issues/3996
@test "if fs.may_detach_mounts is set" {
	file="/proc/sys/fs/may_detach_mounts"
	test -f "$file" || return 0

	grep "1" /proc/sys/fs/may_detach_mounts
}

# VM runtimes cannot pass host devices to privileged containers.
@test "vm runtime defaults privileged_without_host_devices to true" {
	common_sh="${BATS_TEST_DIRNAME:-.}/common.sh"

	# shellcheck disable=SC2016 # expanded in the subshell
	run env -u PRIVILEGED_WITHOUT_HOST_DEVICES \
		CRICTL_BINARY="${CRICTL_BINARY:-}" \
		CONMON_BINARY="${CONMON_BINARY:-}" \
		RUNTIME_TYPE=vm \
		bash -c '. "$1"; printf "%s" "$PRIVILEGED_WITHOUT_HOST_DEVICES"' _ "$common_sh"
	[ "$status" -eq 0 ]
	[ "$output" = "true" ]

	# An explicit value wins.
	# shellcheck disable=SC2016 # expanded in the subshell
	run env PRIVILEGED_WITHOUT_HOST_DEVICES=false \
		CRICTL_BINARY="${CRICTL_BINARY:-}" \
		CONMON_BINARY="${CONMON_BINARY:-}" \
		RUNTIME_TYPE=vm \
		bash -c '. "$1"; printf "%s" "$PRIVILEGED_WITHOUT_HOST_DEVICES"' _ "$common_sh"
	[ "$status" -eq 0 ]
	[ "$output" = "false" ]

	# Non-VM runtimes keep the CRI-O default.
	# shellcheck disable=SC2016 # expanded in the subshell
	run env -u PRIVILEGED_WITHOUT_HOST_DEVICES \
		CRICTL_BINARY="${CRICTL_BINARY:-}" \
		CONMON_BINARY="${CONMON_BINARY:-}" \
		RUNTIME_TYPE=oci \
		bash -c '. "$1"; printf "%s" "$PRIVILEGED_WITHOUT_HOST_DEVICES"' _ "$common_sh"
	[ "$status" -eq 0 ]
	[ -z "$output" ]
}
