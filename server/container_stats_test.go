package server_test

import (
	"context"
	"errors"

	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
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
			Expect(err).NotTo(BeNil())
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
			storeMock.EXPECT().GraphDriver().Return(nil, errors.New("not implemented"))

			// When
			response, err := sut.ListContainerStats(context.Background(),
				&types.ListContainerStatsRequest{})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Stats)).To(Equal(1))
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
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Stats)).To(Equal(0))
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
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Stats)).To(Equal(0))
		})
	})
})
