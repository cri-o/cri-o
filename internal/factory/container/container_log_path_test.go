package container_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	configLogPath  = "ctrLogPath"
	configLogDir   = "sboxLogDir"
	providedLogDir = "providedLogDir"
)

var _ = t.Describe("Container:LogPath", func() {
	It("should succeed to get log path from container config", func() {
		// Given
		config := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			LogPath:  configLogPath,
		}

		sboxConfig := &types.PodSandboxConfig{
			LogDirectory: configLogDir,
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

		// Then
		logPath, err := sut.LogPath(providedLogDir)
		Expect(err).ToNot(HaveOccurred())
		Expect(logPath).To(ContainSubstring(configLogPath))
		Expect(logPath).To(ContainSubstring(configLogDir))
		Expect(logPath).NotTo(ContainSubstring(providedLogDir))
	})
	It("should use provided log dir if sbox config doesn't provide", func() {
		// Given
		config := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			LogPath:  configLogPath,
		}

		sboxConfig := &types.PodSandboxConfig{}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

		// Then
		logPath, err := sut.LogPath(providedLogDir)
		Expect(err).ToNot(HaveOccurred())
		Expect(logPath).To(ContainSubstring(configLogPath))
		Expect(logPath).To(ContainSubstring(providedLogDir))
	})
	It("should use ctrID if log path empty", func() {
		// Given
		config := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{
				Name: "name",
			},
		}

		sboxConfig := &types.PodSandboxConfig{
			Metadata: &types.PodSandboxMetadata{},
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())
		Expect(sut.SetNameAndID("")).To(Succeed())

		// Then
		logPath, err := sut.LogPath(providedLogDir)
		Expect(err).ToNot(HaveOccurred())
		Expect(logPath).To(ContainSubstring(providedLogDir))
		Expect(logPath).To(ContainSubstring(sut.ID()))
	})
})
