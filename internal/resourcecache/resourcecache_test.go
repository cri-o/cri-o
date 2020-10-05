package resourcecache_test

import (
	"time"

	"github.com/cri-o/cri-o/internal/resourcecache"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	testName = "name"
	testID   = "id"
)

type entry struct {
	id      string
	created bool
}

func (e *entry) ID() string {
	return e.id
}

func (e *entry) SetCreated() {
	e.created = true
}

// The actual test suite
var _ = t.Describe("Container", func() {
	// Setup the test
	var (
		sut          *resourcecache.ResourceCache
		cleanupFuncs []func()
		e            *entry
	)
	BeforeEach(func() {
		sut = resourcecache.New()
		cleanupFuncs = make([]func(), 0)
		e = &entry{
			id: testID,
		}
	})
	It("AddResourceToCache should be able to get resource after adding", func() {
		// Given

		// When
		Expect(sut.AddResourceToCache(testName, e, cleanupFuncs)).To(BeNil())

		// Then
		id := sut.GetResourceFromCache(testName)
		Expect(id).To(Equal(e.id))

		id = sut.GetResourceFromCache(testName)
		Expect(id).To(BeEmpty())
	})
	It("AddResourceToCache should fail to readd resource", func() {
		// Given

		// When
		Expect(sut.AddResourceToCache(testName, e, cleanupFuncs)).To(BeNil())

		// Then
		Expect(sut.AddResourceToCache(testName, e, cleanupFuncs)).NotTo(BeNil())
	})
	It("AddResourceToCache should call cleanup funcs after timeout", func() {
		// Given
		timedOutChan := make(chan bool)
		cleanupFuncs = append(cleanupFuncs, func() {
			timedOutChan <- true
		})
		go func() {
			time.Sleep(3 * time.Minute)
			timedOutChan <- false
		}()

		// When
		Expect(sut.AddResourceToCache(testName, e, cleanupFuncs)).To(BeNil())

		// Then
		didCacheCallTimeoutFunc := <-timedOutChan
		Expect(didCacheCallTimeoutFunc).To(Equal(true))

		id := sut.GetResourceFromCache(testName)
		Expect(id).To(BeEmpty())
	})
	It("GetResourceFromCache should call SetCreated", func() {
		// When
		Expect(sut.AddResourceToCache(testName, e, cleanupFuncs)).To(BeNil())

		// Then
		id := sut.GetResourceFromCache(testName)
		Expect(id).To(Equal(e.id))
		Expect(e.created).To(BeTrue())
	})
})
