package server_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
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
				&pb.StatusRequest{})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Status.Conditions)).To(BeEquivalentTo(2))
			Expect(response.Status.Conditions[0].Status).To(BeTrue())
		})

		It("should succeed when CNI plugin status erros", func() {
			// Given
			gomock.InOrder(
				cniPluginMock.EXPECT().Status().Return(t.TestError),
			)

			// When
			response, err := sut.Status(context.Background(),
				&pb.StatusRequest{})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Status.Conditions)).To(BeEquivalentTo(2))
			Expect(response.Status.Conditions[0].Status).To(BeTrue())
		})
	})
})
