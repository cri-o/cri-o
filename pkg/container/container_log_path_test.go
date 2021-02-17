package container_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	configLogPath  = "ctrLogPath"
	configLogDir   = "sboxLogDir"
	providedLogDir = "providedLogDir"
)

var _ = t.Describe("Container:LogPath", func() {
	It("should succeed to get log path from container config", func() {
		// Given
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
			LogPath:  configLogPath,
		}

		sboxConfig := &pb.PodSandboxConfig{
			LogDirectory: configLogDir,
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		logPath, err := sut.LogPath(providedLogDir)
		Expect(err).To(BeNil())
		Expect(logPath).To(ContainSubstring(configLogPath))
		Expect(logPath).To(ContainSubstring(configLogDir))
		Expect(logPath).NotTo(ContainSubstring(providedLogDir))
	})
	It("should use provided log dir if sbox config doesn't provide", func() {
		// Given
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
			LogPath:  configLogPath,
		}

		sboxConfig := &pb.PodSandboxConfig{}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		logPath, err := sut.LogPath(providedLogDir)
		Expect(err).To(BeNil())
		Expect(logPath).To(ContainSubstring(configLogPath))
		Expect(logPath).To(ContainSubstring(providedLogDir))
	})
	It("should use ctrID if log path empty", func() {
		// Given
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{
				Name: "name",
			},
		}

		sboxConfig := &pb.PodSandboxConfig{
			Metadata: &pb.PodSandboxMetadata{},
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())
		Expect(sut.SetNameAndID()).To(BeNil())

		// Then
		logPath, err := sut.LogPath(providedLogDir)
		Expect(err).To(BeNil())
		Expect(logPath).To(ContainSubstring(providedLogDir))
		Expect(logPath).To(ContainSubstring(sut.ID()))
	})
})
