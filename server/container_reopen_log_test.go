package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite.
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
			_, err := sut.ReopenContainerLog(
				context.Background(),
				&types.ReopenContainerLogRequest{},
			)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
})
