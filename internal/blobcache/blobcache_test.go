package blobcache_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	digest "github.com/opencontainers/go-digest"

	"github.com/cri-o/cri-o/internal/blobcache"
)

var _ = t.Describe("BlobCache", func() {
	var (
		ctx    context.Context
		tmpDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		tmpDir = GinkgoT().TempDir()
	})

	t.Describe("New", func() {
		It("should create a valid cache", func() {
			cache, err := blobcache.New(ctx, tmpDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(cache).ToNot(BeNil())
			Expect(filepath.Join(tmpDir, "blobs")).To(BeADirectory())
		})

		It("should fail with empty path", func() {
			_, err := blobcache.New(ctx, "")
			Expect(err).To(MatchError(blobcache.ErrEmptyDirectory))
		})

		It("should fail with relative path", func() {
			_, err := blobcache.New(ctx, "relative/path")
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("StoreBlob", func() {
		var cache *blobcache.BlobCache

		BeforeEach(func() {
			var err error
			cache, err = blobcache.New(ctx, tmpDir)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should store a blob successfully", func() {
			content := []byte("test blob content")
			hash := sha256.Sum256(content)
			dgst := digest.NewDigestFromEncoded(digest.SHA256, hex.EncodeToString(hash[:]))

			err := cache.StoreBlob(ctx, dgst, bytes.NewReader(content), "docker.io", "library/test")
			Expect(err).ToNot(HaveOccurred())

			blobPath := filepath.Join(tmpDir, "blobs", "sha256", dgst.Encoded())
			Expect(blobPath).To(BeAnExistingFile())
		})

		It("should add source on duplicate store", func() {
			content := []byte("test blob content")
			hash := sha256.Sum256(content)
			dgst := digest.NewDigestFromEncoded(digest.SHA256, hex.EncodeToString(hash[:]))

			err := cache.StoreBlob(ctx, dgst, bytes.NewReader(content), "docker.io", "library/test")
			Expect(err).ToNot(HaveOccurred())

			err = cache.StoreBlob(ctx, dgst, bytes.NewReader(content), "quay.io", "other/test")
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail on digest mismatch", func() {
			wrongContent := []byte("wrong content")
			wrongHash := sha256.Sum256([]byte("actual content"))
			wrongDgst := digest.NewDigestFromEncoded(digest.SHA256, hex.EncodeToString(wrongHash[:]))

			err := cache.StoreBlob(ctx, wrongDgst, bytes.NewReader(wrongContent), "docker.io", "library/fail")
			Expect(err).To(HaveOccurred())
		})

		It("should be idempotent for same source", func() {
			content := []byte("test blob content")
			hash := sha256.Sum256(content)
			dgst := digest.NewDigestFromEncoded(digest.SHA256, hex.EncodeToString(hash[:]))

			for range 3 {
				err := cache.StoreBlob(ctx, dgst, bytes.NewReader(content), "docker.io", "library/test")
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should handle concurrent stores", func() {
			content := []byte("concurrent blob")
			hash := sha256.Sum256(content)
			dgst := digest.NewDigestFromEncoded(digest.SHA256, hex.EncodeToString(hash[:]))

			const numGoroutines = 10
			var wg sync.WaitGroup
			errChan := make(chan error, numGoroutines)

			for range numGoroutines {
				wg.Go(func() {
					storeErr := cache.StoreBlob(ctx, dgst, bytes.NewReader(content), "docker.io", "library/test")
					errChan <- storeErr
				})
			}

			wg.Wait()
			close(errChan)

			for storeErr := range errChan {
				Expect(storeErr).ToNot(HaveOccurred())
			}

			blobPath := filepath.Join(tmpDir, "blobs", "sha256", dgst.Encoded())
			Expect(blobPath).To(BeAnExistingFile())
		})

		It("should create algorithm directory on-demand", func() {
			content := []byte("test blob content")
			hash := sha256.Sum256(content)
			dgst := digest.NewDigestFromEncoded(digest.SHA256, hex.EncodeToString(hash[:]))

			err := cache.StoreBlob(ctx, dgst, bytes.NewReader(content), "docker.io", "library/test")
			Expect(err).ToNot(HaveOccurred())

			Expect(filepath.Join(tmpDir, "blobs", "sha256")).To(BeADirectory())
			Expect(filepath.Join(tmpDir, "blobs", "sha256", dgst.Encoded())).To(BeAnExistingFile())
		})
	})

	t.Describe("Metadata reconstruction", func() {
		It("should reconstruct metadata from blob files", func() {
			cache, err := blobcache.New(ctx, tmpDir)
			Expect(err).ToNot(HaveOccurred())

			content := []byte("test blob content")
			hash := sha256.Sum256(content)
			dgst := digest.NewDigestFromEncoded(digest.SHA256, hex.EncodeToString(hash[:]))

			err = cache.StoreBlob(ctx, dgst, bytes.NewReader(content), "docker.io", "library/test")
			Expect(err).ToNot(HaveOccurred())

			// Remove metadata file (simulate corruption)
			err = os.Remove(filepath.Join(tmpDir, "metadata.json"))
			Expect(err).ToNot(HaveOccurred())

			// Reload cache
			cache2, err := blobcache.New(ctx, tmpDir)
			Expect(err).ToNot(HaveOccurred())

			// Store same blob again - should reconstruct metadata from file
			err = cache2.StoreBlob(ctx, dgst, bytes.NewReader(content), "quay.io", "other/test")
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
