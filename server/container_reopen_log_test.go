package server_test

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("ReopenContainerLog", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ReopenContainerLog", func() {
		It("should fail on invalid container ID", func() {
			// Given
			// When
			err := sut.ReopenContainerLog(
				context.Background(),
				&types.ReopenContainerLogRequest{},
			)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
