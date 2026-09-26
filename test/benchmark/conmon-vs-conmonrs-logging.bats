#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

function run_logging_test() {
	local CTR_CNT=$1
	local LOG_LEVEL=$2

	declare -A RUNTIME_MEMORY=(
		["conmon"]=0
		["conmonrs"]=0
	)
	declare -A CRIO_MEMORY=(
		["conmon"]=0
		["conmonrs"]=0
	)
	declare -A RUNTIME_CPU=(
		["conmon"]=0
		["conmonrs"]=0
	)
	declare -A LOG_GROWTH=(
		["conmon"]=0
		["conmonrs"]=0
	)

	setup_crio

	for RUNTIME in "${!RUNTIME_MEMORY[@]}"; do
		RUNTIME_TYPE=oci
		if [[ $RUNTIME == conmonrs ]]; then
			RUNTIME_TYPE=pod
		fi

		MONITOR_PATH="$(command -v "$RUNTIME")"
		cat << EOF > "$CRIO_CONFIG_DIR/99-runtimes.conf"
[crio.runtime]
default_runtime = "$RUNTIME"

[crio.runtime.runtimes.$RUNTIME]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_type = "$RUNTIME_TYPE"
monitor_path = "$MONITOR_PATH"
EOF
		unset CONTAINER_DEFAULT_RUNTIME
		unset CONTAINER_RUNTIMES

		start_crio_no_setup

		CGROUP=crio-test-$CRIO_PID
		CGROUP_CONTROLLER=memory

		cgcreate -g "$CGROUP_CONTROLLER:$CGROUP"
		cgclassify -g "$CGROUP_CONTROLLER:$CGROUP" "$CRIO_PID"

		# Measure initial log directory size
		LOG_DIR="/var/log/crio/pods"
		if [[ -d "$LOG_DIR" ]]; then
			INITIAL_LOG_SIZE=$(du -sb "$LOG_DIR" 2> /dev/null | cut -f1 || echo 0)
		else
			INITIAL_LOG_SIZE=0
		fi

		SBOX_ID=$(crictl runp "$TESTDATA/sandbox_config.json")

		# Create containers with logging
		for ((k = 0; k < CTR_CNT; k++)); do
			jq '.metadata.name = "ctr-'$k'"' "$TESTDATA/container_${LOG_LEVEL}_logging.json" > "$TESTDIR/ctr_${LOG_LEVEL}.json"
			crictl run "$TESTDIR/ctr_${LOG_LEVEL}.json" "$TESTDATA/sandbox_config.json" > /dev/null
		done

		# Let containers run for 60 seconds to generate logs
		sleep 60

		# Measure memory
		CRIO_MEMORY[$RUNTIME]=$(cat "/sys/fs/cgroup/$CGROUP/memory.current")

		# Accumulate runtime memory and CPU time
		SCOPES=$(grep 'Running conmon under slice' "$CRIO_LOG" | sed -n 's;.*\(crio-conmon-.*\.scope\).*;\1;p')
		for SCOPE in $SCOPES; do
			MEMORY_BYTES=$(systemctl show -p MemoryCurrent "$SCOPE" | sed -n 's;.*=\([0-9]\+\).*;\1;p')
			RUNTIME_MEMORY[$RUNTIME]=$((MEMORY_BYTES + ${RUNTIME_MEMORY[$RUNTIME]}))

			# Get CPU time in nanoseconds
			CPU_NSEC=$(systemctl show -p CPUUsageNSec "$SCOPE" | sed -n 's;.*=\([0-9]\+\).*;\1;p')
			if [[ -n "$CPU_NSEC" ]]; then
				RUNTIME_CPU[$RUNTIME]=$((CPU_NSEC + ${RUNTIME_CPU[$RUNTIME]}))
			fi
		done

		# Measure final log directory size
		if [[ -d "$LOG_DIR" ]]; then
			FINAL_LOG_SIZE=$(du -sb "$LOG_DIR" 2> /dev/null | cut -f1 || echo 0)
		else
			FINAL_LOG_SIZE=0
		fi
		LOG_GROWTH[$RUNTIME]=$((FINAL_LOG_SIZE - INITIAL_LOG_SIZE))

		cgdelete "$CGROUP_CONTROLLER:$CGROUP"
		crictl rmp -f "$SBOX_ID"
		truncate -s0 "$CRIO_LOG"
		stop_crio_no_clean
	done

	# Report results
	printf "\nTest results for %s logging with %d containers:\n\n" "$LOG_LEVEL" "$CTR_CNT" >&3

	# Memory results
	printf "Memory (KB):\n" >&3
	printf "  conmon:     %6dkb (runtime)  |  %6dkb (CRI-O)  |  %6dkb (total)\n" \
		$((RUNTIME_MEMORY["conmon"] / 1024)) \
		$((CRIO_MEMORY["conmon"] / 1024)) \
		$(((RUNTIME_MEMORY["conmon"] + CRIO_MEMORY["conmon"]) / 1024)) >&3
	printf "  conmonrs:   %6dkb (runtime)  |  %6dkb (CRI-O)  |  %6dkb (total)\n" \
		$((RUNTIME_MEMORY["conmonrs"] / 1024)) \
		$((CRIO_MEMORY["conmonrs"] / 1024)) \
		$(((RUNTIME_MEMORY["conmonrs"] + CRIO_MEMORY["conmonrs"]) / 1024)) >&3

	TOTAL_DIFF=$(((RUNTIME_MEMORY["conmonrs"] + CRIO_MEMORY["conmonrs"]) - (RUNTIME_MEMORY["conmon"] + CRIO_MEMORY["conmon"])))
	printf "  Difference: %+6dkb\n\n" $((TOTAL_DIFF / 1024)) >&3

	# Logging performance
	printf "Logging Performance:\n" >&3
	printf "  conmon:     %10d bytes in 60s (%d bytes/sec)\n" \
		"${LOG_GROWTH["conmon"]}" \
		$((LOG_GROWTH["conmon"] / 60)) >&3
	printf "  conmonrs:   %10d bytes in 60s (%d bytes/sec)\n\n" \
		"${LOG_GROWTH["conmonrs"]}" \
		$((LOG_GROWTH["conmonrs"] / 60)) >&3

	# CPU time
	printf "CPU Time:\n" >&3
	printf "  conmon:     %8d ms\n" $((RUNTIME_CPU["conmon"] / 1000000)) >&3
	printf "  conmonrs:   %8d ms\n" $((RUNTIME_CPU["conmonrs"] / 1000000)) >&3

	CPU_DIFF=$((RUNTIME_CPU["conmonrs"] - RUNTIME_CPU["conmon"]))
	printf "  Difference: %+8d ms\n\n" $((CPU_DIFF / 1000000)) >&3
}

# Light logging tests (5 lines/sec per container)
@test "light logging: 1 container" {
	run_logging_test 1 light
}

@test "light logging: 5 containers" {
	run_logging_test 5 light
}

@test "light logging: 10 containers" {
	run_logging_test 10 light
}

@test "light logging: 25 containers" {
	run_logging_test 25 light
}

@test "light logging: 50 containers" {
	run_logging_test 50 light
}

# Medium logging tests (100 lines/sec per container)
@test "medium logging: 1 container" {
	run_logging_test 1 medium
}

@test "medium logging: 5 containers" {
	run_logging_test 5 medium
}

@test "medium logging: 10 containers" {
	run_logging_test 10 medium
}

@test "medium logging: 25 containers" {
	run_logging_test 25 medium
}

# Heavy logging tests (1000 lines/sec per container)
@test "heavy logging: 1 container" {
	run_logging_test 1 heavy
}

@test "heavy logging: 5 containers" {
	run_logging_test 5 heavy
}

@test "heavy logging: 10 containers" {
	run_logging_test 10 heavy
}
