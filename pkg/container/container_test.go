package container_test

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/containers/podman/v2/pkg/annotations"
	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

var _ = t.Describe("Container", func() {
	var config *pb.ContainerConfig
	var sboxConfig *pb.PodSandboxConfig
	const defaultMounts = 6
	BeforeEach(func() {
		config = &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
		}
		sboxConfig = &pb.PodSandboxConfig{}
	})
	t.Describe("SpecAddMount", func() {
		It("should add the mount to the spec", func() {
			sut.SpecAddMount(rspec.Mount{
				Destination: "test",
				Type:        "test",
				Source:      "test",
				Options:     []string{"test"},
			})
			Expect(len(sut.Spec().Mounts())).To(Equal(defaultMounts + 1))
		})
		It("should add only one copy to the spec", func() {
			sut.SpecAddMount(rspec.Mount{
				Destination: "test",
				Type:        "test",
				Source:      "test",
				Options:     []string{"test"},
			})
			sut.SpecAddMount(rspec.Mount{
				Destination: "test",
				Type:        "test",
				Source:      "test",
				Options:     []string{"test"},
			})
			Expect(len(sut.Spec().Mounts())).To(Equal(defaultMounts + 1))
		})
	})
	t.Describe("Spec", func() {
		It("should return the spec", func() {
			Expect(sut.Spec()).ToNot(Equal(nil))
		})
	})
	t.Describe("SpecAddAnnotations", func() {
		It("should set the spec annotations", func() {
			// Given
			sandboxConfig := &pb.PodSandboxConfig{
				Metadata: &pb.PodSandboxMetadata{Name: "name"},
			}
			containerConfig := &pb.ContainerConfig{
				Metadata: &pb.ContainerMetadata{Name: "name"},
				Linux: &pb.LinuxContainerConfig{
					SecurityContext: &pb.LinuxContainerSecurityContext{
						Privileged: true,
					},
				},
				Image: &pb.ImageSpec{
					Image: "img",
				},
			}
			err := sut.SetConfig(containerConfig, sandboxConfig)
			Expect(err).To(BeNil())
			currentTime := time.Now()
			volumes := []oci.ContainerVolume{}
			imageResult := storage.ImageResult{}
			mountPoint := "test"
			configStopSignal := "test"

			sb, err := sandbox.New("sandboxID", "", "", "", "test",
				make(map[string]string), make(map[string]string), "", "",
				&pb.PodSandboxMetadata{}, "", "", false, "", "", "",
				[]*hostport.PortMapping{}, false, currentTime, "")
			Expect(err).To(BeNil())

			image, err := sut.Image()
			Expect(err).To(BeNil())

			logpath, err := sut.LogPath(sb.LogDir())
			Expect(err).To(BeNil())

			metadataJSON, err := json.Marshal(sut.Config().GetMetadata())
			Expect(err).To(BeNil())

			labelsJSON, err := json.Marshal(sut.Config().GetLabels())
			Expect(err).To(BeNil())

			volumesJSON, err := json.Marshal(volumes)
			Expect(err).To(BeNil())

			kubeAnnotationsJSON, err := json.Marshal(sut.Config().GetAnnotations())
			Expect(err).To(BeNil())

			Expect(currentTime).ToNot(BeNil())
			Expect(sb).ToNot(BeNil())

			err = sut.SpecAddAnnotations(sb, volumes, mountPoint, configStopSignal, &imageResult, false, false)
			Expect(err).To(BeNil())

			Expect(sut.Spec().Config.Annotations[annotations.Image]).To(Equal(image))
			Expect(sut.Spec().Config.Annotations[annotations.ImageName]).To(Equal(imageResult.Name))
			Expect(sut.Spec().Config.Annotations[annotations.ImageRef]).To(Equal(imageResult.ID))
			Expect(sut.Spec().Config.Annotations[annotations.Name]).To(Equal(sut.Name()))
			Expect(sut.Spec().Config.Annotations[annotations.ContainerID]).To(Equal(sut.ID()))
			Expect(sut.Spec().Config.Annotations[annotations.SandboxID]).To(Equal(sb.ID()))
			Expect(sut.Spec().Config.Annotations[annotations.SandboxName]).To(Equal(sb.Name()))
			Expect(sut.Spec().Config.Annotations[annotations.ContainerType]).To(Equal(annotations.ContainerTypeContainer))
			Expect(sut.Spec().Config.Annotations[annotations.LogPath]).To(Equal(logpath))
			Expect(sut.Spec().Config.Annotations[annotations.TTY]).To(Equal(strconv.FormatBool(sut.Config().Tty)))
			Expect(sut.Spec().Config.Annotations[annotations.Stdin]).To(Equal(strconv.FormatBool(sut.Config().Stdin)))
			Expect(sut.Spec().Config.Annotations[annotations.StdinOnce]).To(Equal(strconv.FormatBool(sut.Config().StdinOnce)))
			Expect(sut.Spec().Config.Annotations[annotations.ResolvPath]).To(Equal(sb.ResolvPath()))
			Expect(sut.Spec().Config.Annotations[annotations.ContainerManager]).To(Equal(lib.ContainerManagerCRIO))
			Expect(sut.Spec().Config.Annotations[annotations.MountPoint]).To(Equal(mountPoint))
			Expect(sut.Spec().Config.Annotations[annotations.SeccompProfilePath]).To(Equal(sut.Config().GetLinux().GetSecurityContext().GetSeccompProfilePath()))
			Expect(sut.Spec().Config.Annotations[annotations.Created]).ToNot(BeNil())
			Expect(sut.Spec().Config.Annotations[annotations.Metadata]).To(Equal(string(metadataJSON)))
			Expect(sut.Spec().Config.Annotations[annotations.Labels]).To(Equal(string(labelsJSON)))
			Expect(sut.Spec().Config.Annotations[annotations.Volumes]).To(Equal(string(volumesJSON)))
			Expect(sut.Spec().Config.Annotations[annotations.Annotations]).To(Equal(string(kubeAnnotationsJSON)))
		})
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
	t.Describe("Image", func() {
		It("should fail when spec not set", func() {
			// Given

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

			// Then
			img, err := sut.Image()
			Expect(err).NotTo(BeNil())
			Expect(img).To(BeEmpty())
		})
		It("should fail when image not set", func() {
			// Given
			config.Image = &pb.ImageSpec{}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

			// Then
			img, err := sut.Image()
			Expect(err).NotTo(BeNil())
			Expect(img).To(BeEmpty())
		})
		It("should be succeed when set", func() {
			// Given
			testImage := "img"
			config.Image = &pb.ImageSpec{
				Image: testImage,
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

			// Then
			img, err := sut.Image()
			Expect(err).To(BeNil())
			Expect(img).To(Equal(testImage))
		})
	})
	t.Describe("ReadOnly", func() {
		BeforeEach(func() {
			config.Linux = &pb.LinuxContainerConfig{
				SecurityContext: &pb.LinuxContainerSecurityContext{},
			}
		})
		It("should not be readonly by default", func() {
			// Given

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

			// Then
			Expect(sut.ReadOnly(false)).To(Equal(false))
		})
		It("should be readonly when specified", func() {
			// Given
			config.Linux.SecurityContext.ReadonlyRootfs = true

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

			// Then
			Expect(sut.ReadOnly(false)).To(Equal(true))
		})
		It("should be readonly when server is", func() {
			// Given

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

			// Then
			Expect(sut.ReadOnly(true)).To(Equal(true))
		})
	})
	t.Describe("SelinuxLabel", func() {
		BeforeEach(func() {
			config.Linux = &pb.LinuxContainerConfig{
				SecurityContext: &pb.LinuxContainerSecurityContext{},
			}
		})
		It("should be empty by default", func() {
			// Given

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

			// Then
			labels, err := sut.SelinuxLabel("")
			Expect(labels).To(BeEmpty())
			Expect(err).To(BeNil())
		})
		It("should not be empty when specified in config", func() {
			// Given
			config.Linux.SecurityContext.SelinuxOptions = &pb.SELinuxOption{
				User:  "a_u",
				Role:  "a_r",
				Type:  "a_t",
				Level: "a_l",
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

			// Then
			labels, err := sut.SelinuxLabel("")
			Expect(len(labels)).To(Equal(4))
			Expect(err).To(BeNil())
		})
		It("should not be empty when specified in sandbox", func() {
			// Given

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

			// Then
			labels, err := sut.SelinuxLabel("a_u:a_t:a_r")
			Expect(len(labels)).To(Equal(3))
			Expect(err).To(BeNil())
		})
	})
})
