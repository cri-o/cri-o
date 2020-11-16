package server_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("UpdateRuntimeConfig", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("UpdateRuntimeConfig", func() {
		It("should succeed", func() {
			// Given
			// When
			response, err := sut.UpdateRuntimeConfig(context.Background(),
				&pb.UpdateRuntimeConfigRequest{})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})
	})
})
