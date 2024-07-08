package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

// The actual test suite.
var _ = t.Describe("ContainerStats", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStats", func() {
		It("should fail on invalid container", func() {
			// Given
			// When
			response, err := sut.ContainerStats(context.Background(),
				&types.ContainerStatsRequest{})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})
	})
})

var _ = t.Describe("ContainerStatsList", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStatsList", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.ListContainerStats(context.Background(),
				&types.ListContainerStatsRequest{})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(response.Stats).To(BeEmpty())
		})
		It("should filter stopped container", func() {
			// Given
			state := oci.ContainerState{}
			state.Status = oci.ContainerStateStopped
			testContainer.SetState(&state)
			addContainerAndSandbox()

			// When
			response, err := sut.ListContainerStats(context.Background(),
				&types.ListContainerStatsRequest{},
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(response.Stats).To(BeEmpty())
		})
		It("should filter by id", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.ListContainerStats(context.Background(),
				&types.ListContainerStatsRequest{
					Filter: &types.ContainerStatsFilter{
						Id: "invalid",
					},
				},
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(response.Stats).To(BeEmpty())
		})
	})
})
