#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

function run_test() {
	CTR_CNT=$1
	EXEC_CNT=$2

	declare -A RUNTIME_MEMORY=(
		["conmon"]=0
		["conmonrs"]=0
	)
	declare -A CRIO_MEMORY=(
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

		SBOX_ID=$(crictl runp "$TESTDATA/sandbox_config.json")

		# Run multiple containers under the same sandbox
		for ((k = 0; k < CTR_CNT; k++)); do
			jq '.metadata.name = "ctr-'$k'"' "$TESTDATA/container_sleep.json" > "$TESTDIR/ctr.json"
			CTR_ID=$(crictl run "$TESTDIR/ctr.json" "$TESTDATA/sandbox_config.json")

			for ((i = 0; i < EXEC_CNT; i++)); do
				crictl exec --sync "$CTR_ID" ps aux
			done
		done

		CRIO_MEMORY[$RUNTIME]=$(cat "/sys/fs/cgroup/$CGROUP/memory.current")

		# Accumulate the conmon/-rs memory
		SCOPES=$(grep 'Running conmon under slice' "$CRIO_LOG" | sed -n 's;.*\(crio-conmon-.*\.scope\).*;\1;p')
		for SCOPE in $SCOPES; do
			MEMORY_BYTES=$(systemctl show -p MemoryCurrent "$SCOPE" | sed -n 's;.*\=\([0-9]\+\).*;\1;p')
			RUNTIME_MEMORY[$RUNTIME]=$((MEMORY_BYTES + ${RUNTIME_MEMORY[$RUNTIME]}))
		done

		cgdelete "$CGROUP_CONTROLLER:$CGROUP"
		crictl rmp -f "$SBOX_ID"
		truncate -s0 "$CRIO_LOG"
		stop_crio_no_clean
	done

	printf "\nTest results using %d containers and %d execs per container:\n" "$CTR_CNT" "$EXEC_CNT" >&3
	printf "conmon:\t\t%dkb\tconmonrs:\t\t%dkb\t(diff: %dkb)\n" \
		$((RUNTIME_MEMORY["conmon"] / 1024)) \
		$((RUNTIME_MEMORY["conmonrs"] / 1024)) \
		$(((RUNTIME_MEMORY["conmonrs"] - RUNTIME_MEMORY["conmon"]) / 1024)) >&3

	printf "CRI-O (conmon):\t%dkb\tCRI-O (conmonrs):\t%dkb\t(diff: %dkb)\n" \
		$((CRIO_MEMORY["conmon"] / 1024)) \
		$((CRIO_MEMORY["conmonrs"] / 1024)) \
		$(((CRIO_MEMORY["conmonrs"] - CRIO_MEMORY["conmon"]) / 1024)) >&3

	printf "Both (conmon):\t%dkb\tBoth (conmonrs):\t%dkb\t(diff: %dkb)\n\n" \
		$(((RUNTIME_MEMORY["conmon"] + CRIO_MEMORY["conmon"]) / 1024)) \
		$(((RUNTIME_MEMORY["conmonrs"] + CRIO_MEMORY["conmonrs"]) / 1024)) \
		$((((RUNTIME_MEMORY["conmonrs"] + CRIO_MEMORY["conmonrs"]) - (RUNTIME_MEMORY["conmon"] + CRIO_MEMORY["conmon"])) / 1024)) >&3

}

@test "compare conmon vs conmonrs using a single container without exec" {
	run_test 1 0
}

@test "compare conmon vs conmonrs using a single container with exec" {
	run_test 1 50
}

@test "compare conmon vs conmonrs using five containers in a pod without exec" {
	run_test 5 0
}

@test "compare conmon vs conmonrs using five containers in a pod with exec" {
	run_test 5 50
}

@test "compare conmon vs conmonrs using fifty containers in a pod without exec" {
	run_test 50 0
}

@test "compare conmon vs conmonrs using fifty containers in a pod with exec" {
	run_test 50 50
}
