package server_test

import (
	"context"

	"github.com/containers/common/pkg/resize"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
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
		It("should succeed", func() {
			// Given
			// When
			response, err := sut.Exec(context.Background(),
				&types.ExecRequest{
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
			response, err := sut.Exec(context.Background(),
				&types.ExecRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("StreamServer: Exec", func() {
		It("shoud fail when container not found", func() {
			// Given
			// When
			err := testStreamService.Exec(context.Background(), testContainer.ID(), []string{},
				nil, nil, nil, false, make(chan resize.TerminalSize))

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
