package lib

import (
	"context"
	"os"
	"runtime"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v8/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rspec "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/cri-o/cri-o/internal/version"
)

var _ = Describe("getCheckpointAnnotations", func() {
	It("should return engine name cri-o", func() {
		ann := getCheckpointAnnotations()
		Expect(ann[metadata.CheckpointAnnotationEngine]).To(Equal("cri-o"))
	})

	It("should return engine version matching version.Version", func() {
		ann := getCheckpointAnnotations()
		Expect(ann[metadata.CheckpointAnnotationEngineVersion]).To(Equal(version.Version))
	})

	It("should return host arch matching runtime.GOARCH", func() {
		ann := getCheckpointAnnotations()
		Expect(ann[metadata.CheckpointAnnotationHostArch]).To(Equal(runtime.GOARCH))
	})

	It("should return cgroup version", func() {
		ann := getCheckpointAnnotations()
		cgroupVersion := ann[metadata.CheckpointAnnotationCgroupVersion]
		// Cgroup detection is currently disabled (requires linux-only build tag),
		// so the value is "v?" until the file is made linux-only.
		Expect(cgroupVersion).To(BeElementOf("v1", "v2", "v?"))
	})

	It("should return kernel version as non-empty string", func() {
		ann := getCheckpointAnnotations()
		Expect(ann).To(HaveKey(metadata.CheckpointAnnotationHostKernel))
		Expect(ann[metadata.CheckpointAnnotationHostKernel]).ToNot(BeEmpty())
	})

	It("should return CRIU version if available", func() {
		if err := utils.CheckForCriu(utils.PodCriuVersion); err != nil {
			Skip("CRIU not available: " + err.Error())
		}

		ann := getCheckpointAnnotations()
		Expect(ann).To(HaveKey(metadata.CheckpointAnnotationCriuVersion))
		Expect(ann[metadata.CheckpointAnnotationCriuVersion]).ToNot(BeEmpty())
	})
})

var _ = Describe("writePodCheckpointMetadata", func() {
	It("should write pod.options file with correct container map and annotations", func() {
		cs := &ContainerServer{}
		tempDir, err := os.MkdirTemp("", "test-pod-metadata-")
		Expect(err).ToNot(HaveOccurred())

		defer os.RemoveAll(tempDir)

		ann := map[string]string{
			metadata.CheckpointAnnotationPod:    "my-pod",
			metadata.CheckpointAnnotationEngine: "cri-o",
		}
		err = cs.writePodCheckpointMetadata(context.Background(), tempDir, map[string]string{"name1": "ctr1-name1", "name2": "ctr2-name2"}, ann)
		Expect(err).ToNot(HaveOccurred())

		readBack := &metadata.CheckpointedPodOptions{}
		_, err = metadata.ReadJSONFile(readBack, tempDir, metadata.PodOptionsFile)
		Expect(err).ToNot(HaveOccurred())
		Expect(readBack.Version).To(Equal(1))
		Expect(readBack.Containers).To(Equal(map[string]string{"name1": "ctr1-name1", "name2": "ctr2-name2"}))
		Expect(readBack.Annotations).To(Equal(ann))
	})

	It("should write metadata with empty container map", func() {
		cs := &ContainerServer{}
		tempDir, err := os.MkdirTemp("", "test-pod-metadata-empty-")
		Expect(err).ToNot(HaveOccurred())

		defer os.RemoveAll(tempDir)

		err = cs.writePodCheckpointMetadata(context.Background(), tempDir, map[string]string{}, nil)
		Expect(err).ToNot(HaveOccurred())

		readBack := &metadata.CheckpointedPodOptions{}
		_, err = metadata.ReadJSONFile(readBack, tempDir, metadata.PodOptionsFile)
		Expect(err).ToNot(HaveOccurred())
		Expect(readBack.Version).To(Equal(1))
		Expect(readBack.Containers).To(BeEmpty())
	})

	It("should fail with non-existent mount point", func() {
		cs := &ContainerServer{}
		err := cs.writePodCheckpointMetadata(context.Background(), "/nonexistent/path/for/test", map[string]string{"ctr1": "ctr1-dir"}, nil)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("skipBindMount", func() {
	It("should return true when a bind mount matches the path", func() {
		spec := &rspec.Spec{
			Mounts: []rspec.Mount{
				{
					Type:        "bind",
					Source:      "/host/data",
					Destination: "/container/data",
				},
			},
		}
		Expect(skipBindMount("/container/data", spec)).To(BeTrue())
	})

	It("should return false when no bind mount matches the path", func() {
		spec := &rspec.Spec{
			Mounts: []rspec.Mount{
				{
					Type:        "bind",
					Source:      "/host/data",
					Destination: "/container/data",
				},
			},
		}
		Expect(skipBindMount("/other/path", spec)).To(BeFalse())
	})

	It("should skip non-bind mounts even if destination matches", func() {
		spec := &rspec.Spec{
			Mounts: []rspec.Mount{
				{
					Type:        "tmpfs",
					Destination: "/container/data",
				},
			},
		}
		Expect(skipBindMount("/container/data", spec)).To(BeFalse())
	})

	It("should return false with empty mount list", func() {
		spec := &rspec.Spec{}
		Expect(skipBindMount("/any/path", spec)).To(BeFalse())
	})
})
