package container_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

var _ = t.Describe("Container", func() {
	var config *pb.ContainerConfig
	var sboxConfig *pb.PodSandboxConfig
	BeforeEach(func() {
		config = &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
		}
		sboxConfig = &pb.PodSandboxConfig{}
	})
	t.Describe("FipsDisable", func() {
		It("should be true when set to true", func() {
			// Given
			labels := make(map[string]string)
			labels["FIPS_DISABLE"] = "true"
			sboxConfig.Labels = labels

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

			// Then
			Expect(sut.DisableFips()).To(Equal(true))
		})
		It("should be false when set to false", func() {
			// Given
			labels := make(map[string]string)
			labels["FIPS_DISABLE"] = "false"
			sboxConfig.Labels = labels

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

			// Then
			Expect(sut.DisableFips()).To(Equal(false))
		})
		It("should be false when not set", func() {
			// Given

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

			// Then
			Expect(sut.DisableFips()).To(Equal(false))
		})
	})
})
