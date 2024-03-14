package container_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/containers/podman/v4/pkg/annotations"
	"github.com/cri-o/cri-o/internal/config/capabilities"
	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	crioann "github.com/cri-o/cri-o/pkg/annotations"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	validate "github.com/opencontainers/runtime-tools/validate/capabilities"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletTypes "k8s.io/kubelet/pkg/types"
)

var _ = t.Describe("Container", func() {
	var config *types.ContainerConfig
	var sboxConfig *types.PodSandboxConfig
	const defaultMounts = 6
	BeforeEach(func() {
		config = &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
		}
		sboxConfig = &types.PodSandboxConfig{}
	})
	t.Describe("SpecAddMount", func() {
		It("should add the mount to the spec", func() {
			sut.SpecAddMount(rspec.Mount{
				Destination: "test",
				Type:        "test",
				Source:      "test",
				Options:     []string{"test"},
			})
			Expect(sut.Spec().Mounts()).To(HaveLen(defaultMounts + 1))
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
			Expect(sut.Spec().Mounts()).To(HaveLen(defaultMounts + 1))
		})
	})
	t.Describe("Spec", func() {
		It("should return the spec", func() {
			Expect(sut.Spec()).ToNot(BeNil())
		})
	})
	t.Describe("SpecAddAnnotations", func() {
		It("should set the spec annotations", func() {
			// Given
			sandboxConfig := &types.PodSandboxConfig{
				Metadata: &types.PodSandboxMetadata{Name: "name"},
			}
			containerConfig := &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
				Linux: &types.LinuxContainerConfig{
					SecurityContext: &types.LinuxContainerSecurityContext{
						Privileged: true,
					},
				},
				Image: &types.ImageSpec{
					Image: "img",
				},
			}
			err := sut.SetConfig(containerConfig, sandboxConfig)
			Expect(err).ToNot(HaveOccurred())
			currentTime := time.Now()
			volumes := []oci.ContainerVolume{}
			imageID, err := storage.ParseStorageImageIDFromOutOfProcessData("8a788232037eaf17794408ff3df6b922a1aedf9ef8de36afdae3ed0b0381907b")
			Expect(err).ToNot(HaveOccurred())
			imageName, err := references.ParseRegistryImageReferenceFromOutOfProcessData("example.com/repo/image:tag")
			Expect(err).ToNot(HaveOccurred())
			imageResult := storage.ImageResult{
				ID:                  imageID,
				SomeNameOfThisImage: &imageName,
			}
			mountPoint := "test"
			configStopSignal := "test"

			sb, err := sandbox.New("sandboxID", "", "", "", "test",
				make(map[string]string), make(map[string]string), "", "",
				&types.PodSandboxMetadata{}, "", "", false, "", "", "",
				[]*hostport.PortMapping{}, false, currentTime, "", nil, nil)
			Expect(err).ToNot(HaveOccurred())

			image, err := sut.UserRequestedImage()
			Expect(err).ToNot(HaveOccurred())

			logpath, err := sut.LogPath(sb.LogDir())
			Expect(err).ToNot(HaveOccurred())

			metadataJSON, err := json.Marshal(sut.Config().Metadata)
			Expect(err).ToNot(HaveOccurred())

			labelsJSON, err := json.Marshal(sut.Config().Labels)
			Expect(err).ToNot(HaveOccurred())

			volumesJSON, err := json.Marshal(volumes)
			Expect(err).ToNot(HaveOccurred())

			kubeAnnotationsJSON, err := json.Marshal(sut.Config().Annotations)
			Expect(err).ToNot(HaveOccurred())

			Expect(currentTime).ToNot(BeNil())
			Expect(sb).ToNot(BeNil())

			err = sut.SpecAddAnnotations(context.Background(), sb, volumes, mountPoint, configStopSignal, &imageResult, false, "foo", "")
			Expect(err).ToNot(HaveOccurred())

			Expect(sut.Spec().Config.Annotations[annotations.Image]).To(Equal(image))
			Expect(sut.Spec().Config.Annotations[annotations.ImageName]).To(Equal(imageResult.SomeNameOfThisImage.StringForOutOfProcessConsumptionOnly()))
			Expect(sut.Spec().Config.Annotations[annotations.ImageRef]).To(Equal(imageResult.ID.IDStringForOutOfProcessConsumptionOnly()))
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
			Expect(sut.Spec().Config.Annotations[annotations.SeccompProfilePath]).To(Equal("foo"))
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
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.DisableFips()).To(BeTrue())
		})
		It("should be false when set to false", func() {
			// Given
			labels := make(map[string]string)
			labels["FIPS_DISABLE"] = "false"
			sboxConfig.Labels = labels

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.DisableFips()).To(BeFalse())
		})
		It("should be false when not set", func() {
			// Given

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.DisableFips()).To(BeFalse())
		})
	})
	t.Describe("UserRequestedImage", func() {
		It("should fail when spec not set", func() {
			// Given

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			img, err := sut.UserRequestedImage()
			Expect(err).To(HaveOccurred())
			Expect(img).To(BeEmpty())
		})
		It("should fail when image not set", func() {
			// Given
			config.Image = &types.ImageSpec{}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			img, err := sut.UserRequestedImage()
			Expect(err).To(HaveOccurred())
			Expect(img).To(BeEmpty())
		})
		It("should be succeed when set", func() {
			// Given
			testImage := "img"
			config.Image = &types.ImageSpec{
				Image: testImage,
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			img, err := sut.UserRequestedImage()
			Expect(err).ToNot(HaveOccurred())
			Expect(img).To(Equal(testImage))
		})
	})
	t.Describe("ReadOnly", func() {
		BeforeEach(func() {
			config.Linux = &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{},
			}
		})
		It("should not be readonly by default", func() {
			// Given

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.ReadOnly(false)).To(BeFalse())
		})
		It("should be readonly when specified", func() {
			// Given
			config.Linux.SecurityContext.ReadonlyRootfs = true

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.ReadOnly(false)).To(BeTrue())
		})
		It("should be readonly when server is", func() {
			// Given

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.ReadOnly(true)).To(BeTrue())
		})
	})
	t.Describe("Restore", func() {
		It("should not be restore by default", func() {
			// Given
			// When
			// Then
			Expect(sut.Restore()).To(BeFalse())
		})
		It("should be restore when specified", func() {
			// Given
			Expect(sut.Restore()).To(BeFalse())

			// When
			sut.SetRestore(true)

			// Then
			Expect(sut.Restore()).To(BeTrue())
		})
	})
	t.Describe("SelinuxLabel", func() {
		BeforeEach(func() {
			config.Linux = &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{},
			}
		})
		It("should be empty by default", func() {
			// Given

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			labels, err := sut.SelinuxLabel("")
			Expect(labels).To(BeEmpty())
			Expect(err).ToNot(HaveOccurred())
		})
		It("should not be empty when specified in config", func() {
			// Given
			config.Linux.SecurityContext.SelinuxOptions = &types.SELinuxOption{
				User:  "a_u",
				Role:  "a_r",
				Type:  "a_t",
				Level: "a_l",
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			labels, err := sut.SelinuxLabel("")
			Expect(labels).To(HaveLen(4))
			Expect(err).ToNot(HaveOccurred())
		})
		It("should not be empty when specified in sandbox", func() {
			// Given

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			labels, err := sut.SelinuxLabel("a_u:a_t:a_r")
			Expect(labels).To(HaveLen(3))
			Expect(err).ToNot(HaveOccurred())
		})
	})
	t.Describe("AddUnifiedResourcesFromAnnotations", func() {
		It("should add the limits", func() {
			// Given
			containerName := "foo"
			config.Labels = map[string]string{
				kubeletTypes.KubernetesContainerNameLabel: containerName,
			}
			annotationKey := fmt.Sprintf("%s.%s", crioann.UnifiedCgroupAnnotation, containerName)
			annotationsMap := map[string]string{
				annotationKey: "memory.max=1000000;memory.min=MTAwMDA=;memory.low=20000",
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())
			Expect(sut.AddUnifiedResourcesFromAnnotations(annotationsMap)).To(Succeed())

			// Then
			spec := sut.Spec()
			Expect(spec).To(Not(BeNil()))
			Expect(spec.Config.Linux.Resources.Unified["memory.max"]).To(Equal("1000000"))
			Expect(spec.Config.Linux.Resources.Unified["memory.min"]).To(Equal("10000"))
			Expect(spec.Config.Linux.Resources.Unified["memory.low"]).To(Equal("20000"))
		})

		It("should not add the limits for a different container", func() {
			// Given
			containerName := "foo"
			config.Labels = map[string]string{
				kubeletTypes.KubernetesContainerNameLabel: containerName,
			}

			differentContainerName := "bar"
			annotationKey := fmt.Sprintf("%s.%s", crioann.UnifiedCgroupAnnotation, differentContainerName)
			annotationsMap := map[string]string{
				annotationKey: "memory.max=1000000;memory.min=MTAwMDA=;memory.low=20000",
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())
			Expect(sut.AddUnifiedResourcesFromAnnotations(annotationsMap)).To(Succeed())

			// Then
			spec := sut.Spec()
			Expect(spec).To(Not(BeNil()))
			Expect(spec.Config.Linux.Resources.Unified["memory.max"]).To(Equal(""))
			Expect(spec.Config.Linux.Resources.Unified["memory.min"]).To(Equal(""))
			Expect(spec.Config.Linux.Resources.Unified["memory.low"]).To(Equal(""))
		})
	})
	t.Describe("SpecSetProcessArgs", func() {
		It("should fail if empty", func() {
			// Given
			config.Command = nil
			config.Args = nil

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.SpecSetProcessArgs(nil)).NotTo(Succeed())
		})

		It("should set to command", func() {
			// Given
			config.Command = []string{"hello", "world"}
			config.Args = nil

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.SpecSetProcessArgs(nil)).To(Succeed())
			Expect(sut.Spec().Config.Process.Args).To(Equal(config.Command))
		})
		It("should set to Args", func() {
			// Given
			config.Command = nil
			config.Args = []string{"hello", "world"}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.SpecSetProcessArgs(nil)).To(Succeed())
			Expect(sut.Spec().Config.Process.Args).To(Equal(config.Args))
		})
		It("should append args and command", func() {
			// Given
			config.Command = []string{"hi", "earth"}
			config.Args = []string{"hello", "world"}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.SpecSetProcessArgs(nil)).To(Succeed())
			Expect(sut.Spec().Config.Process.Args).To(Equal(append(config.Command, config.Args...)))
		})
		It("should inherit entrypoint from image", func() {
			// Given
			config.Command = nil
			config.Args = []string{"world"}
			img := &v1.Image{
				Config: v1.ImageConfig{
					Entrypoint: []string{"goodbye"},
				},
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.SpecSetProcessArgs(img)).To(Succeed())
			Expect(sut.Spec().Config.Process.Args).To(Equal(append(img.Config.Entrypoint, config.Args...)))
		})
		It("should always use Command if specified", func() {
			// Given
			config.Command = []string{"hello"}
			config.Args = nil
			img := &v1.Image{
				Config: v1.ImageConfig{
					Cmd: []string{"mars"},
				},
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.SpecSetProcessArgs(img)).To(Succeed())
			Expect(sut.Spec().Config.Process.Args).To(Equal(config.Command))
		})
		It("should inherit cmd from image", func() {
			// Given
			config.Command = nil
			config.Args = nil
			img := &v1.Image{
				Config: v1.ImageConfig{
					Cmd: []string{"mars"},
				},
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.SpecSetProcessArgs(img)).To(Succeed())
			Expect(sut.Spec().Config.Process.Args).To(Equal(img.Config.Cmd))
		})
		It("should inherit both from image", func() {
			// Given
			config.Command = nil
			config.Args = nil
			img := &v1.Image{
				Config: v1.ImageConfig{
					Entrypoint: []string{"hello"},
					Cmd:        []string{"mars"},
				},
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.SpecSetProcessArgs(img)).To(Succeed())
			Expect(sut.Spec().Config.Process.Args).To(Equal(append(img.Config.Entrypoint, img.Config.Cmd...)))
		})
	})
	t.Describe("WillRunSystemd", func() {
		It("should be considered systemd container if entrypoint is systemd", func() {
			// Given
			config.Command = nil
			config.Args = nil
			img := &v1.Image{
				Config: v1.ImageConfig{
					Entrypoint: []string{"/usr/bin/systemd"},
				},
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.SpecSetProcessArgs(img)).To(Succeed())
			Expect(sut.WillRunSystemd()).To(BeTrue())
		})

		It("should be considered systemd container if entrypoint is /sbin/init", func() {
			// Given
			config.Command = nil
			config.Args = nil
			img := &v1.Image{
				Config: v1.ImageConfig{
					Entrypoint: []string{"/sbin/init"},
				},
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.SpecSetProcessArgs(img)).To(Succeed())
			Expect(sut.WillRunSystemd()).To(BeTrue())
		})
		It("should not be considered systemd container otherwise", func() {
			// Given
			config.Command = nil
			config.Args = nil
			img := &v1.Image{
				Config: v1.ImageConfig{
					Entrypoint: []string{"systemdless"},
				},
			}

			// When
			Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())

			// Then
			Expect(sut.SpecSetProcessArgs(img)).To(Succeed())
			Expect(sut.WillRunSystemd()).To(BeFalse())
		})
	})
	t.Describe("SpecSetupCapabilities", func() {
		verifyCapValues := func(caps *rspec.LinuxCapabilities, expected int) {
			Expect(caps.Bounding).To(HaveLen(expected))
			Expect(caps.Effective).To(HaveLen(expected))
			Expect(caps.Permitted).To(HaveLen(expected))
			Expect(caps.Inheritable).To(BeEmpty())
			Expect(caps.Ambient).To(BeEmpty())
		}
		It("Empty capabilities should use server capabilities", func() {
			var caps *types.Capability
			serverCaps := capabilities.Default()

			Expect(sut.SpecSetupCapabilities(caps, serverCaps, false)).To(Succeed())
			verifyCapValues(sut.Spec().Config.Process.Capabilities, len(serverCaps))
		})
		It("AddCapabilities should add capability", func() {
			caps := &types.Capability{
				AddCapabilities:  []string{"CHOWN"},
				DropCapabilities: nil,
			}
			serverCaps := []string{}

			Expect(sut.SpecSetupCapabilities(caps, serverCaps, false)).To(Succeed())
			verifyCapValues(sut.Spec().Config.Process.Capabilities, len(serverCaps)+1)
		})
		It("DropCapabilities should drop capability", func() {
			caps := &types.Capability{
				AddCapabilities:  []string{"CHOWN"},
				DropCapabilities: []string{"CHOWN"},
			}
			serverCaps := []string{"CHOWN"}

			Expect(sut.SpecSetupCapabilities(caps, serverCaps, false)).To(Succeed())
			verifyCapValues(sut.Spec().Config.Process.Capabilities, len(serverCaps)-1)
		})
		It("AddCapabilities ALL DropCapabilities one should drop that one", func() {
			caps := &types.Capability{
				AddCapabilities:  []string{"ALL"},
				DropCapabilities: []string{"CHOWN"},
			}
			serverCaps := []string{}

			Expect(sut.SpecSetupCapabilities(caps, serverCaps, false)).To(Succeed())
			// `int(validate.LastCap())+1` represents the total number of `ALL` capabilities
			// in the current environment, while `-1` indicates the removal of `CHOWN` from `ALL`.
			verifyCapValues(sut.Spec().Config.Process.Capabilities, int(validate.LastCap())+1-1)
		})
		It("AddCapabilities one DropCapabilities ALL should add that one", func() {
			caps := &types.Capability{
				AddCapabilities:  []string{"CHOWN"},
				DropCapabilities: []string{"ALL"},
			}
			serverCaps := []string{}

			Expect(sut.SpecSetupCapabilities(caps, serverCaps, false)).To(Succeed())
			verifyCapValues(sut.Spec().Config.Process.Capabilities, 1)
		})
		It("AddCapabilities ALL DropCapabilities ALL should drop all", func() {
			caps := &types.Capability{
				AddCapabilities:  []string{"ALL"},
				DropCapabilities: []string{"ALL"},
			}
			serverCaps := []string{}

			Expect(sut.SpecSetupCapabilities(caps, serverCaps, false)).To(Succeed())
			verifyCapValues(sut.Spec().Config.Process.Capabilities, 0)
		})
		It("Invalid values should fail", func() {
			caps := &types.Capability{
				AddCapabilities: []string{"Not a capability"},
			}
			serverCaps := []string{}

			Expect(sut.SpecSetupCapabilities(caps, serverCaps, false)).NotTo(Succeed())
		})
		It("Should add inheritable capabilities if set", func() {
			caps := &types.Capability{
				AddCapabilities:  []string{"CHOWN"},
				DropCapabilities: []string{"ALL"},
			}
			serverCaps := []string{}

			Expect(sut.SpecSetupCapabilities(caps, serverCaps, true)).To(Succeed())
			Expect(sut.Spec().Config.Process.Capabilities.Inheritable).To(HaveLen(1))
		})
	})
})
