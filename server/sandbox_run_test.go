package server_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/containers/libpod/pkg/annotations"
	"github.com/cri-o/cri-o/internal/lib/config"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/pkg/storage"
	"github.com/cri-o/cri-o/server"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-tools/generate"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("RunPodSandbox", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("RunPodSandbox", func() {
		// TODO(sgrunert): refactor the internal function to reduce the
		// cyclomatic complexity and test it separately
		It("should fail when container creation errors", func() {
			// Given
			gomock.InOrder(
				runtimeServerMock.EXPECT().CreatePodSandbox(gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(storage.ContainerInfo{
						RunDir: "/tmp",
						Config: &v1.Image{Config: v1.ImageConfig{}},
					}, nil),
				runtimeServerMock.EXPECT().GetContainerMetadata(gomock.Any()).
					Return(storage.RuntimeContainerMetadata{}, nil),
				runtimeServerMock.EXPECT().SetContainerMetadata(gomock.Any(),
					gomock.Any()).Return(nil),
				runtimeServerMock.EXPECT().StartContainer(gomock.Any()).
					Return("", nil),
				runtimeServerMock.EXPECT().RemovePodSandbox(gomock.Any()).
					Return(nil),
			)

			// When
			response, err := sut.RunPodSandbox(context.Background(),
				&pb.RunPodSandboxRequest{Config: &pb.PodSandboxConfig{
					Metadata: &pb.PodSandboxMetadata{
						Name:      "name",
						Namespace: "default",
					},
					LogDirectory: "/tmp",
					Linux: &pb.LinuxPodSandboxConfig{
						SecurityContext: &pb.LinuxSandboxSecurityContext{
							NamespaceOptions: &pb.NamespaceOption{
								Ipc: pb.NamespaceMode_NODE,
							}}}}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when metadata is nil", func() {
			// Given
			// When
			response, err := sut.RunPodSandbox(context.Background(),
				&pb.RunPodSandboxRequest{Config: &pb.PodSandboxConfig{}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when metadata kubeName is nil", func() {
			// Given
			// When
			response, err := sut.RunPodSandbox(context.Background(),
				&pb.RunPodSandboxRequest{Config: &pb.PodSandboxConfig{
					Metadata: &pb.PodSandboxMetadata{},
				}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when metadata namespace is not provided", func() {
			// Given
			// When
			response, err := sut.RunPodSandbox(context.Background(),
				&pb.RunPodSandboxRequest{Config: &pb.PodSandboxConfig{
					Metadata: &pb.PodSandboxMetadata{
						Name: "name",
					},
				}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail with relative log path", func() {
			// Given
			gomock.InOrder(
				runtimeServerMock.EXPECT().CreatePodSandbox(gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(storage.ContainerInfo{}, nil),
				runtimeServerMock.EXPECT().RemovePodSandbox(gomock.Any()).
					Return(nil),
			)

			// When
			response, err := sut.RunPodSandbox(context.Background(),
				&pb.RunPodSandboxRequest{Config: &pb.PodSandboxConfig{
					Metadata: &pb.PodSandboxMetadata{
						Name:      "name",
						Namespace: "default",
					},
				}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("AddCgroupAnnotation", func() {
		var g generate.Generator

		BeforeEach(func() {
			// Given
			var err error
			g, err = generate.New("linux")
			Expect(err).To(BeNil())
		})

		It("should succeed with empty parent cgroup and manager", func() {
			// When
			res, err := server.AddCgroupAnnotation(context.Background(), g, "",
				"", "", "id")

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(""))
			Expect(g.Config.Annotations[annotations.CgroupParent]).To(BeEmpty())
		})

		It("should succeed with non-systemd manager", func() {
			// Given
			const cgroup = "someCgroup"

			// When
			res, err := server.AddCgroupAnnotation(context.Background(), g, "",
				"manager", cgroup, "id")

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(cgroup))
			Expect(g.Config.Annotations[annotations.CgroupParent]).To(Equal(cgroup))
			Expect(g.Config.Linux.CgroupsPath).To(HavePrefix(cgroup))
		})

		It("should succed with systemd manager", func() {
			// Given
			const cgroup = "some.slice"

			// When
			res, err := server.AddCgroupAnnotation(context.Background(), g, "",
				oci.SystemdCgroupsManager, cgroup, "id")

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(cgroup))
		})

		It("should fail with non-systemd manager but systemd slice", func() {
			// Given
			const cgroup = "some.slice"

			// When
			res, err := server.AddCgroupAnnotation(context.Background(), g, "",
				"manager", cgroup, "id")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
		})

		It("should fail with systemd manager on invalid slice", func() {
			// Given
			const cgroup = "someCgroup"

			// When
			res, err := server.AddCgroupAnnotation(context.Background(), g, "",
				oci.SystemdCgroupsManager, cgroup, "id")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
		})

		It("should fail with systemd manager if ExpandSlice fails", func() {
			// Given
			const cgroup = "some--wrong.slice"

			// When
			res, err := server.AddCgroupAnnotation(context.Background(), g, "",
				oci.SystemdCgroupsManager, cgroup, "id")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
		})

		var prepareCgroupDirs = func(content string) (string, string) {
			const cgroup = "some.slice"
			tmpDir := t.MustTempDir("cgroup")
			Expect(os.MkdirAll(filepath.Join(tmpDir, cgroup), 0755)).To(BeNil())
			Expect(ioutil.WriteFile(
				filepath.Join(tmpDir, cgroup, "memory.limit_in_bytes"),
				[]byte(content), 0644)).To(BeNil())
			return cgroup, tmpDir
		}

		It("should succeed with systemd manager if memory string empty", func() {
			// Given
			cgroup, tmpDir := prepareCgroupDirs("")

			// When
			res, err := server.AddCgroupAnnotation(context.Background(), g,
				tmpDir, oci.SystemdCgroupsManager, cgroup, "id")

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(cgroup))
		})

		It("should succeed with systemd manager with valid memory ", func() {
			// Given
			cgroup, tmpDir := prepareCgroupDirs("13000000")

			// When
			res, err := server.AddCgroupAnnotation(context.Background(), g,
				tmpDir, oci.SystemdCgroupsManager, cgroup, "id")

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(cgroup))
		})

		It("should fail with systemd manager with too low memory", func() {
			// Given
			cgroup, tmpDir := prepareCgroupDirs("10")

			// When
			res, err := server.AddCgroupAnnotation(context.Background(), g,
				tmpDir, oci.SystemdCgroupsManager, cgroup, "id")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
		})

		It("should fail with systemd manager with invalid memory ", func() {
			// Given
			cgroup, tmpDir := prepareCgroupDirs("invalid")

			// When
			res, err := server.AddCgroupAnnotation(context.Background(), g,
				tmpDir, oci.SystemdCgroupsManager, cgroup, "id")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
		})
	})

	t.Describe("PauseCommand", func() {
		var cfg *config.Config

		BeforeEach(func() {
			// Given
			var err error
			cfg, err = config.DefaultConfig()
			Expect(err).To(BeNil())
		})

		It("should succeed with default config", func() {
			// When
			res, err := server.PauseCommand(cfg, nil)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal([]string{sut.Config().PauseCommand}))
		})

		It("should succeed with Entrypoint", func() {
			// Given
			cfg.PauseCommand = ""
			entrypoint := []string{"/custom-pause"}
			image := &v1.Image{Config: v1.ImageConfig{Entrypoint: entrypoint}}

			// When
			res, err := server.PauseCommand(cfg, image)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(entrypoint))
		})

		It("should succeed with Cmd", func() {
			// Given
			cfg.PauseCommand = ""
			cmd := []string{"some-cmd"}
			image := &v1.Image{Config: v1.ImageConfig{Cmd: cmd}}

			// When
			res, err := server.PauseCommand(cfg, image)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(cmd))
		})

		It("should succeed with Entrypoint and Cmd", func() {
			// Given
			cfg.PauseCommand = ""
			entrypoint := "/custom-pause"
			cmd := "some-cmd"
			image := &v1.Image{Config: v1.ImageConfig{
				Entrypoint: []string{entrypoint},
				Cmd:        []string{cmd},
			}}

			// When
			res, err := server.PauseCommand(cfg, image)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(HaveLen(2))
			Expect(res[0]).To(Equal(entrypoint))
			Expect(res[1]).To(Equal(cmd))
		})

		It("should fail if config is nil", func() {
			// When
			res, err := server.PauseCommand(nil, nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail if image config is nil", func() {
			// Given
			cfg.PauseCommand = ""

			// When
			res, err := server.PauseCommand(cfg, nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})
	})
})
