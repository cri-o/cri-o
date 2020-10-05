package resourcestore_test

import (
	"time"

	"github.com/cri-o/cri-o/internal/resourcestore"
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
var _ = t.Describe("ResourceStore", func() {
	// Setup the test
	var (
		sut          *resourcestore.ResourceStore
		cleanupFuncs []func()
		e            *entry
	)
	BeforeEach(func() {
		sut = resourcestore.New()
		cleanupFuncs = make([]func(), 0)
		e = &entry{
			id: testID,
		}
	})
	It("Put should be able to get resource after adding", func() {
		// Given

		// When
		Expect(sut.Put(testName, e, cleanupFuncs)).To(BeNil())

		// Then
		id := sut.Get(testName)
		Expect(id).To(Equal(e.id))

		id = sut.Get(testName)
		Expect(id).To(BeEmpty())
	})
	It("Put should fail to readd resource", func() {
		// Given

		// When
		Expect(sut.Put(testName, e, cleanupFuncs)).To(BeNil())

		// Then
		Expect(sut.Put(testName, e, cleanupFuncs)).NotTo(BeNil())
	})
	It("Get should call SetCreated", func() {
		// When
		Expect(sut.Put(testName, e, cleanupFuncs)).To(BeNil())

		// Then
		id := sut.Get(testName)
		Expect(id).To(Equal(e.id))
		Expect(e.created).To(BeTrue())
	})
})

var _ = t.Describe("ResourceStore and timeout", func() {
	// Setup the test
	var (
		sut          *resourcestore.ResourceStore
		cleanupFuncs []func()
		e            *entry
	)
	BeforeEach(func() {
		cleanupFuncs = make([]func(), 0)
		e = &entry{
			id: testID,
		}
	})
	It("Put should call cleanup funcs after timeout", func() {
		// Given
		timeout := 2 * time.Second
		sut = resourcestore.NewWithTimeout(timeout)

		timedOutChan := make(chan bool)
		cleanupFuncs = append(cleanupFuncs, func() {
			timedOutChan <- true
		})
		go func() {
			time.Sleep(timeout * 3)
			timedOutChan <- false
		}()

		// When
		Expect(sut.Put(testName, e, cleanupFuncs)).To(BeNil())

		// Then
		didStoreCallTimeoutFunc := <-timedOutChan
		Expect(didStoreCallTimeoutFunc).To(Equal(true))

		id := sut.Get(testName)
		Expect(id).To(BeEmpty())
	})
})
