package supplychain_test

import (
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/supplychain"
)

var _ = Describe("Cache", func() {
	var cache *supplychain.Cache

	newAllowed := func() *supplychain.Result {
		return &supplychain.Result{Allowed: true, Reason: "test pass"}
	}

	newDenied := func() *supplychain.Result {
		return &supplychain.Result{Allowed: false, Reason: "test fail"}
	}

	Describe("with a positive TTL", func() {
		BeforeEach(func() {
			cache = supplychain.NewCache(1 * time.Hour)
		})

		It("should return nil for a cache miss", func() {
			Expect(cache.Get("sha256:abc", "default")).To(BeNil())
		})

		It("should return a cached result after Set", func() {
			cache.Set("sha256:abc", "default", newAllowed())
			result := cache.Get("sha256:abc", "default")
			Expect(result).ToNot(BeNil())
			Expect(result.Allowed).To(BeTrue())
			Expect(result.Reason).To(Equal("test pass"))
		})

		It("should distinguish entries by digest", func() {
			cache.Set("sha256:aaa", "default", newAllowed())
			cache.Set("sha256:bbb", "default", newDenied())

			Expect(cache.Get("sha256:aaa", "default").Allowed).To(BeTrue())
			Expect(cache.Get("sha256:bbb", "default").Allowed).To(BeFalse())
		})

		It("should distinguish entries by namespace", func() {
			cache.Set("sha256:abc", "ns1", newAllowed())
			cache.Set("sha256:abc", "ns2", newDenied())

			Expect(cache.Get("sha256:abc", "ns1").Allowed).To(BeTrue())
			Expect(cache.Get("sha256:abc", "ns2").Allowed).To(BeFalse())
		})

		It("should return nil for a different namespace", func() {
			cache.Set("sha256:abc", "ns1", newAllowed())
			Expect(cache.Get("sha256:abc", "ns2")).To(BeNil())
		})

		It("should clear all entries", func() {
			cache.Set("sha256:aaa", "default", newAllowed())
			cache.Set("sha256:bbb", "other", newDenied())
			cache.Clear()
			Expect(cache.Get("sha256:aaa", "default")).To(BeNil())
			Expect(cache.Get("sha256:bbb", "other")).To(BeNil())
		})

		It("should overwrite existing entries", func() {
			cache.Set("sha256:abc", "default", newAllowed())
			cache.Set("sha256:abc", "default", newDenied())
			result := cache.Get("sha256:abc", "default")
			Expect(result).ToNot(BeNil())
			Expect(result.Allowed).To(BeFalse())
		})
	})

	Describe("with expired entries", func() {
		It("should return nil and evict expired entries", func() {
			cache = supplychain.NewCache(1 * time.Nanosecond)
			cache.Set("sha256:abc", "default", newAllowed())
			time.Sleep(2 * time.Millisecond)
			Expect(cache.Get("sha256:abc", "default")).To(BeNil())
		})
	})

	Describe("with size limit", func() {
		It("should stop accepting new entries when at capacity", func() {
			cache = supplychain.NewCache(1 * time.Hour)

			// Fill the cache to capacity.
			for i := range 10000 {
				cache.Set(fmt.Sprintf("sha256:%d", i), "default", newAllowed())
			}

			// New entry beyond capacity should be silently dropped.
			cache.Set("sha256:overflow", "default", newAllowed())
			Expect(cache.Get("sha256:overflow", "default")).To(BeNil())
		})
	})

	Describe("with zero TTL", func() {
		It("should not store entries", func() {
			cache = supplychain.NewCache(0)
			cache.Set("sha256:abc", "default", newAllowed())
			Expect(cache.Get("sha256:abc", "default")).To(BeNil())
		})
	})

	Describe("concurrent access", func() {
		It("should handle concurrent reads and writes safely", func() {
			cache = supplychain.NewCache(1 * time.Hour)

			var wg sync.WaitGroup

			for range 100 {
				wg.Go(func() {
					cache.Set("sha256:abc", "default", newAllowed())
				})
				wg.Go(func() {
					cache.Get("sha256:abc", "default")
				})
			}

			wg.Go(func() {
				cache.Clear()
			})

			wg.Wait()
		})
	})
})
