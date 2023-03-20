#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

function remove_images_by() {
	local by="$1"
	local image=quay.io/crio/fedora-crio-ci

	start_crio
	# Add a second name to the image.
	copyimg --image-name="$image":latest --add-name="$image":othertag
	# Get the list of image names and IDs.
	output=$(crictl images -v)
	[ "$output" != "" ]
	# Cycle through each name, removing it by either name or id.
	# When removing by name, the image that we assigned a second name to
	# should still be around when we get to removing its second name.
	for tag in $(echo "$output" | awk '$1 == "'"$by"'" {print $2}'); do
		crictl rmi "$tag"
	done
	# List all images and their names.  There should be none now.
	output=$(crictl images --quiet)
	[ "$output" = "" ]
}

@test "image remove with multiple names, by name" {
	remove_images_by "RepoTags:"
}

@test "image remove with multiple names, by ID" {
	remove_images_by "ID:"
}
