#!/usr/bin/env bats

load helpers
CRUN_WASM_BINARY=${CRUN_WASM_BINARY:-$(command -v crun-wasm || true)}

IMAGE=quay.io/crio/pause
SIGNED_IMAGE=registry.access.redhat.com/rhel7-atomic:latest

IMAGE_LIST_TAG=quay.io/crio/alpine:3.9
IMAGE_LIST_DIGEST_FOR_TAG=quay.io/crio/alpine@sha256:414e0518bb9228d35e4cd5165567fb91d26c6a214e9c95899e1e056fcd349011
IMAGE_LIST_DIGEST_FOR_TAG_AMD64=quay.io/crio/alpine@sha256:65b3a80ebe7471beecbc090c5b2cdd0aafeaefa0715f8f12e40dc918a3a70e32
# Currently unused
# IMAGE_LIST_DIGEST_FOR_TAG_ARM64=quay.io/crio/alpine@sha256:f920ccc826134587fffcf1ddc6b2a554947e0f1a5ae5264bbf3435da5b2e8e61

IMAGE_LIST_DIGEST_AMD64=quay.io/crio/alpine@sha256:65b3a80ebe7471beecbc090c5b2cdd0aafeaefa0715f8f12e40dc918a3a70e32
IMAGE_LIST_DIGEST=quay.io/crio/alpine@sha256:414e0518bb9228d35e4cd5165567fb91d26c6a214e9c95899e1e056fcd349011

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "run container in pod with image ID" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq '.image.image = "'"$REDIS_IMAGEID"'" | .image.user_specified_image = "'"$REDIS_IMAGEDIGEST"'"' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/ctr.json
	ctr_id=$(crictl create --no-pull "$pod_id" "$TESTDIR"/ctr.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
}

@test "container status when created by image ID" {
	start_crio

	jq '.image.image = "'"$REDIS_IMAGEID"'" | .image.user_specified_image = "'"$REDIS_IMAGEDIGEST"'"' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/ctr.json
	ctr_id=$(crictl run --no-pull "$TESTDIR"/ctr.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl inspect -o yaml "$ctr_id")
	[[ "$output" == *"image: quay.io/crio/fedora-crio-ci:latest"* ]]
	[[ "$output" == *"imageRef: $REDIS_IMAGEREF"* ]]
}

@test "container status when created by image tagged reference" {
	start_crio

	jq '.image.image = "quay.io/crio/fedora-crio-ci:latest"' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/ctr.json

	ctr_id=$(crictl run "$TESTDIR"/ctr.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl inspect -o yaml "$ctr_id")
	[[ "$output" == *"image: quay.io/crio/fedora-crio-ci:latest"* ]]
	[[ "$output" == *"imageRef: $REDIS_IMAGEREF"* ]]
}

@test "container status when created by image canonical reference" {
	start_crio

	jq '.image.image = "'"$REDIS_IMAGEREF"'"' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/ctr.json

	ctr_id=$(crictl run "$TESTDIR"/ctr.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl inspect -o yaml "$ctr_id")
	[[ "$output" == *"image: quay.io/crio/fedora-crio-ci:latest"* ]]
	[[ "$output" == *"imageRef: $REDIS_IMAGEREF"* ]]
}

@test "container status when created by image list canonical reference" {
	start_crio

	crictl pull "$IMAGE_LIST_DIGEST"

	jq '.image.image = "'"$IMAGE_LIST_DIGEST"'" | .image.user_specified_image = "'"$IMAGE_LIST_DIGEST"'"' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/ctr.json

	ctr_id=$(crictl run "$TESTDIR"/ctr.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl inspect -o yaml "$ctr_id")
	[[ "$output" == *"image: $IMAGE_LIST_DIGEST"* ]]
	[[ "$output" == *"imageRef: $IMAGE_LIST_DIGEST"* ]]
}

@test "image pull and list" {
	start_crio
	crictl pull "$IMAGE"
	imageid=$(crictl images --quiet "$IMAGE")
	[ "$imageid" != "" ]

	output=$(crictl images "$imageid")
	[[ "$output" == *"$IMAGE"* ]]

	output=$(crictl images --quiet "$imageid")
	[ "$output" != "" ]
	cleanup_images
}

