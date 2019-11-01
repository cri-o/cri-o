#!/usr/bin/env bats

load helpers

IMAGE=quay.io/crio/pause
SIGNED_IMAGE=registry.access.redhat.com/rhel7-atomic:latest
UNSIGNED_IMAGE=quay.io/crio/hello-world:latest
IMAGE_LIST_TAG=docker.io/library/alpine:3.9
IMAGE_LIST_DIGEST=docker.io/library/alpine@sha256:7746df395af22f04212cd25a92c1d6dbc5a06a0ca9579a229ef43008d4d1302a
IMAGE_LIST_DIGEST_AMD64=docker.io/library/alpine@sha256:bf1684a6e3676389ec861c602e97f27b03f14178e5bc3f70dce198f9f160cce9
IMAGE_LIST_DIGEST_ARM64=docker.io/library/alpine@sha256:1032bdba4c5f88facf7eceb259c18deb28a51785eb35e469285a03eba78dd3fc
IMAGE_LIST_DIGEST_PPC64LE=docker.io/library/alpine@sha256:cb238aa5b34dfd5e57ddfb1bfbb564f01df218e6f6453e4036b302e32bca8bb5
IMAGE_LIST_DIGEST_S390X=docker.io/library/alpine@sha256:d438d3b6a72b602b70bd259ebfb344e388d8809c5abf691f6de397de8c9e4572

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
	run crictl create "$pod_id" "$TESTDIR"/ctr_by_imageid.json "$TESTDATA"/sandbox_config.json
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

	run crictl create "$pod_id" "$TESTDIR"/ctr_by_imageid.json "$TESTDATA"/sandbox_config.json
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

@test "image pull and list" {
	start_crio "" "" --no-pause-image
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
	start_crio "" "" --no-pause-image
	run crictl pull "$SIGNED_IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_images
}

@test "image pull without signature" {
	start_crio "" "" --no-pause-image
	run crictl image pull "$UNSIGNED_IMAGE"
	echo "$output"
	[ "$status" -ne 0 ]
	cleanup_images
}

@test "image pull and list by tag and ID" {
	start_crio "" "" --no-pause-image
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
	start_crio "" "" --no-pause-image
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
	start_crio "" "" --no-pause-image

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

@test "image pull and list by manifest list tag" {
	start_crio "" "" --no-pause-image

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
	start_crio "" "" --no-pause-image

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
	start_crio "" "" --no-pause-image

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
	start_crio "" "" --no-pause-image
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
	start_crio "" "" --no-pause-image
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
	start_crio "" "" --no-pause-image
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
