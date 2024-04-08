#!/usr/bin/env bats

load helpers
CRUN_WASM_BINARY=${CRUN_WASM_BINARY:-$(command -v crun-wasm || true)}

IMAGE=quay.io/crio/pause
SIGNED_IMAGE=registry.access.redhat.com/rhel7-atomic:latest

IMAGE_LIST_TAG=quay.io/crio/alpine:3.9
IMAGE_LIST_DIGEST_FOR_TAG=quay.io/crio/alpine@sha256:414e0518bb9228d35e4cd5165567fb91d26c6a214e9c95899e1e056fcd349011
IMAGE_LIST_DIGEST_FOR_TAG_AMD64=quay.io/crio/alpine@sha256:65b3a80ebe7471beecbc090c5b2cdd0aafeaefa0715f8f12e40dc918a3a70e32

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
	jq '.image.image = "'"$REDIS_IMAGEID"'"' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/ctr.json
	ctr_id=$(crictl create --no-pull "$pod_id" "$TESTDIR"/ctr.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
}

@test "container status when created by image ID" {
	start_crio

	jq '.image.image = "'"$REDIS_IMAGEID"'"' \
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

	jq '.image.image = "'"$IMAGE_LIST_DIGEST"'"' \
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

	FEDORA="registry.fedoraproject.org/fedora"
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

	case $(go env GOARCH) in
	amd64)
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

	case $(go env GOARCH) in
	amd64)
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

	case $(go env GOARCH) in
	amd64)
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

	case $(go env GOARCH) in
	amd64)
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
	cat << EOF > "$CRIO_CONFIG_DIR/99-crun-wasm.conf"
[crio.runtime]
default_runtime = "crun-wasm"

[crio.runtime.runtimes.crun-wasm]
runtime_path = "/usr/bin/crun"

platform_runtime_paths = {"wasi/wasm32" = "/usr/bin/crun-wasm", "abc/def" = "/usr/bin/acme"}
EOF
	start_crio

	jq '.metadata.name = "podsandbox-wasm"
		|.image.image = "quay.io/crio/hello-wasm:latest"
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