@test "image pull and list using imagestore" {
	# Start crio with imagestore
	mkdir -p "$TESTDIR/imagestore"
	CONTAINER_IMAGESTORE="$TESTDIR/imagestore" start_crio

	# registry.fedoraproject.org is pretty flaky
	# Moving to the stable quay.io
	FEDORA="quay.io/fedora/fedora"
	crictl pull $FEDORA
	imageid=$(crictl images --quiet "$FEDORA")
	[ "$imageid" != "" ]

	output=$(crictl images "$imageid")
	[[ "$output" == *"$FEDORA"* ]]

	output=$(crictl images --quiet "$imageid")
	[ "$output" != "" ]

	stop_crio
	unset CONTAINER_IMAGESTORE
	# start crio without imagestore
	start_crio
	imageid=$(crictl images --quiet "$FEDORA")
	# no image must be found on default root
	[[ "$imageid" == "" ]]
	cleanup_images
}

@test "image pull with signature" {
	skip "registry has some issues"
	start_crio
	crictl pull "$SIGNED_IMAGE"
	cleanup_images
}

@test "image pull and list by tag and ID" {
	start_crio
	crictl pull "$IMAGE:go"

	imageid=$(crictl images --quiet "$IMAGE:go")
	[ "$imageid" != "" ]

	output=$(crictl images --quiet "$imageid")
	[ "$output" != "" ]

	cleanup_images
}

@test "image pull and list by digest and ID" {
	start_crio
	NGINX_IMAGE=quay.io/crio/nginx@sha256:960355a671fb88ef18a85f92ccf2ccf8e12186216c86337ad808c204d69d512d
	crictl pull "$NGINX_IMAGE"

	imageid=$(crictl images --quiet "$NGINX_IMAGE")
	[ "$imageid" != "" ]

	output=$(crictl images --quiet "$imageid")
	[ "$output" != "" ]

	cleanup_images
}

@test "image pull and list by manifest list digest" {
	start_crio

	crictl pull ${IMAGE_LIST_DIGEST}

	imageid=$(crictl images --quiet ${IMAGE_LIST_DIGEST})
	[ "$imageid" != "" ]

	output=$(crictl images -v ${IMAGE_LIST_DIGEST})
	[ "$output" != "" ]
	[[ "$output" == *"RepoDigests: ${IMAGE_LIST_DIGEST}"* ]]

	case $ARCH in
	x86_64)
		output=$(crictl images -v ${IMAGE_LIST_DIGEST_AMD64})
		[ "$output" != "" ]
		[[ "$output" == *"RepoDigests: ${IMAGE_LIST_DIGEST_AMD64}"* ]]
		;;
	esac

	output=$(crictl images --quiet "$imageid")
	[ "$output" != "" ]

	cleanup_images
}

@test "image pull and list by manifest list tag" {
	start_crio

	crictl pull ${IMAGE_LIST_TAG}
	imageid=$(crictl images --quiet ${IMAGE_LIST_TAG})
	[ "$imageid" != "" ]

	output=$(crictl images -v ${IMAGE_LIST_DIGEST_FOR_TAG})
	if [ "$output" == "" ]; then
		echo "NOTE: THIS TEST PROBABLY FAILED BECAUSE DIGEST HAS CHANGED, CONSIDER UPDATING TO MATCH THE FOLLOWING DIGEST:"
		crictl inspecti ${IMAGE_LIST_TAG} | jq .status.repoDigests
		echo "$output"
		exit 1
	fi
	[[ "$output" == *"RepoDigests: ${IMAGE_LIST_DIGEST_FOR_TAG}"* ]]

	case $ARCH in
	x86_64)
		output=$(crictl images -v ${IMAGE_LIST_DIGEST_FOR_TAG_AMD64})
		[[ "$output" == *"RepoDigests: ${IMAGE_LIST_DIGEST_FOR_TAG_AMD64}"* ]]
		;;
	esac

	output=$(crictl images --quiet "$imageid")
	[ "$output" != "" ]

	cleanup_images
}

@test "image pull and list by manifest list and individual digest" {
	start_crio

	crictl pull ${IMAGE_LIST_DIGEST}
	imageid=$(crictl images --quiet ${IMAGE_LIST_DIGEST})
	[ "$imageid" != "" ]

	case $ARCH in
	x86_64)
		crictl pull ${IMAGE_LIST_DIGEST_AMD64}
		output=$(crictl images -v ${IMAGE_LIST_DIGEST_AMD64})
		[[ "$output" == *"RepoDigests: ${IMAGE_LIST_DIGEST_AMD64}"* ]]
		;;
	esac

	output=$(crictl images -v ${IMAGE_LIST_DIGEST})
	[[ "$output" == *"RepoDigests: ${IMAGE_LIST_DIGEST}"* ]]

	output=$(crictl images --quiet "$imageid")
	[ "$output" != "" ]

	cleanup_images
}

