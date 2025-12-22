package server_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.podman.io/image/v5/docker/reference"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/server"
)

// The actual test suite.
var _ = t.Describe("ContainerCreate", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	newContainerConfig := func() *types.ContainerConfig {
		return &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{},
			Image:    &types.ImageSpec{},
			Linux: &types.LinuxContainerConfig{
				Resources: &types.LinuxContainerResources{},
				SecurityContext: &types.LinuxContainerSecurityContext{
					Capabilities:     &types.Capability{},
					NamespaceOptions: &types.NamespaceOption{},
					SelinuxOptions:   &types.SELinuxOption{},
					RunAsUser:        &types.Int64Value{},
					RunAsGroup:       &types.Int64Value{},
				},
			},
		}
	}

	newPodSandboxConfig := func() *types.PodSandboxConfig {
		return &types.PodSandboxConfig{
			Metadata:     &types.PodSandboxMetadata{},
			DnsConfig:    &types.DNSConfig{},
			PortMappings: []*types.PortMapping{},
			Linux: &types.LinuxPodSandboxConfig{
				SecurityContext: &types.LinuxSandboxSecurityContext{
					NamespaceOptions: &types.NamespaceOption{},
					SelinuxOptions:   &types.SELinuxOption{},
					RunAsUser:        &types.Int64Value{},
					RunAsGroup:       &types.Int64Value{},
				},
			},
		}
	}

	t.Describe("ContainerCreate", func() {
		It("should fail when container config image is nil", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId: testSandbox.ID(),
					Config: &types.ContainerConfig{
						Metadata: &types.ContainerMetadata{
							Name: "name",
						},
					},
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail when container config metadata name is empty", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId: testSandbox.ID(),
					Config: &types.ContainerConfig{
						Metadata: &types.ContainerMetadata{},
					},
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail when container config metadata is nil", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId: testSandbox.ID(),
					Config:       &types.ContainerConfig{},
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail when container config is nil", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId:  testSandbox.ID(),
					SandboxConfig: newPodSandboxConfig(),
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail when container is stopped", func() {
			// Given
			ctx := context.TODO()
			addContainerAndSandbox()
			testSandbox.SetStopped(ctx, false)

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId:  testSandbox.ID(),
					Config:        newContainerConfig(),
					SandboxConfig: newPodSandboxConfig(),
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail when container checkpoint archive is empty", func() {
			// Given
			ctx := context.TODO()
			addContainerAndSandbox()
			testSandbox.SetStopped(ctx, false)

			request := &types.CreateContainerRequest{
				PodSandboxId:  testSandbox.ID(),
				Config:        newContainerConfig(),
				SandboxConfig: newPodSandboxConfig(),
			}

			emptyTar := "empty.tar"
			archive, err := os.OpenFile(emptyTar, os.O_RDONLY|os.O_CREATE, 0o644)
			Expect(err).ToNot(HaveOccurred())
			archive.Close()
			defer os.RemoveAll(emptyTar)

			request.Config.Image.Image = emptyTar

			// When
			response, err := sut.CreateContainer(
				context.Background(),
				request,
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail when sandbox not found", func() {
			// Given
			Expect(sut.PodIDIndex().Add(testSandbox.ID())).To(Succeed())

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId:  testSandbox.ID(),
					Config:        newContainerConfig(),
					SandboxConfig: newPodSandboxConfig(),
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail on invalid pod sandbox ID", func() {
			// Given
			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId:  testSandbox.ID(),
					Config:        newContainerConfig(),
					SandboxConfig: newPodSandboxConfig(),
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail on empty pod sandbox ID", func() {
			// Given
			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					Config:        newContainerConfig(),
					SandboxConfig: newPodSandboxConfig(),
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})
	})
})

