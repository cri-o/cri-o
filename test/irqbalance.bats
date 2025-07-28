#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function detect_irqbalance_config() {
	# debian/ubuntu
	[ -f /etc/default/irqbalance ] && echo "/etc/default/irqbalance"
	# fedora/centos/RHEL
	[ -f /etc/sysconfig/irqbalance ] && echo "/etc/sysconfig/irqbalance"
	# default
	echo ""
}

function setup_file() {
	if ! command -v irqbalance > /dev/null; then
		skip "irqbalance not found."
	fi
	IRQBALANCE_CONF=$(detect_irqbalance_config)
	if [ -z "$IRQBALANCE_CONF" ]; then
		echo "error: unable to find irqbalance config file"
		return 1
	fi
	CONFIGLET="$CRIO_CONFIG_DIR/99-irqbalance.conf"

	mkdir -p "/etc/sysconfig"
	BANNEDCPUS_CONF="/etc/sysconfig/orig_irq_banned_cpus"

	export IRQBALANCE_CONF
	export CONFIGLET
	export BANNEDCPUS_CONF

	export BATS_NO_PARALLELIZE_WITHIN_FILE=true
}

function setup_serial_test() {
	setup_test
	stop_crio
	# we don't unconditionally restore because the irqbalance package may be missing
	# from the test system. if this is the case, one less thing to worry about, we can't
	# pollute the system state anyway :)
	if [ -f "$IRQBALANCE_CONF" ]; then
		cp -v "$IRQBALANCE_CONF" "$IRQBALANCE_CONF".bkp
	else
		# placeholder to make the rest of the suite simpler.
		touch "$IRQBALANCE_CONF"
		touch /tmp/.test_owns_irqbalance_conf
	fi
	if [ -f "$BANNEDCPUS_CONF" ]; then
		cp -v "$BANNEDCPUS_CONF" "$BANNEDCPUS_CONF".bkp
	else
		# empty file is fine and meaningful
		touch "$BANNEDCPUS_CONF"
		touch /tmp/.test_owns_bannedcpus_conf
	fi
}

function teardown_serial_test() {
	cleanup_test
	stop_crio
	# see setup about why we have these conditionals
	if [ -f "$IRQBALANCE_CONF".bkp ]; then
		mv -v "$IRQBALANCE_CONF".bkp "$IRQBALANCE_CONF"
	elif [ -f /tmp/.test_owns_irqbalance_conf ]; then
		rm -f "$IRQBALANCE_CONF"
		rm -f /tmp/.test_owns_irqbalance_conf
	fi

	if [ -f "$BANNEDCPUS_CONF".bkp ]; then
		mv -v "$BANNEDCPUS_CONF".bkp "$BANNEDCPUS_CONF"
	elif [ -f /tmp/.test_owns_bannedcpus_conf ]; then
		rm -f "$BANNEDCPUS_CONF"
		rm -f /tmp/.test_owns_bannedcpus_conf
	fi
}