@test "image pull and list by individual and manifest list digest" {
	start_crio

	case $ARCH in
	x86_64)
		crictl pull ${IMAGE_LIST_DIGEST_AMD64}
		output=$(crictl images -v ${IMAGE_LIST_DIGEST_AMD64})
		[[ "$output" == *"RepoDigests: ${IMAGE_LIST_DIGEST_AMD64}"* ]]
		;;
	esac

	crictl pull ${IMAGE_LIST_DIGEST}

	imageid=$(crictl images --quiet ${IMAGE_LIST_DIGEST})
	[ "$imageid" != "" ]

	output=$(crictl images -v ${IMAGE_LIST_DIGEST})
	[[ "$output" == *"RepoDigests: ${IMAGE_LIST_DIGEST}"* ]]

	output=$(crictl images --quiet "$imageid")
	[ "$output" != "" ]

	cleanup_images
}

@test "image list with filter" {
	start_crio
	crictl pull "$IMAGE"
	output=$(crictl images --quiet "$IMAGE")
	[ "$output" != "" ]
	for id in $output; do
		crictl rmi "$id"
	done
	crictl images --quiet

	cleanup_images
}

@test "image list/remove" {
	start_crio
	crictl pull "$IMAGE"
	output=$(crictl images --quiet)
	[ "$output" != "" ]
	for id in $output; do
		crictl rmi "$id"
	done
	output=$(crictl images --quiet)
	[ "$output" = "" ]

	cleanup_images
}

@test "image status/remove" {
	start_crio
	crictl pull "$IMAGE"
	output=$(crictl images --quiet)
	[ "$output" != "" ]
	for id in $output; do
		img=$(crictl images -v "$id")
		[ "$img" != "" ]
		crictl rmi "$id"
	done
	output=$(crictl images --quiet)
	[ "$output" = "" ]

	cleanup_images
}

@test "run container in pod with crun-wasm enabled" {
	if [ -z "$CRUN_WASM_BINARY" ] || [[ "$RUNTIME_TYPE" == "vm" ]]; then
		skip "crun-wasm not installed or runtime type is VM"
	fi
	setup_crio

	cat << EOF > "$CRIO_CONFIG_DIR/99-crun-wasm.conf"
[crio.runtime]
default_runtime = "crun-wasm"

[crio.runtime.runtimes.crun-wasm]
runtime_path = "/usr/bin/crun"

platform_runtime_paths = {"wasi/wasm32" = "/usr/bin/crun-wasm", "abc/def" = "/usr/bin/acme"}
EOF
	unset CONTAINER_DEFAULT_RUNTIME
	unset CONTAINER_RUNTIMES

	start_crio_no_setup

	# these two variables are used by this test
	json=$(crictl images -o json)
	eval "$(jq -r '.images[] |
        select(.repoTags[0] == "quay.io/crio/hello-wasm:latest") |
        "WASM_IMAGEID=" + .id + "\n" +
        "WASM_IMAGEDIGEST=" + .repoDigests[0] + "\n" +
	"REDIS_IMAGEREF=" + .repoDigests[0]' <<< "$json")"

	jq '.metadata.name = "podsandbox-wasm"
		| .image.image = "'"$WASM_IMAGEID"'" | .image.user_specified_image = "'"$WASM_IMAGEDIGEST"'"
		| del(.command, .args, .linux.resources)' \
		"$TESTDATA"/container_config.json > "$TESTDIR/wasm.json"

	ctr_id=$(crictl run "$TESTDIR/wasm.json" "$TESTDATA/sandbox_config.json")
	output=$(crictl logs "$ctr_id")
	[[ "$output" == *"Hello, world!"* ]]
}

@test "check if image is pinned appropriately" {
	cat << EOF > "$CRIO_CONFIG_DIR/99-pinned-image.conf"
[crio.image]
pinned_images = [ "quay.io/crio/hello-wasm:latest" ]
EOF
	start_crio
	crictl pull quay.io/crio/hello-wasm:latest
	output=$(crictl images -o json | jq '.images[] | select(.repoTags[] == "quay.io/crio/hello-wasm:latest") | .pinned')
	[ "$output" == "true" ]
}

