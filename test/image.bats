#!/usr/bin/env bats

load helpers

IMAGE=quay.io/crio/pause
SIGNED_IMAGE=registry.access.redhat.com/rhel7-atomic:latest
UNSIGNED_IMAGE=quay.io/crio/hello-world:latest
IMAGE_LIST_TAG=docker.io/library/alpine:3.9
IMAGE_LIST_DIGEST_AMD64=docker.io/library/alpine@sha256:ab3fe83c0696e3f565c9b4a734ec309ae9bd0d74c192de4590fd6dc2ef717815
IMAGE_LIST_DIGEST=docker.io/library/alpine@sha256:115731bab0862031b44766733890091c17924f9b7781b79997f5f163be262178

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "run container in pod with image ID" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	sed -e "s/%VALUE%/$REDIS_IMAGEID/g" "$TESTDATA"/container_config_by_imageid.json > "$TESTDIR"/ctr_by_imageid.json
	run crictl create --no-pull "$pod_id" "$TESTDIR"/ctr_by_imageid.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "container status when created by image ID" {
	start_crio

	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	sed -e "s/%VALUE%/$REDIS_IMAGEID/g" "$TESTDATA"/container_config_by_imageid.json > "$TESTDIR"/ctr_by_imageid.json

	run crictl create --no-pull "$pod_id" "$TESTDIR"/ctr_by_imageid.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl inspect "$ctr_id" --output yaml
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "image: quay.io/crio/redis:alpine" ]]
	[[ "$output" =~ "imageRef: $REDIS_IMAGEREF" ]]
}

@test "container status when created by image tagged reference" {
	start_crio

	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	sed -e "s/%VALUE%/quay.io\/crio\/redis:alpine/g" "$TESTDATA"/container_config_by_imageid.json > "$TESTDIR"/ctr_by_imagetag.json

	run crictl create "$pod_id" "$TESTDIR"/ctr_by_imagetag.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl inspect "$ctr_id" --output yaml
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "image: quay.io/crio/redis:alpine" ]]
	[[ "$output" =~ "imageRef: $REDIS_IMAGEREF" ]]
}

@test "container status when created by image canonical reference" {
	start_crio

	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	sed -e "s|%VALUE%|$REDIS_IMAGEREF|g" "$TESTDATA"/container_config_by_imageid.json > "$TESTDIR"/ctr_by_imageref.json

	run crictl create "$pod_id" "$TESTDIR"/ctr_by_imageref.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl inspect "$ctr_id" --output yaml
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "image: quay.io/crio/redis:alpine" ]]
	[[ "$output" =~ "imageRef: $REDIS_IMAGEREF" ]]
}

@test "container status when created by image list canonical reference" {
	start_crio

	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	sed -e "s|%VALUE%|$IMAGE_LIST_DIGEST|g" -e 's|"/bin/ls"|"/bin/sleep", "1d"|g' "$TESTDATA"/container_config_by_imageid.json > "$TESTDIR"/ctr_by_imagelistref.json

	run crictl create "$pod_id" "$TESTDIR"/ctr_by_imagelistref.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl inspect "$ctr_id" --output yaml
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "image: $IMAGE_LIST_DIGEST" ]]
	[[ "$output" =~ "imageRef: $IMAGE_LIST_DIGEST" ]]
}

@test "image pull and list" {
	start_crio "" ""
	run crictl pull "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl images --quiet "$IMAGE"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]
	imageid="$output"

	run crictl images @"$imageid"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "$IMAGE" ]]

	run crictl images --quiet "$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]
	cleanup_images
}

@test "image pull with signature" {
	skip "registry has some issues"
	start_crio "" ""
	run crictl pull "$SIGNED_IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_images
}

@test "image pull and list by tag and ID" {
	start_crio "" ""
	run crictl pull "$IMAGE:go"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl images --quiet "$IMAGE:go"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]
	imageid="$output"

	run crictl images --quiet @"$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	run crictl images --quiet "$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	cleanup_images
}

@test "image pull and list by digest and ID" {
	start_crio "" ""
	run crictl pull quay.io/crio/nginx@sha256:1ad874092a55efe2be0507a01d8a300e286f8137510854606ab1dd28861507a3
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl images --quiet quay.io/crio/nginx@sha256:1ad874092a55efe2be0507a01d8a300e286f8137510854606ab1dd28861507a3
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]
	imageid="$output"

	run crictl images --quiet @"$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	run crictl images --quiet "$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	cleanup_images
}

@test "image pull and list by manifest list digest" {
	start_crio "" ""

	run crictl pull ${IMAGE_LIST_DIGEST}
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl images --quiet ${IMAGE_LIST_DIGEST}
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]
	imageid="$output"

	run crictl images -v ${IMAGE_LIST_DIGEST}
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]
	[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST}" ]]

	case $(go env GOARCH) in
	amd64)
		run crictl images -v ${IMAGE_LIST_DIGEST_AMD64}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_AMD64}" ]]
		;;
	esac

	run crictl images --quiet @"$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	run crictl images --quiet "$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	cleanup_images
	stop_crio
}