function hex_to_cpuset() {
	# Convert a hexadecimal CPU mask to a space-separated list of CPU numbers
	# Each hex digit represents 4 CPUs (bits 0-3)
	# Example: "f" (hex) = 1111 (binary) = CPUs 0,1,2,3
	local hex_mask="$1"
	local cpus=()

	# Process each hexadecimal character from left to right
	for ((i = 0; i < ${#hex_mask}; i++)); do
		# Extract one hex character at position i
		local char=${hex_mask:$i:1}

		# Convert hex character to decimal (e.g., 'f' -> 15, 'a' -> 10)
		local decimal_char=$((16#$char))

		# Check each of the 4 bits represented by this hex digit
		for ((j = 0; j < 4; j++)); do
			# Use bitwise AND to check if bit j is set
			# (1 << j) creates a mask: j=0->1, j=1->2, j=2->4, j=3->8
			if [[ $((decimal_char & (1 << j))) != 0 ]]; then
				# If bit j is set, calculate the corresponding CPU number
				# CPU = (hex_position * 4) + bit_position
				cpus+=($((i * 4 + j)))
			fi
		done
	done

	echo "${cpus[@]}"
}

function expand_cpuset() {
	# Convert CPU range notation (e.g., "0-19,24,25-27") to space-separated list
	# Example: "0-3,5" -> "0 1 2 3 5"
	local cpuset="$1"
	local cpus=()

	# Split by comma to get individual groups
	IFS=',' read -ra groups <<< "$cpuset"

	for group in "${groups[@]}"; do
		# Check if this group contains a range (has a dash)
		if [[ "$group" =~ ^[0-9]+-[0-9]+$ ]]; then
			# Split the range by dash
			local start=${group%-*} # Everything before the last dash
			local end=${group#*-}   # Everything after the first dash

			# Expand the range
			for ((i = start; i <= end; i++)); do
				cpus+=("$i")
			done
		else
			# Single CPU number, not a range
			cpus+=("$group")
		fi
	done

	echo "${cpus[@]}"
}

# irqbalance tests have to run in sequence
# shellcheck disable=SC2218
@test "irqbalance tests (in sequence)" {
	irqbalance_cpu_ban_list_save
	irqbalance_cpu_ban_list_restore_default
	irqbalance_cpu_ban_list_restore_disable_and_file_missing
	irqbalance_cpu_ban_list_restore_disable
	irqbalance_cpu_ban_list_restore_explicit_file
}

# given
#	there is no previous status of banned cpus
# when
#	cri-o starts using the proper irqbalance service configuration file
# then
#	we expect cri-o to save the irqbalance banned cpus mask in a file
# 	pointed by the "$BANNEDCPUS_CONF" env var
#	and the mask must have value stated in "IRQBALANCE_BANNED_CPULIST" field
#   from irqbalance service configuration file.
irqbalance_cpu_ban_list_save() {
	setup_serial_test
	# given
	if ! grep -Eq '^[1,3,7,f]{1,}$' /proc/irq/default_smp_affinity; then
		skip "requires default IRQ smp affinity (not banned CPUs)"
	fi
	[ -f "$CONFIGLET" ] && rm -f "$CONFIGLET"

	[ -f "$BANNEDCPUS_CONF" ] && rm -f "$BANNEDCPUS_CONF"

	# Check if IRQBALANCE_BANNED_CPULIST line exists in the config file
	if ! grep -q "^IRQBALANCE_BANNED_CPULIST=" "$IRQBALANCE_CONF"; then
		skip "IRQBALANCE_BANNED_CPULIST not configured in $IRQBALANCE_CONF"
	fi

	local expected_banned_cpus
	expected_banned_cpus=$(sed -n 's/^IRQBALANCE_BANNED_CPULIST=\"\?\([^\"]*\)\"\?/\1/p' "$IRQBALANCE_CONF")

	# when
	IRQBALANCE_CONFIG_FILE="${IRQBALANCE_CONF}" IRQBALANCE_CONFIG_RESTORE_FILE="$BANNEDCPUS_CONF" start_crio

	# then
	if [ ! -f "$BANNEDCPUS_CONF" ]; then
		echo "error: ${BANNEDCPUS_CONF} file should have been created by CRI-o"
		return 2
	fi

	local banned_cpus
	banned_cpus=$(cat "$BANNEDCPUS_CONF")
	[ "$expected_banned_cpus" == "$banned_cpus" ]

	teardown_serial_test
}

# given
#	there is a previous status of banned cpus saved by cri-o
# when
#	cri-o starts using the proper irqbalance service configuration file
# then
#	we expect cri-o to read the irqbalance banned cpus mask from a file
# 	pointed by the "$BANNEDCPUS_CONF" env var
#	and save the mask value in "IRQBALANCE_BANNED_CPULIST" field
#   of irqbalance service configuration file.
irqbalance_cpu_ban_list_restore_default() {
	setup_serial_test
	# given
	if ! grep -Eq '^[1,3,7,f]{1,}$' /proc/irq/default_smp_affinity; then
		skip "requires default IRQ smp affinity (not banned CPUs)"
	fi
	[ -f "$CONFIGLET" ] && rm -f "$CONFIGLET"

	echo "IRQBALANCE_BANNED_CPUS=\"0\"" > "${IRQBALANCE_CONF}"
	echo "IRQBALANCE_BANNED_CPULIST=-" >> "${IRQBALANCE_CONF}"

	local banned_cpus_for_conf
	banned_cpus_for_conf=$(cat /proc/irq/default_smp_affinity)
	echo "$banned_cpus_for_conf" > "$BANNEDCPUS_CONF"

	# when
	IRQBALANCE_CONFIG_FILE="${IRQBALANCE_CONF}" IRQBALANCE_CONFIG_RESTORE_FILE="$BANNEDCPUS_CONF" start_crio

	# then
	local banned_cpus
	banned_cpus=$(sed -n 's/^IRQBALANCE_BANNED_CPULIST=\"\?\([^\"]*\)\"\?/\1/p' "$IRQBALANCE_CONF")
	[ "$(hex_to_cpuset "$banned_cpus_for_conf")" == "$(expand_cpuset "$banned_cpus")" ]

	teardown_serial_test
}

# restore option disabled. Check it does not disturb previous behaviour.
# given
#	there is no previous status of banned cpus saved by cri-o
# when
#	cri-o starts using the proper irqbalance service configuration file
#   and we explicitly disable the restore file option
# then
#	restore option does not disturb cri-o behaviour
#	so it reads banned cpus mask from "IRQBALANCE_BANNED_CPULIST" field
#   and save it in a file pointer by "BANNEDCPUS_CONF"  env var
irqbalance_cpu_ban_list_restore_disable_and_file_missing() {
	setup_serial_test
	# given
	if ! grep -Eq '^[1,3,7,f]{1,}$' /proc/irq/default_smp_affinity; then
		skip "requires default IRQ smp affinity (not banned CPUs)"
	fi
	[ -f "$CONFIGLET" ] && rm -f "$CONFIGLET"

	local expected_banned_cpus
	expected_banned_cpus=$(sed -n 's/^IRQBALANCE_BANNED_CPULIST=\"\?\([^\"]*\)\"\?/\1/p' "$IRQBALANCE_CONF")

	[ -f "$BANNEDCPUS_CONF" ] && rm -f "$BANNEDCPUS_CONF"

	# when
	IRQBALANCE_CONFIG_FILE="${IRQBALANCE_CONF}" start_crio

	# then
	local banned_cpus
	banned_cpus=$(sed -n 's/^IRQBALANCE_BANNED_CPULIST=\"\?\([^\"]*\)\"\?/\1/p' "$IRQBALANCE_CONF")

	[ "$expected_banned_cpus" == "$banned_cpus" ] && [ ! -f "$BANNEDCPUS_CONF" ]

	teardown_serial_test
}

# restore option disabled. Check it does not disturb previous behaviour.
# given
#	there is a previous status of banned cpus saved by cri-o in a file
#   pointer by "BANNEDCPUS_CONF" env var
# when
#	cri-o starts using the proper irqbalance service configuration file
#   and we explicitly disable the restore file option
# then
#	restore option does not disturb cri-o behaviour
#	so cri-o reads banned cpus mask from "IRQBALANCE_BANNED_CPULIST" field
#   and save it in a file pointer by "BANNEDCPUS_CONF"  env var
irqbalance_cpu_ban_list_restore_disable() {
	setup_serial_test
	# given
	if ! grep -Eq '^[1,3,7,f]{1,}$' /proc/irq/default_smp_affinity; then
		skip "requires default IRQ smp affinity (not banned CPUs)"
	fi
	[ -f "$CONFIGLET" ] && rm -f "$CONFIGLET"

	local expected_banned_cpus
	expected_banned_cpus=$(sed -n 's/^IRQBALANCE_BANNED_CPULIST=\"\?\([^\"]*\)\"\?/\1/p' "$IRQBALANCE_CONF")

	local banned_cpus_for_conf
	banned_cpus_for_conf=$(cat /proc/irq/default_smp_affinity)
	echo "$banned_cpus_for_conf" > "$BANNEDCPUS_CONF"

	# when
	IRQBALANCE_CONFIG_FILE="${IRQBALANCE_CONF}" start_crio

	# then
	local banned_cpus
	banned_cpus=$(sed -n 's/^IRQBALANCE_BANNED_CPULIST=\"\?\([^\"]*\)\"\?/\1/p' "$IRQBALANCE_CONF")

	[ "$expected_banned_cpus" == "$banned_cpus" ]

	teardown_serial_test
}

# explicit restore file. Check it uses it instead of any other file.
# given
#	there is no previous status of banned cpus saved by cri-o
# when
#	cri-o starts using the proper irqbalance service configuration file
#   and the restore file option pointing to an existing file.
# then
#	cri-o should read banned cpus mask from restore file
#	and save it in "IRQBALANCE_BANNED_CPULIST" field.
irqbalance_cpu_ban_list_restore_explicit_file() {
	setup_serial_test
	# given
	if ! grep -Eq '^[1,3,7,f]{1,}$' /proc/irq/default_smp_affinity; then
		skip "requires default IRQ smp affinity (not banned CPUs)"
	fi
	[ -f "$CONFIGLET" ] && rm -f "$CONFIGLET"

	[ -f "$BANNEDCPUS_CONF" ] && rm -f "$BANNEDCPUS_CONF"

	echo "IRQBALANCE_BANNED_CPUS=\"0\"" > "${IRQBALANCE_CONF}"
	echo "IRQBALANCE_BANNED_CPULIST=-" >> "${IRQBALANCE_CONF}"

	local irqbalance_restore_file
	irqbalance_restore_file="$(mktemp /tmp/irq-restore.XXXXXXXXX)"

	cat /proc/irq/default_smp_affinity > "$irqbalance_restore_file"

	local banned_cpus_for_restore
	banned_cpus_for_restore=$(cat "$irqbalance_restore_file")

	# when
	IRQBALANCE_CONFIG_FILE="${IRQBALANCE_CONF}" IRQBALANCE_CONFIG_RESTORE_FILE="${irqbalance_restore_file}" start_crio

	# then
	local banned_cpus
	banned_cpus=$(sed -n 's/^IRQBALANCE_BANNED_CPULIST=\"\?\([^\"]*\)\"\?/\1/p' "$IRQBALANCE_CONF")

	# when a explicit file is used to restore default one is completely ignored,
	# and so, it should not be created.( as it did not existed before)
	[ "$banned_cpus_for_restore" == "$banned_cpus" ] && [ ! -f "$BANNEDCPUS_CONF" ]

	teardown_serial_test
}
