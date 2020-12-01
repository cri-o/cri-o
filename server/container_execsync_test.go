package server_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("ContainerStart", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStart", func() {
		It("should fail with invalid container ID", func() {
			// Given
			// When
			response, err := sut.ExecSync(context.Background(),
				&pb.ExecSyncRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