@test "image pull and list by manifest list tag" {
	start_crio "" ""

	run crictl pull ${IMAGE_LIST_TAG}
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl images --quiet ${IMAGE_LIST_TAG}
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]
	imageid="$output"

	run crictl images -v ${IMAGE_LIST_DIGEST}
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]
	[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST}" ]]

	case $(go env GOARCH) in
	amd64)
		run crictl images -v ${IMAGE_LIST_DIGEST_AMD64}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_AMD64}" ]]
		;;
	arm64)
		run crictl images -v ${IMAGE_LIST_DIGEST_ARM64}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_ARM64}" ]]
		;;
	ppc64le)
		run crictl images -v ${IMAGE_LIST_DIGEST_PPC64LE}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_PPC64LE}" ]]
		;;
	s390x)
		run crictl images -v ${IMAGE_LIST_DIGEST_S390X}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_S390X}" ]]
		;;
	esac

	run crictl images --quiet @"$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	run crictl images --quiet "$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	cleanup_images
	stop_crio
}

@test "image pull and list by manifest list and individual digest" {
	start_crio "" ""

	run crictl pull ${IMAGE_LIST_DIGEST}
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl images --quiet ${IMAGE_LIST_DIGEST}
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]
	imageid="$output"

	case $(go env GOARCH) in
	amd64)
		run crictl pull ${IMAGE_LIST_DIGEST_AMD64}
		run crictl images -v ${IMAGE_LIST_DIGEST_AMD64}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_AMD64}" ]]
		;;
	arm64)
		run crictl pull ${IMAGE_LIST_DIGEST_ARM64}
		run crictl images -v ${IMAGE_LIST_DIGEST_ARM64}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_ARM64}" ]]
		;;
	ppc64le)
		run crictl pull ${IMAGE_LIST_DIGEST_PPC64LE}
		run crictl images -v ${IMAGE_LIST_DIGEST_PPC64LE}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_PPC64LE}" ]]
		;;
	s390x)
		run crictl pull ${IMAGE_LIST_DIGEST_S390X}
		run crictl images -v ${IMAGE_LIST_DIGEST_S390X}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_S390X}" ]]
		;;
	esac

	run crictl images -v ${IMAGE_LIST_DIGEST}
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]
	[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST}" ]]

	run crictl images --quiet @"$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	run crictl images --quiet "$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	cleanup_images
	stop_crio
}

@test "image pull and list by individual and manifest list digest" {
	start_crio "" ""

	case $(go env GOARCH) in
	amd64)
		run crictl pull ${IMAGE_LIST_DIGEST_AMD64}
		run crictl images -v ${IMAGE_LIST_DIGEST_AMD64}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_AMD64}" ]]
		;;
	arm64)
		run crictl pull ${IMAGE_LIST_DIGEST_ARM64}
		run crictl images -v ${IMAGE_LIST_DIGEST_ARM64}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_ARM64}" ]]
		;;
	ppc64le)
		run crictl pull ${IMAGE_LIST_DIGEST_PPC64LE}
		run crictl images -v ${IMAGE_LIST_DIGEST_PPC64LE}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_PPC64LE}" ]]
		;;
	s390x)
		run crictl pull ${IMAGE_LIST_DIGEST_S390X}
		run crictl images -v ${IMAGE_LIST_DIGEST_S390X}
		[ "$status" -eq 0 ]
		echo "$output"
		[ "$output" != "" ]
		[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST_S390X}" ]]
		;;
	esac

	run crictl pull ${IMAGE_LIST_DIGEST}
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl images --quiet ${IMAGE_LIST_DIGEST}
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]
	imageid="$output"

	run crictl images -v ${IMAGE_LIST_DIGEST}
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]
	[[ "$output" =~ "RepoDigests: ${IMAGE_LIST_DIGEST}" ]]

	run crictl images --quiet @"$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	run crictl images --quiet "$imageid"
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	cleanup_images
	stop_crio
}

@test "image list with filter" {
	start_crio "" ""
	run crictl pull "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl images --quiet "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		run crictl rmi "$id"
		echo "$output"
		[ "$status" -eq 0 ]
	done
	run crictl images --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		echo "$id"
		status=1
	done
	cleanup_images
}

@test "image list/remove" {
	start_crio "" ""
	run crictl pull "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl images --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" != "" ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		run crictl rmi "$id"
		echo "$output"
		[ "$status" -eq 0 ]
	done
	run crictl images --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" = "" ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		echo "$id"
		status=1
	done
	cleanup_images
}

@test "image status/remove" {
	start_crio "" ""
	run crictl pull "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl images --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" != "" ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		run crictl images -v "$id"
		echo "$output"
		[ "$status" -eq 0 ]
		[ "$output" != "" ]
		run crictl rmi "$id"
		echo "$output"
		[ "$status" -eq 0 ]
	done
	run crictl images --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" = "" ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		echo "$id"
		status=1
	done
	cleanup_images
}
