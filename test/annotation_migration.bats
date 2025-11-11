#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	sboxconfig="$TESTDIR/sandbox_config.json"
	ctrconfig="$TESTDIR/container_config.json"
}

function teardown() {
	cleanup_test
}

# Test V2 annotation: userns-mode
@test "v2 userns-mode annotation should work" {
	# Check if user namespaces are properly configured
	if ! grep -q "^containers:" /etc/subuid 2> /dev/null || ! grep -q "^containers:" /etc/subgid 2> /dev/null; then
		skip "user namespace not configured (containers user not in /etc/subuid or /etc/subgid)"
	fi

	create_workload_with_allowed_annotation "userns-mode.crio.io"
	start_crio

	jq '.annotations."userns-mode.crio.io" = "auto"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test backward compatibility: V1 annotation still works
@test "v1 userns-mode annotation should still work" {
	# Check if user namespaces are properly configured
	if ! grep -q "^containers:" /etc/subuid 2> /dev/null || ! grep -q "^containers:" /etc/subgid 2> /dev/null; then
		skip "user namespace not configured (containers user not in /etc/subuid or /etc/subgid)"
	fi

	create_workload_with_allowed_annotation "io.kubernetes.cri-o.userns-mode"
	start_crio

	jq '.annotations."io.kubernetes.cri-o.userns-mode" = "auto"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test V2 annotation: umask
@test "v2 umask annotation should work" {
	setup_crio
	create_runtime_with_allowed_annotation "umask" "umask.crio.io"
	start_crio_no_setup

	pod_id=$(crictl runp <(jq '.annotations."umask.crio.io" = "022"' "$TESTDATA"/sandbox_config.json))
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Confirm that the umask (022) is applied
	umask_output=$(crictl exec "$ctr_id" grep Umask /proc/1/status)
	[[ "$umask_output" == *"22"* ]]
}

# Test backward compatibility: V1 umask annotation
@test "v1 umask annotation should still work" {
	setup_crio
	create_runtime_with_allowed_annotation "umask" "io.kubernetes.cri-o.umask"
	start_crio_no_setup

	pod_id=$(crictl runp <(jq '.annotations."io.kubernetes.cri-o.umask" = "077"' "$TESTDATA"/sandbox_config.json))
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Confirm that the umask (077) is applied
	umask_output=$(crictl exec "$ctr_id" grep Umask /proc/1/status)
	[[ "$umask_output" == *"77"* ]]
}

# Test V2 annotation: shm-size
@test "v2 shm-size annotation should work" {
	create_workload_with_allowed_annotation "shm-size.crio.io"
	start_crio

	jq '.annotations."shm-size.crio.io" = "128Mi"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Verify shm size is set correctly (128Mi = 134217728 bytes)
	shm_size=$(crictl exec "$ctr_id" df /dev/shm | awk 'NR==2 {print $2}')
	[[ "$shm_size" -ge 131072 ]] # At least 128MB in KB
}

# Test backward compatibility: V1 shm-size annotation
@test "v1 shm-size annotation should still work" {
	create_workload_with_allowed_annotation "io.kubernetes.cri-o.ShmSize"
	start_crio

	jq '.annotations."io.kubernetes.cri-o.ShmSize" = "128Mi"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Verify shm size is set correctly
	shm_size=$(crictl exec "$ctr_id" df /dev/shm | awk 'NR==2 {print $2}')
	[[ "$shm_size" -ge 131072 ]]
}

# Test V2 annotation: devices
@test "v2 devices annotation should work" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	if [[ ! -e /dev/null ]]; then
		skip "device /dev/null not available"
	fi

	setup_crio
	create_runtime_with_allowed_annotation "device" "devices.crio.io"
	CONTAINER_ALLOWED_DEVICES="/dev/null" start_crio_no_setup

	jq '.annotations."devices.crio.io" = "/dev/null:/dev/qifoo:rwm"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Verify device is accessible
	output=$(crictl exec --sync "$ctr_id" sh -c "ls /dev/qifoo")
	[[ "$output" == "/dev/qifoo" ]]
}

# Test backward compatibility: V1 devices annotation
@test "v1 devices annotation should still work" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	if [[ ! -e /dev/null ]]; then
		skip "device /dev/null not available"
	fi

	setup_crio
	create_runtime_with_allowed_annotation "device" "io.kubernetes.cri-o.Devices"
	CONTAINER_ALLOWED_DEVICES="/dev/null" start_crio_no_setup

	jq '.annotations."io.kubernetes.cri-o.Devices" = "/dev/null:/dev/qifoo:rwm"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Verify device is accessible
	output=$(crictl exec --sync "$ctr_id" sh -c "ls /dev/qifoo")
	[[ "$output" == "/dev/qifoo" ]]
}

