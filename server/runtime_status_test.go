package server_test

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
			// Given
			gomock.InOrder(
				cniPluginMock.EXPECT().Status().Return(nil),
			)

			// When
			response, err := sut.Status(context.Background(),
				&types.StatusRequest{})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Status.Conditions)).To(BeEquivalentTo(2))
			Expect(response.Status.Conditions[0].Status).To(BeTrue())
		})

		It("should succeed when CNI plugin status errors", func() {
			// When
			response, err := sut.Status(context.Background(),
				&types.StatusRequest{})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Status.Conditions)).To(BeEquivalentTo(2))
			Expect(response.Status.Conditions[0].Status).To(BeTrue())
		})
	})
})
