#!/usr/bin/env bats

load helpers

IMAGE=quay.io/crio/pause
SIGNED_IMAGE=registry.access.redhat.com/rhel7-atomic:latest

IMAGE_LIST_TAG=docker.io/library/alpine:3.9
IMAGE_LIST_DIGEST_FOR_TAG=docker.io/library/alpine@sha256:414e0518bb9228d35e4cd5165567fb91d26c6a214e9c95899e1e056fcd349011
IMAGE_LIST_DIGEST_FOR_TAG_AMD64=docker.io/library/alpine@sha256:65b3a80ebe7471beecbc090c5b2cdd0aafeaefa0715f8f12e40dc918a3a70e32

IMAGE_LIST_DIGEST_AMD64=docker.io/library/alpine@sha256:ab3fe83c0696e3f565c9b4a734ec309ae9bd0d74c192de4590fd6dc2ef717815
IMAGE_LIST_DIGEST=docker.io/library/alpine@sha256:ab3fe83c0696e3f565c9b4a734ec309ae9bd0d74c192de4590fd6dc2ef717815

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

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '.image.image = "'"$REDIS_IMAGEID"'"' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/ctr.json
	ctr_id=$(crictl create --no-pull "$pod_id" "$TESTDIR"/ctr.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl inspect -o yaml "$ctr_id")
	[[ "$output" == *"image: quay.io/crio/redis:alpine"* ]]
	[[ "$output" == *"imageRef: $REDIS_IMAGEREF"* ]]
}

@test "container status when created by image tagged reference" {
	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '.image.image = "quay.io/crio/redis:alpine"' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/ctr.json

	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/ctr.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl inspect -o yaml "$ctr_id")
	[[ "$output" == *"image: quay.io/crio/redis:alpine"* ]]
	[[ "$output" == *"imageRef: $REDIS_IMAGEREF"* ]]
}

@test "container status when created by image canonical reference" {
	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '.image.image = "'"$REDIS_IMAGEREF"'"' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/ctr.json

	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/ctr.json "$TESTDATA"/sandbox_config.json)

	crictl start "$ctr_id"
	output=$(crictl inspect -o yaml "$ctr_id")
	[[ "$output" == *"image: quay.io/crio/redis:alpine"* ]]
	[[ "$output" == *"imageRef: $REDIS_IMAGEREF"* ]]
}

@test "container status when created by image list canonical reference" {
	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	crictl pull "$IMAGE_LIST_DIGEST"

	jq '.image.image = "'"$IMAGE_LIST_DIGEST"'"' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/ctr.json

	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/ctr.json "$TESTDATA"/sandbox_config.json)

	crictl start "$ctr_id"
	output=$(crictl inspect -o yaml "$ctr_id")
	[[ "$output" == *"image: $IMAGE_LIST_DIGEST"* ]]
	[[ "$output" == *"imageRef: $IMAGE_LIST_DIGEST"* ]]
}

@test "image pull and list" {
	start_crio
	crictl pull "$IMAGE"
	imageid=$(crictl images --quiet "$IMAGE")
	[ "$imageid" != "" ]

	output=$(crictl images @"$imageid")
	[[ "$output" == *"$IMAGE"* ]]

	output=$(crictl images --quiet "$imageid")
	[ "$output" != "" ]
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

	output=$(crictl images --quiet @"$imageid")
	[ "$output" != "" ]

	output=$(crictl images --quiet "$imageid")
	[ "$output" != "" ]

	cleanup_images
}

@test "image pull and list by digest and ID" {
	start_crio
	crictl pull quay.io/crio/nginx@sha256:1ad874092a55efe2be0507a01d8a300e286f8137510854606ab1dd28861507a3

	imageid=$(crictl images --quiet quay.io/crio/nginx@sha256:1ad874092a55efe2be0507a01d8a300e286f8137510854606ab1dd28861507a3)
	[ "$imageid" != "" ]
	output=$(crictl images --quiet @"$imageid")
	[ "$output" != "" ]

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

	output=$(crictl images --quiet @"$imageid")
	[ "$output" != "" ]

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

	output=$(crictl images --quiet @"$imageid")
	[ "$output" != "" ]

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

	output=$(crictl images --quiet @"$imageid")
	[ "$output" != "" ]

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

	output=$(crictl images --quiet @"$imageid")
	[ "$output" != "" ]

	output=$(crictl images --quiet "$imageid")
	[ "$output" != "" ]

	cleanup_images
}

@test "image list with filter" {
	start_crio
	crictl pull "$IMAGE"
	output=$(crictl images --quiet "$IMAGE")
	printf '%s\n' "$output" | while IFS= read -r id; do
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
	printf '%s\n' "$output" | while IFS= read -r id; do
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
	printf '%s\n' "$output" | while IFS= read -r id; do
		output=$(crictl images -v "$id")
		[ "$output" != "" ]
		crictl rmi "$id"
	done
	output=$(crictl images --quiet)
	[ "$output" = "" ]

	cleanup_images
}
