#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function detect_irqbalance_config() {
	# debian/ubuntu
	[ -f /etc/default/irqbalance ] && echo "/etc/default/irqbalance"
	# fedora/centos/RHEL
	[ -f /etc/sysconfig/irqbalance ] && echo "/etc/sysconfig/irqbalance"
	# default - we need to have something!
	echo "/etc/sysconfig/irqbalance"
}

CONFIGLET="$CRIO_CONFIG_DIR/99-irqbalance.conf"
IRQBALANCE_CONF=$(detect_irqbalance_config)
# default setting
BANNEDCPUS_CONF="/etc/sysconfig/orig_irq_banned_cpus"

function setup() {
	setup_test
	start_crio
	# we don't uncondintionally restore because the irqbalance package may be missing
	# from the test system. if this is the case, one less thing to worry about, we can't
	# pollute the system state anyway :)
	if [ -f "$IRQBALANCE_CONF" ]; then
		cp -v "$IRQBALANCE_CONF" "$IRQBALANCE_CONF".bkp
	else
		# placeholder to make the rest of the suite simpler.
		# note it's intentionally commented.
		echo "# IRQBALANCE_BANNED_CPUS=" > "$IRQBALANCE_CONF"
		touch /tmp/.test_owns_irqbalance_conf
	fi
	if [ -f ${BANNEDCPUS_CONF} ]; then
		cp -v "$BANNEDCPUS_CONF" "$BANNEDCPUS_CONF".bkp
	else
		# empty file is fine and meaningful
		touch "$BANNEDCPUS_CONF"
		touch /tmp/.test_owns_bannedcpus_conf
	fi
}

function teardown() {
	cleanup_test
	# see setup about why we have these conditionals
	if [ -f "$IRQBALANCE_CONF".bkp ]; then
		mv -v "$IRQBALANCE_CONF".bkp "$IRQBALANCE_CONF"
	elif [ -f /tmp/.test_owns_irqbalance_conf ]; then
		rm -f "$IRQBALANCE_CONF"
		rm -f /tmp/.test_owns_irqbalance_conf
	# else how come?
	fi
	if [ -f "$BANNEDCPUS_CONF".bkp ]; then
		mv -v "$BANNEDCPUS_CONF".bkp "$BANNEDCPUS_CONF"
	elif [ -f /tmp/.test_owns_bannedcpus_conf ]; then
		rm -f "$BANNEDCPUS_CONF"
		rm -f /tmp/.test_owns_bannedcpus_conf
	fi
}

@test "irqbalance cpu ban list save" {
	# given
	if ! grep -Eq '^f{1,}$' /proc/irq/default_irq_smp_affinity; then
		skip "requires default IRQ smp affinity (not banned CPUs)"
	fi
	[ -f "$CONFIGLET" ] && rm -f "$CONFIGLET"

	[ -f "$BANNEDCPUS_CONF" ] && rm -f "$BANNEDCPUS_CONF"

	# when
	restart_crio

	# then
	len=$(awk 'END {print NR}' "$BANNEDCPUS_CONF")
	[ "$len" -eq 0 ]
}

@test "irqbalance cpu ban list restore - default" {
	# given
	if ! grep -Eq '^f{1,}$' /proc/irq/default_irq_smp_affinity; then
		skip "requires default IRQ smp affinity (not banned CPUs)"
	fi
	[ -f "$CONFIGLET" ] && rm -f "$CONFIGLET"

	truncate -s 0 "$BANNEDCPUS_CONF"

	# whem
	restart_crio

	# then
	len=$(awk 'END {print NR}' "$BANNEDCPUS_CONF")
	[ "$len" -eq 0 ]
	banned_cpus=$(sed -n 's/^IRQBALANCE_BANNED_CPUS=\(.*\)/\1/p' "$IRQBALANCE_CONF")
	[ -z "$banned_cpus" ]
}

# disable restore file, check it does NOT clear the irqbalance config

# explicit restore file, check it does SET the irqbalance config accordingly
