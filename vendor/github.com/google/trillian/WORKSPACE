load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "bazel_skylib",
    sha256 = "bbccf674aa441c266df9894182d80de104cabd19be98be002f6d478aaa31574d",
    strip_prefix = "bazel-skylib-2169ae1c374aab4a09aa90e65efe1a3aad4e279b",
    urls = ["https://github.com/bazelbuild/bazel-skylib/archive/2169ae1c374aab4a09aa90e65efe1a3aad4e279b.tar.gz"],
)

load("@bazel_skylib//lib:versions.bzl", "versions")

versions.check(minimum_bazel_version = "0.5.4")

# This com_google_protobuf repository is required for proto_library rule.
# It provides the protocol compiler binary (i.e., protoc).
http_archive(
    name = "com_google_protobuf",
    strip_prefix = "protobuf-3.9.0",
    urls = ["https://github.com/protocolbuffers/protobuf/archive/v3.9.0.zip"],
)

load("@com_google_protobuf//:protobuf_deps.bzl", "protobuf_deps")

protobuf_deps()

http_archive(
    name = "com_google_googleapis",
    strip_prefix = "googleapis-master",
    url = "https://github.com/googleapis/googleapis/archive/master.zip",
)

load("@com_google_googleapis//:repository_rules.bzl", "switched_rules_by_language")

switched_rules_by_language(
    name = "com_google_googleapis_imports",
    go = True,
    java = True,
    python = False, # Broken by https://github.com/googleapis/googleapis/commit/248abde0
)

http_archive(
    name = "io_bazel_rules_go",
    strip_prefix = "rules_go-7d17d496a6b32f6a573c6c22e29c58204eddf3d4",
    urls = ["https://github.com/bazelbuild/rules_go/archive/7d17d496a6b32f6a573c6c22e29c58204eddf3d4.zip"],
)