var _ = t.Describe("FindRepoDigestForImage", func() {
	// Helper to create canonical reference
	mustCanonical := func(s string) reference.Canonical {
		GinkgoHelper()
		ref, err := reference.ParseNormalizedNamed(s)
		Expect(err).ToNot(HaveOccurred())
		canonical, ok := ref.(reference.Canonical)
		Expect(ok).To(BeTrue(), "expected canonical reference for %s", s)

		return canonical
	}

	const (
		digest1 = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		digest2 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		digest3 = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	)

	It("should return empty string when repoDigests is empty", func() {
		result := server.FindRepoDigestForImage(nil, "docker.io/library/nginx:latest")
		Expect(result).To(Equal(""))

		result = server.FindRepoDigestForImage([]reference.Canonical{}, "docker.io/library/nginx:latest")
		Expect(result).To(Equal(""))
	})

	It("should return exact match when userRequestedImage matches a repo digest", func() {
		exactMatch := "docker.io/library/nginx@" + digest1
		repoDigests := []reference.Canonical{
			mustCanonical("docker.io/library/alpine@" + digest2),
			mustCanonical(exactMatch),
			mustCanonical("docker.io/library/busybox@" + digest3),
		}

		result := server.FindRepoDigestForImage(repoDigests, exactMatch)
		Expect(result).To(Equal(exactMatch))
	})

	It("should return matching repository digest when repo name matches", func() {
		repoDigests := []reference.Canonical{
			mustCanonical("docker.io/library/alpine@" + digest1),
			mustCanonical("docker.io/library/nginx@" + digest2),
			mustCanonical("docker.io/library/busybox@" + digest3),
		}

		// User requested nginx:latest, should match docker.io/library/nginx@digest2
		result := server.FindRepoDigestForImage(repoDigests, "nginx:latest")
		Expect(result).To(Equal("docker.io/library/nginx@" + digest2))

		// User requested docker.io/library/nginx:v1, should match docker.io/library/nginx@digest2
		result = server.FindRepoDigestForImage(repoDigests, "docker.io/library/nginx:v1")
		Expect(result).To(Equal("docker.io/library/nginx@" + digest2))
	})

	It("should return first digest when no match is found", func() {
		repoDigests := []reference.Canonical{
			mustCanonical("docker.io/library/alpine@" + digest1),
			mustCanonical("docker.io/library/nginx@" + digest2),
		}

		// User requested an image that doesn't match any repository
		result := server.FindRepoDigestForImage(repoDigests, "redis:latest")
		Expect(result).To(Equal("docker.io/library/alpine@" + digest1))
	})

	It("should return first digest when userRequestedImage is invalid", func() {
		repoDigests := []reference.Canonical{
			mustCanonical("docker.io/library/alpine@" + digest1),
			mustCanonical("docker.io/library/nginx@" + digest2),
		}

		// Invalid image reference
		result := server.FindRepoDigestForImage(repoDigests, ":::invalid:::")
		Expect(result).To(Equal("docker.io/library/alpine@" + digest1))
	})

	It("should handle fully qualified registry names", func() {
		repoDigests := []reference.Canonical{
			mustCanonical("gcr.io/project/image@" + digest1),
			mustCanonical("quay.io/namespace/image@" + digest2),
		}

		// User requested gcr.io/project/image:v1
		result := server.FindRepoDigestForImage(repoDigests, "gcr.io/project/image:v1")
		Expect(result).To(Equal("gcr.io/project/image@" + digest1))

		// User requested quay.io/namespace/image:latest
		result = server.FindRepoDigestForImage(repoDigests, "quay.io/namespace/image:latest")
		Expect(result).To(Equal("quay.io/namespace/image@" + digest2))
	})

	It("should prefer exact match over repository match", func() {
		exactMatch := "docker.io/library/nginx@" + digest2
		repoDigests := []reference.Canonical{
			mustCanonical("docker.io/library/nginx@" + digest1),
			mustCanonical(exactMatch),
		}

		// When user requested the exact digest, return it even though there's another nginx digest
		result := server.FindRepoDigestForImage(repoDigests, exactMatch)
		Expect(result).To(Equal(exactMatch))
	})
})
