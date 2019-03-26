package server_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("Version", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})
	AfterEach(afterEach)

	t.Describe("Version", func() {
		It("should succeed", func() {
			// Given
			request := &pb.VersionRequest{}

			// When
			response, err := sut.Version(context.Background(), request)

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(response.Version).NotTo(BeEmpty())
			Expect(response.RuntimeName).NotTo(BeEmpty())
			Expect(response.RuntimeName).NotTo(BeEmpty())
			Expect(response.RuntimeApiVersion).NotTo(BeEmpty())
		})
	})
})
