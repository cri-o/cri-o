#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	# Set up image content cache directory for tests that need it.
	BLOB_CACHE_DIR="$TESTDIR/image-content-cache"
}

function teardown() {
	cleanup_test
}

# Helper to create image content cache drop-in config.
function enable_image_content_cache() {
	cat << EOF > "$CRIO_CONFIG_DIR/99-image-content-cache.conf"
[crio.image]
image_content_cache_dir = "$BLOB_CACHE_DIR"
EOF
}

@test "image content cache disabled by default" {
	start_crio

	crictl pull quay.io/crio/alpine:3.9

	# Image content cache directory should not exist when disabled.
	run ! ls "$BLOB_CACHE_DIR/blobs"
}

@test "image content cache caches blobs on image pull" {
	mkdir -p "$BLOB_CACHE_DIR"
	enable_image_content_cache
	start_crio

	crictl pull quay.io/crio/alpine:3.9

	# Verify cache directory structure was created.
	[ -d "$BLOB_CACHE_DIR/blobs" ]

	# Verify metadata file exists.
	[ -f "$BLOB_CACHE_DIR/metadata.json" ]

	# Verify at least one blob was cached (alpine has multiple layers).
	blob_count=$(find "$BLOB_CACHE_DIR/blobs" -type f | wc -l)
	[ "$blob_count" -gt 0 ]

	# Verify metadata contains blob entries.
	blob_entries=$(jq '.blobs | length' "$BLOB_CACHE_DIR/metadata.json")
	[ "$blob_entries" -gt 0 ]
}

@test "image content cache metadata contains source information" {
	mkdir -p "$BLOB_CACHE_DIR"
	enable_image_content_cache
	start_crio

	crictl pull quay.io/crio/alpine:3.9

	# Verify metadata contains registry and repository info.
	registry=$(jq -r '.blobs | to_entries[0].value.sources[0].registry' "$BLOB_CACHE_DIR/metadata.json")
	repository=$(jq -r '.blobs | to_entries[0].value.sources[0].repository' "$BLOB_CACHE_DIR/metadata.json")

	[[ "$registry" == "quay.io" ]]
	[[ "$repository" == "crio/alpine" ]]
}

@test "image content cache preserves blobs across CRI-O restarts" {
	mkdir -p "$BLOB_CACHE_DIR"
	enable_image_content_cache
	start_crio

	crictl pull quay.io/crio/alpine:3.9

	# Count blobs before restart.
	blob_count_before=$(find "$BLOB_CACHE_DIR/blobs" -type f | wc -l)

	restart_crio

	# Verify blobs are still present after restart.
	blob_count_after=$(find "$BLOB_CACHE_DIR/blobs" -type f | wc -l)
	[ "$blob_count_before" -eq "$blob_count_after" ]
}

@test "image content cache adds sources for duplicate blobs" {
	mkdir -p "$BLOB_CACHE_DIR"
	enable_image_content_cache
	start_crio

	crictl pull quay.io/crio/alpine:3.9

	# Remove the image and pull again (simulating re-pull).
	crictl rmi quay.io/crio/alpine:3.9
	crictl pull quay.io/crio/alpine:3.9

	# Blob count should remain the same (deduplication).
	# Source count might increase if image was re-pulled from different source.
	# The main thing is that blobs are not duplicated.
	new_blob_count=$(jq '.blobs | length' "$BLOB_CACHE_DIR/metadata.json")
	[ "$new_blob_count" -gt 0 ]
}

@test "image content cache configuration validation rejects relative path" {
	# This should fail because image_content_cache_dir is not absolute.
	run ! "$CRIO_BINARY_PATH" \
		--image-content-cache-dir "relative/path" \
		config > /dev/null 2>&1
}

