#!/usr/bin/env bats
# vim:set ft=bash :

# Security profiles distributed as OCI artifacts (KEP-6061). crictl does not
# support the PullSecurityProfile RPC and the OCI profile type yet, so the
# securityprofile test binary drives them.

load helpers

function setup() {
	if ! "$CHECKSECCOMP_BINARY"; then
		skip "seccomp is not enabled"
	fi

	setup_test
}

function teardown() {
	cleanup_test
}

PROFILES=registry.k8s.io/security-profiles-operator/seccomp-test-profiles
# The tags deny-chmod, permissive, invalid and oversized.
DENY_CHMOD=$PROFILES@sha256:9b9d6cb98e5c58448c341628db6b20e7c4866abc53fbd5e1f5a938739b26fbe6
PERMISSIVE=$PROFILES@sha256:e7279f26947707f3328552d72167700d80431179f3023e8efaa7a86f08dc5176
INVALID=$PROFILES@sha256:9dcf79f9985870bfe05bf0c94beb0ec24cd2391e762f8e3bb1853693cd3c904d
OVERSIZED=$PROFILES@sha256:49e08cf9b77aae392eb555fa1d9a3ea833052626746322151319fbd42f177e36
# The tag v1.5.1, the syscalls runc needs.
BASE_RUNC=registry.k8s.io/security-profiles-operator/base/runc@sha256:7e3cc47105b0b005555dcb3295595234f631c71e7f9982337117ca4675d0506f

DENY_MKDIR='{
	"defaultAction": "SCMP_ACT_ALLOW",
	"syscalls": [{"names": ["mkdir", "mkdirat"], "action": "SCMP_ACT_ERRNO", "errnoRet": 1}]
}'

DENY_MODE_777='{
	"defaultAction": "SCMP_ACT_ALLOW",
	"syscalls": [
		{"names": ["chmod"], "action": "SCMP_ACT_ERRNO", "errnoRet": 1,
			"args": [{"index": 1, "value": 511, "op": "SCMP_CMP_EQ"}]},
		{"names": ["fchmodat", "fchmodat2"], "action": "SCMP_ACT_ERRNO", "errnoRet": 1,
			"args": [{"index": 2, "value": 511, "op": "SCMP_CMP_EQ"}]}
	]
}'

function securityprofile() {
	"$SECURITYPROFILE_BINARY" -r "unix://$CRIO_SOCKET" "$@"
}

# Write a container config using the OCI seccomp profile $1, with the
# additional jq filter $2.
function container_config() {
	jq --arg REF "$1" \
		'.linux.security_context.seccomp = {"profile_type": 3, "oci_ref": $REF}'"${2:+ | $2}" \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container.json"
}

# Create and start the container of $TESTDIR/container.json and print its ID.
function start_container() {
	local pod ctr
	pod=$(crictl runp "$TESTDATA/sandbox_config.json")
	ctr=$(securityprofile create "$pod" "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")
	crictl start "$ctr" > /dev/null
	echo "$ctr"
}

@test "security profile OCI is reported as a runtime feature" {
	start_crio

	run securityprofile features

	[[ "$output" == "security_profile_oci=true" ]]
}

@test "security profile pulls only contact the registry once" {
	start_crio

	run securityprofile pull "$DENY_CHMOD"
	[[ "$output" == "cached=false" ]]

	run securityprofile pull "$DENY_CHMOD"
	[[ "$output" == "cached=true" ]]

	run securityprofile pull "$BASE_RUNC"
	[[ "$output" == "cached=false" ]]

	# One registry request per profile.
	[[ $(grep -c "Pulling security profile .* from the registry" "$CRIO_LOG") == 2 ]]

	# Profiles are not images, so the kubelet never garbage collects them.
	crictl images -o json | jq -e '[.images[].repoDigests[]? | select(contains("security-profiles-operator"))] | length == 0'
}

@test "security profile pulls reject invalid profiles permanently" {
	start_crio

	run ! securityprofile pull "$INVALID"
	[[ "$output" == *"SecurityProfileInvalid"*"SCMP_ACT_NOTIFY"* ]]

	# Present profiles are validated as well.
	run ! securityprofile pull "$INVALID"
	[[ "$output" == *"SecurityProfileInvalid"* ]]

	run ! securityprofile pull "$OVERSIZED"
	[[ "$output" == *"SecurityProfileInvalid"*"the maximum is 1048576 bytes"* ]]

	run ! securityprofile pull -kind AppArmor "$DENY_CHMOD"
	[[ "$output" == *"InvalidArgument"*"SecurityProfileInvalid"* ]]

	run ! securityprofile pull "$PROFILES:deny-chmod"
	[[ "$output" == *"InvalidArgument"*"SecurityProfileInvalid"*"not pinned by digest"* ]]
}

@test "security profile pulls honor the size limit" {
	CONTAINER_SECURITY_PROFILE_MAX_SIZE=100 start_crio

	run ! securityprofile pull "$DENY_CHMOD"
	[[ "$output" == *"SecurityProfileInvalid"*"the maximum is 100 bytes"* ]]

	run securityprofile pull "$PERMISSIVE"
	[[ "$output" == "cached=false" ]]
}

@test "security profile pulls report an unavailable registry" {
	start_crio

	run ! securityprofile pull "localhost:1/profile@sha256:1111111111111111111111111111111111111111111111111111111111111111"
	[[ "$output" == *"RegistryUnavailable"* ]]
}

@test "security profile pulls honor the signature policy" {
	SIGNATURE_POLICY="$INTEGRATION_ROOT/policy-signature.json" start_crio

	run ! securityprofile pull "$DENY_CHMOD"
	[[ "$output" == *"SignatureValidationFailed"* ]]
}

