#!/bin/bash

set -eu

get_major() {
	echo "$@" | grep -Po '^(\"|crio version )?\K\d(?=\..*)' || true
}

get_minor() {
	echo "$@" | grep -Po '^(\"|crio version )?\d\.\K\d+(?=\..*)' || true
}

perform_wipe() {
	if [[ $WIPE -eq 0 ]]; then
		echo "Wiping storage"
		rm -rf "$CONTAINERS_STORAGE_DIR"/*
		# best effort remove the parent directory. it may fail because it is mounted,
		# but we've already removed what we want to.
		rm -rf "$CONTAINERS_STORAGE_DIR" || true
		exit
	fi
	exit 0
}

check_versions_wipe_if_necessary() {
	# $1 should be the new version
	# $2 should be the old version

	# cast as integers to be used
	declare -i new=$1
	declare -i old=$2

	if [[ $old -lt $new ]]; then
		echo "New version detected"
		perform_wipe
		exit
	fi
}

main() {
	# Fail and don't update if current major or minor versions can't be read
	NEW_MAJOR_VERSION=$(get_major "$NEW_VERSION")
	if [[ -z "$NEW_MAJOR_VERSION" ]]; then
		>&2 echo "New major version not set"
		exit 1
	fi

	NEW_MINOR_VERSION=$(get_minor "$NEW_VERSION")
	if [[ -z "$NEW_MINOR_VERSION" ]]; then
		>&2 echo "New minor version not set"
		exit 1
	fi

	# Unconditionally update if there is no version file
	if ! test -f "$VERSION_FILE_LOCATION"; then
		echo "Old version not found"
		perform_wipe
		exit
	fi

	OLD_VERSION=$(cat "$VERSION_FILE_LOCATION")

	OLD_MAJOR_VERSION=$(get_major "$OLD_VERSION")
	if [[ -z "$OLD_MAJOR_VERSION" ]]; then
		>&2 echo "Invalid major version in version file"
		perform_wipe
		exit
	fi
	MAJOR_CHECK=$(check_versions_wipe_if_necessary "$NEW_MAJOR_VERSION" "$OLD_MAJOR_VERSION")
	MAJOR_CHECK_EC=$?
	if [[ ! -z "$MAJOR_CHECK" ]]; then
		echo "Reading major version in file exited with: $MAJOR_CHECK_EC and returned: $MAJOR_CHECK"
		exit $MAJOR_CHECK_EC
	fi

	OLD_MINOR_VERSION=$(get_minor "$OLD_VERSION")
	if [[ -z "$OLD_MINOR_VERSION" ]]; then
		>&2 echo "Invalid minor version in version file"
		perform_wipe
		exit
	fi
	MINOR_CHECK=$(check_versions_wipe_if_necessary "$NEW_MINOR_VERSION" "$OLD_MINOR_VERSION")
	MINOR_CHECK_EC=$?
	if [[ ! -z "$MINOR_CHECK" ]]; then
		echo "Reading minor version in file exited with: $MINOR_CHECK_EC and returned: $MINOR_CHECK"
		exit $MINOR_CHECK_EC
	fi

	# no update needed
	exit 0
}