# Test V2 annotation: unified-cgroup
@test "v2 unified-cgroup annotation should work" {
	if ! is_cgroup_v2; then
		skip "cgroup v2 not available"
	fi

	create_workload_with_allowed_annotation "unified-cgroup.crio.io"
	start_crio

	jq '.annotations."unified-cgroup.crio.io" = "[{\"memory.min\": \"10485760\"}]"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test backward compatibility: V1 unified-cgroup annotation
@test "v1 unified-cgroup annotation should still work" {
	if ! is_cgroup_v2; then
		skip "cgroup v2 not available"
	fi

	create_workload_with_allowed_annotation "io.kubernetes.cri-o.UnifiedCgroup"
	start_crio

	jq '.annotations."io.kubernetes.cri-o.UnifiedCgroup" = "[{\"memory.min\": \"10485760\"}]"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test container-specific V2 annotation: seccomp-profile
@test "v2 seccomp-profile annotation for specific container should work" {
	create_workload_with_allowed_annotation "seccomp-profile.crio.io"
	start_crio

	# Create a basic seccomp profile
	mkdir -p "$TESTDIR"/seccomp
	cat > "$TESTDIR"/seccomp/profile.json << EOF
{
	"defaultAction": "SCMP_ACT_ALLOW",
	"syscalls": []
}
EOF

	# Set container-specific seccomp annotation using V2 format
	jq --arg profile "$TESTDIR/seccomp/profile.json" \
		'.annotations."seccomp-profile.crio.io/POD" = $profile' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test backward compatibility: V1 seccomp-profile annotation for specific container
@test "v1 seccomp-profile annotation for specific container should still work" {
	create_workload_with_allowed_annotation "seccomp-profile.kubernetes.cri-o.io"
	start_crio

	# Create a basic seccomp profile
	mkdir -p "$TESTDIR"/seccomp
	cat > "$TESTDIR"/seccomp/profile.json << EOF
{
	"defaultAction": "SCMP_ACT_ALLOW",
	"syscalls": []
}
EOF

	# Set container-specific seccomp annotation using V1 format
	jq --arg profile "$TESTDIR/seccomp/profile.json" \
		'.annotations."seccomp-profile.kubernetes.cri-o.io/POD" = $profile' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test V2 annotation: cgroup2-mount-hierarchy-rw
@test "v2 cgroup2-mount-hierarchy-rw annotation should work" {
	if ! is_cgroup_v2; then
		skip "cgroup v2 not available"
	fi

	create_workload_with_allowed_annotation "cgroup2-mount-hierarchy-rw.crio.io"
	start_crio

	jq '.annotations."cgroup2-mount-hierarchy-rw.crio.io" = "true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Check that cgroup2 is mounted as rw
	mount_info=$(crictl exec "$ctr_id" grep cgroup2 /proc/mounts)
	[[ "$mount_info" == *"rw"* ]]
}

# Test backward compatibility: V1 cgroup2-mount-hierarchy-rw annotation
@test "v1 cgroup2-mount-hierarchy-rw annotation should still work" {
	if ! is_cgroup_v2; then
		skip "cgroup v2 not available"
	fi

	create_workload_with_allowed_annotation "io.kubernetes.cri-o.cgroup2-mount-hierarchy-rw"
	start_crio

	jq '.annotations."io.kubernetes.cri-o.cgroup2-mount-hierarchy-rw" = "true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Check that cgroup2 is mounted as rw
	mount_info=$(crictl exec "$ctr_id" grep cgroup2 /proc/mounts)
	[[ "$mount_info" == *"rw"* ]]
}

# Test V2 annotation: try-skip-volume-selinux-label
@test "v2 try-skip-volume-selinux-label annotation should work" {
	if ! command -v getenforce &> /dev/null || [[ $(getenforce) == "Disabled" ]]; then
		skip "SELinux not available or disabled"
	fi

	create_workload_with_allowed_annotation "try-skip-volume-selinux-label.crio.io"
	start_crio

	jq '.annotations."try-skip-volume-selinux-label.crio.io" = "true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test backward compatibility: V1 try-skip-volume-selinux-label annotation
@test "v1 try-skip-volume-selinux-label annotation should still work" {
	if ! command -v getenforce &> /dev/null || [[ $(getenforce) == "Disabled" ]]; then
		skip "SELinux not available or disabled"
	fi

	create_workload_with_allowed_annotation "io.kubernetes.cri-o.TrySkipVolumeSELinuxLabel"
	start_crio

	jq '.annotations."io.kubernetes.cri-o.TrySkipVolumeSELinuxLabel" = "true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test container-specific V2 annotation with slash separator: unified-cgroup.crio.io/containerName
@test "v2 unified-cgroup annotation for specific container with slash separator should work" {
	if ! is_cgroup_v2; then
		skip "cgroup v2 not available"
	fi

	create_workload_with_allowed_annotation "unified-cgroup.crio.io"
	start_crio

	CONTAINER_NAME="test-container"

	jq --arg name "$CONTAINER_NAME" \
		'.annotations."unified-cgroup.crio.io/\($name)" = "[{\"memory.min\": \"10485760\"}]"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq --arg name "$CONTAINER_NAME" \
		'.metadata.name = $name' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$ctrconfig" "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test V2 annotation: seccomp-notifier-action
@test "v2 seccomp-notifier-action annotation should work" {
	if ! checkseccomp; then
		skip "seccomp feature not available"
	fi

	create_workload_with_allowed_annotation "seccomp-notifier-action.crio.io"
	start_crio

	jq '.annotations."seccomp-notifier-action.crio.io" = "stop"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test backward compatibility: V1 seccomp-notifier-action annotation
@test "v1 seccomp-notifier-action annotation should still work" {
	if ! checkseccomp; then
		skip "seccomp feature not available"
	fi

	create_workload_with_allowed_annotation "io.kubernetes.cri-o.seccompNotifierAction"
	start_crio

	jq '.annotations."io.kubernetes.cri-o.seccompNotifierAction" = "stop"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test V2 annotation: disable-fips
@test "v2 disable-fips annotation should work" {
	create_workload_with_allowed_annotation "disable-fips.crio.io"
	start_crio

	jq '.annotations."disable-fips.crio.io" = "true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test backward compatibility: V1 disable-fips annotation
@test "v1 disable-fips annotation should still work" {
	create_workload_with_allowed_annotation "io.kubernetes.cri-o.DisableFIPS"
	start_crio

	jq '.annotations."io.kubernetes.cri-o.DisableFIPS" = "true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test V2 annotation: link-logs
@test "v2 link-logs annotation should work" {
	if [[ $RUNTIME_TYPE == vm ]]; then
		skip "not applicable to vm runtime type"
	fi

	setup_crio
	create_runtime_with_allowed_annotation "logs" "link-logs.crio.io"
	start_crio_no_setup

	pod_log_dir="$TESTDIR/pod-logs"
	pod_uid=$(head -c 32 /proc/sys/kernel/random/uuid)
	mkdir -p "$pod_log_dir"

	jq --arg pod_log_dir "$pod_log_dir" --arg pod_uid "$pod_uid" \
		'.annotations."link-logs.crio.io" = "logging-volume" | .log_directory = $pod_log_dir | .metadata.uid = $pod_uid' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test backward compatibility: V1 link-logs annotation
@test "v1 link-logs annotation should still work" {
	if [[ $RUNTIME_TYPE == vm ]]; then
		skip "not applicable to vm runtime type"
	fi

	setup_crio
	create_runtime_with_allowed_annotation "logs" "io.kubernetes.cri-o.LinkLogs"
	start_crio_no_setup

	pod_log_dir="$TESTDIR/pod-logs"
	pod_uid=$(head -c 32 /proc/sys/kernel/random/uuid)
	mkdir -p "$pod_log_dir"

	jq --arg pod_log_dir "$pod_log_dir" --arg pod_uid "$pod_uid" \
		'.annotations."io.kubernetes.cri-o.LinkLogs" = "logging-volume" | .log_directory = $pod_log_dir | .metadata.uid = $pod_uid' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test V2 annotation: pod-linux-overhead
@test "v2 pod-linux-overhead annotation should work" {
	create_workload_with_allowed_annotation "pod-linux-overhead.crio.io"
	start_crio

	jq '.annotations."pod-linux-overhead.crio.io" = "{\"cpu\":\"100m\",\"memory\":\"50Mi\"}"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test backward compatibility: V1 pod-linux-overhead annotation
@test "v1 pod-linux-overhead annotation should still work" {
	create_workload_with_allowed_annotation "io.kubernetes.cri-o.PodLinuxOverhead"
	start_crio

	jq '.annotations."io.kubernetes.cri-o.PodLinuxOverhead" = "{\"cpu\":\"100m\",\"memory\":\"50Mi\"}"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test V2 annotation: pod-linux-resources
@test "v2 pod-linux-resources annotation should work" {
	create_workload_with_allowed_annotation "pod-linux-resources.crio.io"
	start_crio

	jq '.annotations."pod-linux-resources.crio.io" = "{\"cpu\":\"200m\",\"memory\":\"100Mi\"}"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test backward compatibility: V1 pod-linux-resources annotation
@test "v1 pod-linux-resources annotation should still work" {
	create_workload_with_allowed_annotation "io.kubernetes.cri-o.PodLinuxResources"
	start_crio

	jq '.annotations."io.kubernetes.cri-o.PodLinuxResources" = "{\"cpu\":\"200m\",\"memory\":\"100Mi\"}"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test V2 annotation: platform-runtime-path
@test "v2 platform-runtime-path annotation should work" {
	create_workload_with_allowed_annotation "platform-runtime-path.crio.io"
	start_crio

	jq '.annotations."platform-runtime-path.crio.io" = "/usr/bin/crun"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

# Test backward compatibility: V1 platform-runtime-path annotation
@test "v1 platform-runtime-path annotation should still work" {
	create_workload_with_allowed_annotation "io.kubernetes.cri-o.PlatformRuntimePath"
	start_crio

	jq '.annotations."io.kubernetes.cri-o.PlatformRuntimePath" = "/usr/bin/crun"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}
