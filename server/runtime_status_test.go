package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("Status", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("Status", func() {
		It("should succeed", func() {
			// When
			response, err := sut.Status(context.Background(),
				&types.StatusRequest{})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Status.Conditions)).To(BeEquivalentTo(2))
			for _, condition := range response.Status.Conditions {
				Expect(condition.Status).To(BeTrue())
			}
		})

		It("should succeed when CNI plugin status errors", func() {
			// When
			response, err := sut.Status(context.Background(),
				&types.StatusRequest{})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Status.Conditions)).To(BeEquivalentTo(2))
			for _, condition := range response.Status.Conditions {
				Expect(condition.Status).To(BeTrue())
			}
		})

		It("should return info as part of a verbose response", func() {
			// When
			response, err := sut.Status(context.Background(),
				&types.StatusRequest{Verbose: true})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(response.Info).NotTo(BeNil())
		})
	})
})