@test "security profile pulls honor namespace signature policies for present profiles" {
	start_crio

	run securityprofile pull -namespace unrestrictive "$DENY_CHMOD"
	[[ "$output" == "cached=false" ]]

	# Verified for another policy only, so pulled and rejected.
	run ! securityprofile pull -namespace restrictive "$DENY_CHMOD"
	[[ "$output" == *"SignatureValidationFailed"* ]]

	run securityprofile pull -namespace unrestrictive "$DENY_CHMOD"
	[[ "$output" == "cached=true" ]]
}

@test "OCI seccomp profile is merged with the runtime default" {
	start_crio
	securityprofile pull "$DENY_CHMOD"
	container_config "$DENY_CHMOD"

	ctr=$(start_container)

	run ! crictl exec --sync "$ctr" chmod 777 /tmp
	[[ "$output" == *"Operation not permitted"* ]]
	crictl exec --sync "$ctr" ls /

	# The profile allows by default, the runtime default does not.
	crictl inspect "$ctr" | jq -e '.info.runtimeSpec.linux.seccomp.defaultAction == "SCMP_ACT_ERRNO"'
	grep -q "SecurityProfileMergeConstrained.*profile=\"\?$DENY_CHMOD" "$CRIO_LOG"
}

@test "OCI seccomp profile cannot allow what the runtime default denies" {
	start_crio
	securityprofile pull "$PERMISSIVE"
	container_config "$PERMISSIVE"

	ctr=$(start_container)

	# The default profile allows unshare only with CAP_SYS_ADMIN.
	run ! crictl exec --sync "$ctr" unshare --user true
	[[ "$output" == *"Operation not permitted"* ]]
	crictl exec --sync "$ctr" ls /
}

@test "OCI seccomp profile is merged with argument filters of a base profile" {
	start_crio
	securityprofile pull "$PERMISSIVE"
	# No published profile has argument filters, so the base profile brings
	# them: it denies setting the mode 0777 (511), whichever syscall does it.
	echo "$DENY_MODE_777" > "$TESTDIR/base.json"
	container_config "$PERMISSIVE" \
		".linux.security_context.seccomp.base_profile = {\"type\": 2, \"localhost_ref\": \"$TESTDIR/base.json\"}"

	ctr=$(start_container)

	run ! crictl exec --sync "$ctr" chmod 777 /tmp
	[[ "$output" == *"Operation not permitted"* ]]
	crictl exec --sync "$ctr" chmod 755 /tmp
}

@test "OCI seccomp profile is merged with a localhost base profile" {
	start_crio
	securityprofile pull "$DENY_CHMOD"
	echo "$DENY_MKDIR" > "$TESTDIR/base.json"
	container_config "$DENY_CHMOD" \
		".linux.security_context.seccomp.base_profile = {\"type\": 2, \"localhost_ref\": \"$TESTDIR/base.json\"}"

	ctr=$(start_container)

	run ! crictl exec --sync "$ctr" mkdir /tmp/dir
	[[ "$output" == *"Operation not permitted"* ]]
	run ! crictl exec --sync "$ctr" chmod 777 /tmp
	[[ "$output" == *"Operation not permitted"* ]]
	grep -q "SecurityProfileMergeConstrained.*baseProfile=" "$CRIO_LOG"
}

@test "OCI seccomp profile is merged with the configured baseline" {
	echo "$DENY_MKDIR" > "$TESTDIR/baseline.json"
	CONTAINER_SECCOMP_BASELINE_PROFILE="$TESTDIR/baseline.json" start_crio
	securityprofile pull "$PERMISSIVE"
	container_config "$PERMISSIVE"

	ctr=$(start_container)

	run ! crictl exec --sync "$ctr" mkdir /tmp/dir
	[[ "$output" == *"Operation not permitted"* ]]
	crictl exec --sync "$ctr" chmod 777 /tmp
}

@test "invalid seccomp baseline profile fails the configuration" {
	setup_crio
	echo '{"defaultAction": "SCMP_ACT_WRONG"}' > "$TESTDIR/baseline.json"

	# The timeout ends a CRI-O that starts although it should not.
	CONTAINER_SECCOMP_BASELINE_PROFILE="$TESTDIR/baseline.json" \
		run ! timeout 60 "$CRIO_BINARY_PATH" -c "$CRIO_CONFIG" -d "$CRIO_CONFIG_DIR"
	[[ "$output" == *"seccomp baseline profile"* ]]
}

@test "OCI seccomp profile applies to the sandbox" {
	start_crio
	securityprofile pull "$DENY_CHMOD"
	jq --arg REF "$DENY_CHMOD" \
		'.linux.security_context.seccomp = {"profile_type": 3, "oci_ref": $REF}' \
		"$TESTDATA/sandbox_config.json" > "$TESTDIR/sandbox.json"

	pod=$(securityprofile runp "$TESTDIR/sandbox.json")

	crictl inspectp "$pod" | jq -e '.info.runtimeSpec.linux.seccomp.defaultAction == "SCMP_ACT_ERRNO"'
}

@test "OCI seccomp profile must have been pulled" {
	start_crio
	container_config "$DENY_CHMOD"
	pod=$(crictl runp "$TESTDATA/sandbox_config.json")

	run ! securityprofile create "$pod" "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json"
	[[ "$output" == *"has not been pulled"* ]]
}

@test "OCI profile type is rejected for AppArmor" {
	start_crio
	jq --arg REF "$DENY_CHMOD" \
		'.linux.security_context.apparmor = {"profile_type": 3, "oci_ref": $REF}' \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container.json"
	pod=$(crictl runp "$TESTDATA/sandbox_config.json")

	run ! securityprofile create "$pod" "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json"
	[[ "$output" == *"InvalidArgument"* ]]
}
