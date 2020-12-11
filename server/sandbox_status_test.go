package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/server/cri/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// The actual test suite
var _ = t.Describe("PodSandboxStatus", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("PodSandboxStatus", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetStateAndSpoofPid(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})

			// When
			response, err := sut.PodSandboxStatus(context.Background(),
				&types.PodSandboxStatusRequest{PodSandboxID: testSandbox.ID()})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should succeed with multiple IPs", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			const (
				ipv4 = "10.0.0.2"
				ipv6 = "ff02::1"
			)
			testSandbox.AddIPs([]string{ipv4, ipv6})

			// When
			response, err := sut.PodSandboxStatus(context.Background(),
				&types.PodSandboxStatusRequest{PodSandboxID: testSandbox.ID()})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(response.Status.Network.IP).To(Equal(ipv4))
			Expect(response.Status.Network.AdditionalIps).To(HaveLen(1))
			Expect(response.Status.Network.AdditionalIps[0].IP).To(Equal(ipv6))
		})

		It("should fail with empty sandbox ID", func() {
			// Given
			// When
			response, err := sut.PodSandboxStatus(context.Background(),
				&types.PodSandboxStatusRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should return info as part of a verbose response", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetStateAndSpoofPid(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			testContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			// When
			response, err := sut.PodSandboxStatus(context.Background(),
				&types.PodSandboxStatusRequest{PodSandboxID: testSandbox.ID(), Verbose: true})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(response.Info).NotTo(BeNil())
			Expect(response.Info["info"]).To(ContainSubstring(`"ociVersion":"1.0.0"`))
			Expect(response.Info["info"]).To(ContainSubstring(`"image":"pauseImage"`))
		})
	})
})