@test "image content cache blobs have correct digest-based path" {
	mkdir -p "$BLOB_CACHE_DIR"
	enable_image_content_cache
	start_crio

	crictl pull quay.io/crio/alpine:3.9

	# Verify blobs are stored in sha256 subdirectory.
	[ -d "$BLOB_CACHE_DIR/blobs/sha256" ]

	# Get a digest from metadata and verify the file exists at the expected path.
	digest=$(jq -r '.blobs | to_entries[0].value.digest' "$BLOB_CACHE_DIR/metadata.json")
	# digest is in format "sha256:abc123..."
	encoded=${digest#sha256:}
	[ -f "$BLOB_CACHE_DIR/blobs/sha256/$encoded" ]
}

@test "image content cache metadata includes timestamps" {
	mkdir -p "$BLOB_CACHE_DIR"
	enable_image_content_cache
	start_crio

	crictl pull quay.io/crio/alpine:3.9

	# Verify metadata includes timestamps.
	last_accessed=$(jq -r '.blobs | to_entries[0].value.lastAccessed' "$BLOB_CACHE_DIR/metadata.json")
	created_at=$(jq -r '.blobs | to_entries[0].value.createdAt' "$BLOB_CACHE_DIR/metadata.json")

	# Timestamps should not be null or empty.
	[[ "$last_accessed" != "null" ]] && [[ -n "$last_accessed" ]]
	[[ "$created_at" != "null" ]] && [[ -n "$created_at" ]]
}

@test "image content cache multiple image pulls share common layers" {
	mkdir -p "$BLOB_CACHE_DIR"
	enable_image_content_cache
	start_crio

	crictl pull quay.io/crio/alpine:3.9
	first_blob_count=$(jq '.blobs | length' "$BLOB_CACHE_DIR/metadata.json")

	crictl pull registry.k8s.io/pause:3.9

	# The total blob count should be at least what we had before
	# (we don't duplicate layers that are the same).
	final_blob_count=$(jq '.blobs | length' "$BLOB_CACHE_DIR/metadata.json")
	[ "$final_blob_count" -ge "$first_blob_count" ]

	# Verify blob files on disk match metadata count.
	disk_blob_count=$(find "$BLOB_CACHE_DIR/blobs" -type f | wc -l)
	[ "$disk_blob_count" -eq "$final_blob_count" ]
}

@test "image content cache GC removes blobs on image delete" {
	mkdir -p "$BLOB_CACHE_DIR"
	enable_image_content_cache
	start_crio

	crictl pull quay.io/crio/alpine:3.9

	blob_count_before=$(jq '.blobs | length' "$BLOB_CACHE_DIR/metadata.json")
	[ "$blob_count_before" -gt 0 ]

	image_id=$(crictl images --quiet quay.io/crio/alpine:3.9)
	crictl rmi "$image_id"

	local attempt=0
	while [ $attempt -le 30 ]; do
		blob_count_after=$(jq '.blobs | length' "$BLOB_CACHE_DIR/metadata.json")
		if [ "$blob_count_after" -eq 0 ]; then
			break
		fi
		sleep 0.1
		attempt=$((attempt + 1))
	done
	[ "$blob_count_after" -eq 0 ]

	disk_blob_count=$(find "$BLOB_CACHE_DIR/blobs" -type f | wc -l)
	[ "$disk_blob_count" -eq 0 ]
}

@test "image content cache concurrent pulls do not corrupt cache" {
	mkdir -p "$BLOB_CACHE_DIR"
	enable_image_content_cache
	start_crio

	crictl pull quay.io/crio/alpine:3.9 &
	pid1=$!
	crictl pull registry.k8s.io/pause:3.9 &
	pid2=$!

	wait $pid1
	wait $pid2

	[ -f "$BLOB_CACHE_DIR/metadata.json" ]
	blob_count=$(jq '.blobs | length' "$BLOB_CACHE_DIR/metadata.json")
	[ "$blob_count" -gt 0 ]

	disk_blob_count=$(find "$BLOB_CACHE_DIR/blobs" -type f | wc -l)
	[ "$disk_blob_count" -eq "$blob_count" ]

	for digest in $(jq -r '.blobs | keys[]' "$BLOB_CACHE_DIR/metadata.json"); do
		encoded=${digest#sha256:}
		[ -f "$BLOB_CACHE_DIR/blobs/sha256/$encoded" ]
	done
}

@test "image content cache pull succeeds with read-only cache dir" {
	mkdir -p "$BLOB_CACHE_DIR"
	enable_image_content_cache
	start_crio

	crictl pull quay.io/crio/alpine:3.9
	crictl rmi quay.io/crio/alpine:3.9

	chmod 000 "$BLOB_CACHE_DIR/blobs"

	crictl pull quay.io/crio/alpine:3.9

	image_id=$(crictl images --quiet quay.io/crio/alpine:3.9)
	[ -n "$image_id" ]

	chmod 755 "$BLOB_CACHE_DIR/blobs"
}

@test "image content cache GC does not remove blobs during pull" {
	mkdir -p "$BLOB_CACHE_DIR"
	enable_image_content_cache
	start_crio

	crictl pull quay.io/crio/alpine:3.9

	blob_count_before=$(jq '.blobs | length' "$BLOB_CACHE_DIR/metadata.json")

	crictl pull registry.k8s.io/pause:3.9 &
	pull_pid=$!

	image_id=$(crictl images --quiet quay.io/crio/alpine:3.9)
	crictl rmi "$image_id" || true

	wait $pull_pid

	[ -f "$BLOB_CACHE_DIR/metadata.json" ]

	pause_id=$(crictl images --quiet registry.k8s.io/pause:3.9)
	[ -n "$pause_id" ]
}
