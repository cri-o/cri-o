package storage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	digest "github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/types"

	"github.com/cri-o/cri-o/internal/blobcache"
	"github.com/cri-o/cri-o/internal/storage"
)

// mockDestination implements types.ImageDestination for testing.
type mockDestination struct {
	types.ImageDestination

	putBlobFunc func(stream io.Reader, blobInfo types.BlobInfo) (types.BlobInfo, error)
}

//nolint:gocritic // hugeParam: blobInfo signature required by types.ImageDestination interface
func (m *mockDestination) PutBlob(ctx context.Context, stream io.Reader, blobInfo types.BlobInfo, cache types.BlobInfoCache, isConfig bool) (types.BlobInfo, error) {
	if m.putBlobFunc != nil {
		return m.putBlobFunc(stream, blobInfo)
	}

	// Consume stream by default.
	if _, err := io.Copy(io.Discard, stream); err != nil {
		return types.BlobInfo{}, err
	}

	return blobInfo, nil
}

// blobExists checks if a blob file exists in the cache directory.
func blobExists(cacheDir string, dgst digest.Digest) bool {
	blobPath := filepath.Join(cacheDir, "blobs", dgst.Algorithm().String(), dgst.Encoded())
	_, err := os.Stat(blobPath)

	return err == nil
}

