package oci_test

import (
	"context"
	"os"

	criu "github.com/checkpoint-restore/go-criu/v7/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/cri-o/cri-o/internal/oci"
	v2 "github.com/cri-o/cri-o/pkg/annotations/v2"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

// The actual test suite.
var _ = t.Describe("Oci", func() {
	t.Describe("New", func() {
		It("should succeed with default config", func() {
			// Given
			c, err := libconfig.DefaultConfig()
			Expect(err).ToNot(HaveOccurred())
			// so we have permission to make a directory within it
			c.ContainerAttachSocketDir = t.MustTempDir("crio")

			// When
			runtime, err := oci.New(c)
			Expect(err).ToNot(HaveOccurred())

			// Then
			Expect(runtime).NotTo(BeNil())
		})
	})

	t.Describe("Oci", func() {
		// The system under test
		var sut *oci.Runtime

		// Test constants
		const (
			invalidRuntime     = "invalid"
			defaultRuntime     = "runc"
			usernsRuntime      = "userns"
			performanceRuntime = "high-performance"
			vmRuntime          = "kata"
		)
		runtimes := libconfig.Runtimes{
			defaultRuntime: &libconfig.RuntimeHandler{
				RuntimePath: "/bin/sh",
				RuntimeType: "",
				RuntimeRoot: "/run/runc",
			},
			invalidRuntime: &libconfig.RuntimeHandler{},
			usernsRuntime: &libconfig.RuntimeHandler{
				RuntimePath:        "/bin/sh",
				RuntimeType:        "",
				RuntimeRoot:        "/run/runc",
				AllowedAnnotations: []string{v2.UsernsMode},
			},
			performanceRuntime: &libconfig.RuntimeHandler{
				RuntimePath: "/bin/sh",
				RuntimeType: "",
				RuntimeRoot: "/run/runc",
				AllowedAnnotations: []string{
					v2.CPULoadBalancing,
					v2.IRQLoadBalancing,
					v2.CPUQuota,
					v2.OCISeccompBPFHook,
				},
			},
			vmRuntime: &libconfig.RuntimeHandler{
				RuntimePath:                  "/usr/bin/containerd-shim-kata-v2",
				RuntimeType:                  "vm",
				RuntimeRoot:                  "/run/vc",
				PrivilegedWithoutHostDevices: true,
				RuntimeConfigPath:            "/opt/kata-containers/config.toml",
			},
		}

		BeforeEach(func() {
			var err error
			config, err = libconfig.DefaultConfig()
			Expect(err).ToNot(HaveOccurred())
			config.DefaultRuntime = defaultRuntime
			config.Runtimes = runtimes
			// so we have permission to make a directory within it
			config.ContainerAttachSocketDir = t.MustTempDir("crio")

			sut, err = oci.New(config)
			Expect(err).ToNot(HaveOccurred())
			Expect(sut).NotTo(BeNil())
		})

		It("should succeed to retrieve the runtimes", func() {
			// Given
			// When
			result := sut.Runtimes()

			// Then
			Expect(result).To(Equal(runtimes))
		})

		It("should succeed to validate a runtime handler", func() {
			// Given
			// When
			handler, err := sut.ValidateRuntimeHandler(defaultRuntime)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(handler).To(Equal(runtimes[defaultRuntime]))
		})
		It("should return an OCI runtime type if none is set", func() {
			// Given
			// When
			runtimeType, err := sut.RuntimeType(defaultRuntime)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(runtimeType).To(Equal(""))
		})
		It("should return a VM runtime type when it is set", func() {
			// Given
			// When
			runtimeType, err := sut.RuntimeType(vmRuntime)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(runtimeType).To(Equal(libconfig.RuntimeTypeVM))
		})
		It("Seccomp should return the runtime seccomp config", func() {
			// Given
			// When
			_, err := sut.Seccomp(defaultRuntime)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
		It("Seccomp should fail when runtime is not present", func() {
			// Given
			// When
			_, err := sut.Seccomp(invalidRuntime)

			// Then
			Expect(err).To(HaveOccurred())
		})
		Context("AllowedAnnotations", func() {
			It("should succeed to return allowed annotation", func() {
				// Given
				Expect(runtimes[performanceRuntime].ValidateRuntimeAllowedAnnotations()).To(Succeed())

				// When
				foundAnn, err := sut.AllowedAnnotations(performanceRuntime)

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(foundAnn).NotTo(ContainElement(v2.Devices))
				Expect(foundAnn).To(ContainElement(v2.IRQLoadBalancing))
			})
			It("should fail to return allowed annotation of unknown runtime", func() {
				// Given
				// When
				_, err := sut.AllowedAnnotations("invalid")

				// Then
				Expect(err).To(HaveOccurred())
			})
		})

		It("PrivilegedWithoutHostDevices should be true when set", func() {
			// Given
			// When
			privileged, err := sut.PrivilegedWithoutHostDevices(vmRuntime)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(privileged).To(BeTrue())
		})
		It("PrivilegedWithoutHostDevices should be false when runtime invalid", func() {
			// Given
			// When
			privileged, err := sut.PrivilegedWithoutHostDevices(invalidRuntime)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(privileged).To(BeFalse())
		})
		It("PrivilegedWithoutHostDevices should be false when runtime is the default", func() {
			// Given
			// When
			privileged, err := sut.PrivilegedWithoutHostDevices(defaultRuntime)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(privileged).To(BeFalse())
		})
		It("CheckpointContainer should succeed", func() {
			if err := criu.CheckForCriu(criu.PodCriuVersion); err != nil {
				Skip("Check CRIU: " + err.Error())
			}
			// Given
			beforeEach()
			defer os.RemoveAll("dump.log")
			config.Runtimes["runc"] = &libconfig.RuntimeHandler{
				RuntimePath: "/bin/true",
			}

			specgen := &specs.Spec{
				Version: "1.0.0",
				Process: &specs.Process{
					SelinuxLabel: "",
				},
				Linux: &specs.Linux{
					MountLabel: "",
				},
			}
			// When
			err := sut.CheckpointContainer(context.Background(), myContainer, specgen, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
		It("CheckpointContainer should fail", func() {
			if err := criu.CheckForCriu(criu.PodCriuVersion); err != nil {
				Skip("Check CRIU: " + err.Error())
			}
			// Given
			defer os.RemoveAll("dump.log")
			beforeEach()
			config.Runtimes["runc"] = &libconfig.RuntimeHandler{
				RuntimePath: "/bin/false",
			}

			specgen := &specs.Spec{
				Version: "1.0.0",
				Process: &specs.Process{
					SelinuxLabel: "",
				},
				Linux: &specs.Linux{
					MountLabel: "",
				},
			}
			// When
			err := sut.CheckpointContainer(context.Background(), myContainer, specgen, true)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("configured runtime does not support checkpoint/restore"))
		})
		It("RestoreContainer should fail with destination sandbox detection", func() {
			if err := criu.CheckForCriu(criu.PodCriuVersion); err != nil {
				Skip("Check CRIU: " + err.Error())
			}
			// Given
			beforeEach()
			config.Runtimes["runc"] = &libconfig.RuntimeHandler{
				RuntimePath: "/bin/true",
				MonitorPath: "/bin/true",
			}

			err := os.Mkdir("checkpoint", 0o700)
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("checkpoint")
			inventory, err := os.OpenFile("checkpoint/inventory.img", os.O_RDONLY|os.O_CREATE, 0o644)
			Expect(err).ToNot(HaveOccurred())
			inventory.Close()

			specgen := &specs.Spec{
				Version:     "1.0.0",
				Annotations: map[string]string{"io.kubernetes.cri-o.SandboxID": "sandboxID"},
				Linux: &specs.Linux{
					MountLabel: ".",
				},
				Process: &specs.Process{
					SelinuxLabel: "",
				},
			}
			myContainer.SetSpec(specgen)

			// When
			err = sut.RestoreContainer(context.Background(), myContainer, "no-parent-cgroup-exists", "label")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed"))
		})
		It("RestoreContainer should fail", func() {
			if err := criu.CheckForCriu(criu.PodCriuVersion); err != nil {
				Skip("Check CRIU: " + err.Error())
			}
			// Given
			beforeEach()
			config.Runtimes["runc"] = &libconfig.RuntimeHandler{
				RuntimePath: "/bin/true",
				MonitorPath: "/bin/true",
			}

			specgen := &specs.Spec{
				Version:     "1.0.0",
				Annotations: map[string]string{"io.kubernetes.cri-o.SandboxID": "sandboxID"},
				Linux: &specs.Linux{
					MountLabel: ".",
				},
				Process: &specs.Process{
					SelinuxLabel: "",
				},
			}
			myContainer.SetSpec(specgen)

			err := os.Mkdir("checkpoint", 0o700)
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("checkpoint")
			inventory, err := os.OpenFile("checkpoint/inventory.img", os.O_RDONLY|os.O_CREATE, 0o644)
			Expect(err).ToNot(HaveOccurred())
			inventory.Close()

			err = os.WriteFile(
				"config.json",
				[]byte(
					`{"ociVersion": "1.0.0","annotations":`+
						`{"io.kubernetes.cri-o.SandboxID": "sandboxID"},`+
						`"linux": {"mountLabel": ""}}`,
				),
				0o644,
			)
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("config.json")

			config.Conmon = "/bin/true"

			// When
			err = sut.RestoreContainer(context.Background(), myContainer, "no-parent-cgroup-exists", "label")
			defer os.RemoveAll("restore.log")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed"))
		})
		It("RestoreContainer should fail with missing inventory", func() {
			if err := criu.CheckForCriu(criu.PodCriuVersion); err != nil {
				Skip("Check CRIU: " + err.Error())
			}
			// Given
			beforeEach()
			// When
			err := sut.RestoreContainer(context.Background(), myContainer, "no-parent-cgroup-exists", "label")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("a complete checkpoint for this container cannot be found, cannot restore: stat checkpoint/inventory.img: no such file or directory"))
		})
	})
})