@test "run container in pod with timezone configured" {
	CONTAINER_TIME_ZONE="Asia/Singapore" start_crio
	jq '.metadata.name = "podsandbox-timezone"
		|.image.image = "quay.io/crio/fedora-crio-ci:latest"
		| del(.command, .args, .linux.resources)' \
		"$TESTDATA"/container_config.json > "$TESTDIR/timezone.json"

	ctr_id=$(crictl run "$TESTDIR/timezone.json" "$TESTDATA/sandbox_config.json")
	datestr=$(date +%s)
	output=$(crictl exec "$ctr_id" date -d "@$datestr" +"%a %b %e %H:%M:%S %Z %Y")
	expected_output=$(TZ="Asia/Singapore" date -d "@$datestr" +"%a %b %e %H:%M:%S %Z %Y")
	[[ "$output" == *"$expected_output"* ]]
}

@test "run container in pod with local timezone" {
	CONTAINER_TIME_ZONE="local" start_crio
	jq '.metadata.name = "podsandbox-empty-timezone"
        | .image.image = "quay.io/crio/fedora-crio-ci:latest"
        | del(.command, .args, .linux.resources)' \
		"$TESTDATA"/container_config.json > "$TESTDIR/empty_timezone.json"

	ctr_id=$(crictl run "$TESTDIR/empty_timezone.json" "$TESTDATA/sandbox_config.json")
	datestr=$(date +%s)
	output=$(crictl exec "$ctr_id" date -d "@$datestr" +"%a %b %e %H:%M:%S %Z %Y")
	expected_output=$(date -d "@$datestr" +"%a %b %e %H:%M:%S %Z %Y")
	[[ "$output" == *"$expected_output"* ]]
}

@test "pull progress timeout should trigger when being set too low" {
	CONTAINER_PULL_PROGRESS_TIMEOUT=1ms start_crio

	run ! crictl pull "$IMAGE_LIST_TAG"
	[[ "$output" == *"context canceled"* ]]
}

@test "pull progress timeout should not timeout when set to 0" {
	CONTAINER_PULL_PROGRESS_TIMEOUT=0 start_crio

	crictl pull "$IMAGE_LIST_TAG"
}

@test "short name mode enabled should fail to pull ambiguous image" {
	start_crio

	# There should be many nginx images
	run crictl pull nginx
	[[ "$output" == *"short name mode is enforcing, but image name nginx returns ambiguous list"* ]]
	[[ "$status" -ne 0 ]]
}

@test "short name mode disabled should succeed to pull ambiguous image" {
	CONTAINER_SHORT_NAME_MODE="disabled" start_crio

	# There should be many nginx images
	crictl pull nginx
}

@test "image status preserves pulled digest order for multi-arch images" {
	start_crio

	# Pull a multi-arch image by its manifest list digest
	crictl pull "$IMAGE_LIST_DIGEST"

	# Get image status as JSON
	output=$(crictl inspecti -o json "$IMAGE_LIST_DIGEST")

	# On amd64, the platform-specific digest should appear FIRST in RepoDigests
	# because it's the one that was actually pulled (stored in knownRepoDigests).
	# The manifest list digest should also be present, but should come AFTER the
	# platform-specific digest.
	case $ARCH in
	x86_64)
		# Verify the first digest is the platform-specific one (what was actually pulled)
		firstDigest=$(jq -r '.status.repoDigests[0]' <<< "$output")
		[[ "$firstDigest" == "$IMAGE_LIST_DIGEST_AMD64" ]]

		# Verify the manifest list digest appears somewhere in the array
		manifestListIndex=$(jq '.status.repoDigests | index("'"$IMAGE_LIST_DIGEST"'")' <<< "$output")
		[[ -n "$manifestListIndex" ]] && [[ "$manifestListIndex" != "null" ]]

		# Verify the manifest list digest comes AFTER the platform-specific digest (index > 0)
		[[ "$manifestListIndex" -gt 0 ]]
		;;
	esac

	cleanup_images
}

@test "image status preserves pulled digest order for single-arch images" {
	start_crio

	# Pull a single-arch image by tag to verify tag-to-digest conversion works
	crictl pull "$IMAGE"

	# Get image status as JSON
	output=$(crictl inspecti -o json "$IMAGE")

	# Extract the first RepoDigest
	firstDigest=$(jq -r '.status.repoDigests[0]' <<< "$output")

	# Verify we got at least one digest
	[[ -n "$firstDigest" ]] && [[ "$firstDigest" != "null" ]]

	# The first digest should reference the correct image repository
	[[ "$firstDigest" == quay.io/crio/pause@sha256:* ]]

	# Verify RepoDigests is not empty and is deterministically ordered
	repoDigestsCount=$(jq '.status.repoDigests | length' <<< "$output")
	[[ "$repoDigestsCount" -gt 0 ]]

	cleanup_images
}