var _ = t.Describe("BlobCacheWrapper", func() {
	t.Describe("parseRegistryAndRepository", func() {
		DescribeTable("should parse image references correctly",
			func(input, expectedRegistry, expectedRepository string, expectError bool) {
				reg, repo, err := storage.ParseRegistryAndRepository(input)
				if expectError {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).ToNot(HaveOccurred())
					Expect(reg).To(Equal(expectedRegistry))
					Expect(repo).To(Equal(expectedRepository))
				}
			},
			Entry("docker.io with library", "docker.io/library/nginx:latest", "docker.io", "library/nginx", false),
			Entry("quay.io", "quay.io/prometheus/prometheus:v2.45.0", "quay.io", "prometheus/prometheus", false),
			Entry("registry.k8s.io", "registry.k8s.io/pause:3.9", "registry.k8s.io", "pause", false),
			Entry("short name", "nginx", "docker.io", "library/nginx", false),
			Entry("localhost with port", "localhost:5000/myimage", "localhost:5000", "myimage", false),
			Entry("invalid reference", "invalid reference", "", "", true),
		)
	})

	t.Describe("PutBlob", func() {
		var (
			ctx    context.Context
			tmpDir string
			cache  *blobcache.BlobCache
			dest   *mockDestination
		)

		BeforeEach(func() {
			ctx = context.Background()
			tmpDir = GinkgoT().TempDir()

			var err error
			cache, err = blobcache.New(ctx, tmpDir)
			Expect(err).ToNot(HaveOccurred())

			dest = &mockDestination{}
		})

		It("should cache blob successfully", func() {
			wrapper := storage.NewBlobCachingDestination(dest, cache, "docker.io", "library/test")

			content := "test blob content"
			hash := sha256.Sum256([]byte(content))
			digestStr := "sha256:" + hex.EncodeToString(hash[:])

			dest.putBlobFunc = func(stream io.Reader, blobInfo types.BlobInfo) (types.BlobInfo, error) {
				data, err := io.ReadAll(stream)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(data)).To(Equal(content))

				return blobInfo, nil
			}

			_, err := wrapper.PutBlob(ctx, strings.NewReader(content), types.BlobInfo{Digest: digest.Digest(digestStr)}, nil, false)
			Expect(err).ToNot(HaveOccurred())

			// Verify it's in cache.
			Eventually(func() bool {
				return blobExists(tmpDir, digest.Digest(digestStr))
			}).Should(BeTrue())
		})

		It("should pass through config blobs without caching", func() {
			wrapper := storage.NewBlobCachingDestination(dest, cache, "docker.io", "library/test")

			content := "config content"
			hash := sha256.Sum256([]byte(content))
			digestStr := "sha256:" + hex.EncodeToString(hash[:])

			dest.putBlobFunc = func(stream io.Reader, blobInfo types.BlobInfo) (types.BlobInfo, error) {
				_, err := io.Copy(io.Discard, stream)

				return blobInfo, err
			}

			_, err := wrapper.PutBlob(ctx, strings.NewReader(content), types.BlobInfo{Digest: digest.Digest(digestStr)}, nil, true)
			Expect(err).ToNot(HaveOccurred())

			// Config blob should NOT be cached.
			Expect(blobExists(tmpDir, digest.Digest(digestStr))).To(BeFalse())
		})

		It("should pass through blobs without digest", func() {
			wrapper := storage.NewBlobCachingDestination(dest, cache, "docker.io", "library/test")

			content := "no digest content"

			dest.putBlobFunc = func(stream io.Reader, blobInfo types.BlobInfo) (types.BlobInfo, error) {
				_, err := io.Copy(io.Discard, stream)

				return blobInfo, err
			}

			_, err := wrapper.PutBlob(ctx, strings.NewReader(content), types.BlobInfo{}, nil, false)
			Expect(err).ToNot(HaveOccurred())

			// Verify cache blobs directory is empty.
			blobsDir := filepath.Join(tmpDir, "blobs")
			entries, err := os.ReadDir(blobsDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(BeEmpty())
		})
	})

	t.Describe("newBlobCachingReference", func() {
		It("should return nil when cache is nil", func() {
			ref := storage.NewBlobCachingReference(nil, nil, "docker.io", "library/test")
			Expect(ref).To(BeNil())
		})
	})

	t.Describe("TeeReader behavior", func() {
		var (
			ctx    context.Context
			tmpDir string
			cache  *blobcache.BlobCache
			dest   *mockDestination
		)

		BeforeEach(func() {
			ctx = context.Background()
			tmpDir = GinkgoT().TempDir()

			var err error
			cache, err = blobcache.New(ctx, tmpDir)
			Expect(err).ToNot(HaveOccurred())

			dest = &mockDestination{}
		})

		It("should succeed storage write even if cache becomes read-only", func() {
			wrapper := storage.NewBlobCachingDestination(dest, cache, "docker.io", "library/test")

			content := "test blob for readonly cache"
			hash := sha256.Sum256([]byte(content))
			digestStr := "sha256:" + hex.EncodeToString(hash[:])

			blobsDir := filepath.Join(tmpDir, "blobs")
			Expect(os.Chmod(blobsDir, 0o000)).To(Succeed())
			DeferCleanup(os.Chmod, blobsDir, os.FileMode(0o755))

			var storedContent string
			dest.putBlobFunc = func(stream io.Reader, blobInfo types.BlobInfo) (types.BlobInfo, error) {
				data, err := io.ReadAll(stream)
				if err != nil {
					return types.BlobInfo{}, err
				}
				storedContent = string(data)

				return blobInfo, nil
			}

			_, err := wrapper.PutBlob(ctx, strings.NewReader(content), types.BlobInfo{Digest: digest.Digest(digestStr)}, nil, false)
			Expect(err).ToNot(HaveOccurred())
			Expect(storedContent).To(Equal(content))
		})

		It("should deliver identical bytes to storage and cache", func() {
			wrapper := storage.NewBlobCachingDestination(dest, cache, "docker.io", "library/test")

			content := "identical bytes test content"
			hash := sha256.Sum256([]byte(content))
			dgst := digest.NewDigestFromEncoded(digest.SHA256, hex.EncodeToString(hash[:]))

			var storedContent string
			dest.putBlobFunc = func(stream io.Reader, blobInfo types.BlobInfo) (types.BlobInfo, error) {
				data, err := io.ReadAll(stream)
				if err != nil {
					return types.BlobInfo{}, err
				}
				storedContent = string(data)

				return blobInfo, nil
			}

			_, err := wrapper.PutBlob(ctx, strings.NewReader(content), types.BlobInfo{Digest: dgst}, nil, false)
			Expect(err).ToNot(HaveOccurred())

			Expect(storedContent).To(Equal(content))

			blobPath := filepath.Join(tmpDir, "blobs", "sha256", dgst.Encoded())
			cachedContent, err := os.ReadFile(blobPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(cachedContent)).To(Equal(content))
		})
	})
})
