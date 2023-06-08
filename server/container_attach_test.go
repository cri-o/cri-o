package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/remotecommand"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("ContainerAttach", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerAttach", func() {
		It("should succeed", func() {
			// Given
			// When
			response, err := sut.Attach(context.Background(),
				&types.AttachRequest{
					ContainerId: testContainer.ID(),
					Stdout:      true,
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail on invalid request", func() {
			// Given
			// When
			response, err := sut.Attach(context.Background(),
				&types.AttachRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("StreamServer: Attach", func() {
		It("should fail if container was not found", func() {
			// Given
			// When
			err := testStreamService.Attach(context.Background(), testContainer.ID(),
				nil, nil, nil, false, make(chan remotecommand.TerminalSize))

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
