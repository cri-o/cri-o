package server_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/cgroups"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	cstorage "go.podman.io/storage"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/memorystore"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/server"
	cgmgrmock "github.com/cri-o/cri-o/test/mocks/config/cgmgr"
)

// podCheckpointCgroup stands in for the Pod cgroup and saves the freezer
// states that CheckpointPod requests.
type podCheckpointCgroup struct {
	cgroups.Manager

	mutex     sync.Mutex
	state     cgroups.FreezerState
	stateErr  error
	freezeErr map[cgroups.FreezerState]error
	requested []cgroups.FreezerState

	// inspected runs when CheckpointPod reads the freezer state, after it
	// validated the output directory and before it writes to it.
	inspected func()
}

func (c *podCheckpointCgroup) Freeze(state cgroups.FreezerState) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.requested = append(c.requested, state)
	if err := c.freezeErr[state]; err != nil {
		return err
	}

	c.state = state

	return nil
}

func (c *podCheckpointCgroup) GetFreezerState() (cgroups.FreezerState, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.inspected != nil {
		c.inspected()
	}

	return c.state, c.stateErr
}

func (c *podCheckpointCgroup) freezeRequests() []cgroups.FreezerState {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return append([]cgroups.FreezerState(nil), c.requested...)
}

const (
	podCheckpointSandboxID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	podCheckpointParent    = "pod-checkpoint.slice"
	podCheckpointImageID   = "1111111111111111111111111111111111111111111111111111111111111111"
)

var _ = t.Describe("CheckpointPod backend", func() {
	var (
		cgroupManagerMock *cgmgrmock.MockCgroupManager
		podCgroup         *podCheckpointCgroup
		podSandbox        *sandbox.Sandbox
		graphRoot         string
		output            string
		appContainers     []*oci.Container
		registered        []*oci.Container
	)

	writeJSON := func(dir, name string, value any) {
		GinkgoHelper()

		data, err := json.Marshal(value)
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(dir, name), data, 0o600)).To(Succeed())
	}

	newPodCheckpointContainer := func(id, name, sandboxID string) *oci.Container {
		GinkgoHelper()

		dir := t.MustTempDir("pod-checkpoint-container")
		imageID, err := storage.ParseStorageImageIDFromOutOfProcessData(podCheckpointImageID)
		Expect(err).ToNot(HaveOccurred())

		container, err := oci.NewContainer(id, name, dir, "", nil, nil, nil, "image", nil,
			&imageID, "", &types.ContainerMetadata{Name: name}, sandboxID, false, false,
			false, "", dir, time.Now(), "")
		Expect(err).ToNot(HaveOccurred())

		writeJSON(dir, "config.json", &specs.Spec{Version: "1.0.0", Linux: &specs.Linux{}})
		writeJSON(dir, "container-config.json", &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: name},
			Image:    &types.ImageSpec{Image: "image"},
		})
		container.SetState(&oci.ContainerState{
			State: specs.State{Status: oci.ContainerStateRunning},
		})

		return container
	}

	addPodCheckpointSandbox := func(id, runtimeHandler string) *sandbox.Sandbox {
		GinkgoHelper()

		builder := sandbox.NewBuilder()
		builder.SetID(id)
		builder.SetName("pod-checkpoint-" + id[:8])
		builder.SetLogDir(t.MustTempDir("pod-checkpoint-log"))
		builder.SetShmPath("")
		builder.SetNamespace("default")
		builder.SetKubeName("pod")
		builder.SetMountLabel("")
		builder.SetProcessLabel("")
		builder.SetCgroupParent(podCheckpointParent)
		builder.SetRuntimeHandler(runtimeHandler)
		builder.SetResolvPath("")
		builder.SetHostname("")
		builder.SetPortMappings([]*hostport.PortMapping{})
		builder.SetHostNetwork(false)
		builder.SetUsernsMode("")
		builder.SetPodLinuxOverhead(nil)
		builder.SetPodLinuxResources(nil)
		builder.SetCreatedAt(time.Now())
		Expect(builder.SetCRISandbox(id, nil, nil, &types.PodSandboxMetadata{
			Name: "pod", Namespace: "default", Uid: "uid",
		})).To(Succeed())
		builder.SetPrivileged(false)
		builder.SetContainers(memorystore.New[*oci.Container]())
		sb, err := builder.GetSandbox()
		Expect(err).ToNot(HaveOccurred())

		infra := newPodCheckpointContainer(id, "infra", id)
		writeJSON(infra.Dir(), "pod-sandbox-config.json", &types.PodSandboxConfig{
			Metadata: &types.PodSandboxMetadata{Name: "pod", Namespace: "default", Uid: "uid"},
		})

		ctx := context.Background()
		Expect(sut.AddSandbox(ctx, sb)).To(Succeed())
		Expect(sb.SetInfraContainer(infra)).To(Succeed())
		Expect(sut.PodIDIndex().Add(id)).To(Succeed())
		sb.SetCreated()

		return sb
	}

	registerContainer := func(container *oci.Container) {
		GinkgoHelper()

		sut.AddContainer(context.Background(), container)
		Expect(sut.CtrIDIndex().Add(container.ID())).To(Succeed())
		sut.Runtime().SetRuntimeImpl(container.ID(), ociRuntimeMock)
		container.SetCreated()

		registered = append(registered, container)
	}

	addAppContainer := func(id, name string) *oci.Container {
		GinkgoHelper()

		container := newPodCheckpointContainer(id, name, podSandbox.ID())
		registerContainer(container)

		return container
	}

	podCheckpointRequest := func(ids ...string) *types.CheckpointPodRequest {
		return &types.CheckpointPodRequest{
			PodSandboxId: podSandbox.ID(),
			ContainerIds: ids,
			OutputPath:   output,
		}
	}

	checkpointPod := func(request *types.CheckpointPodRequest) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		_, err := server.CheckpointPodForTest(sut, ctx, request)

		return err
	}

	recoveryMarkerPath := func(sandboxID string) string {
		digest := sha256.Sum256([]byte(sandboxID))

		return filepath.Join(
			graphRoot,
			"crio",
			"pod-checkpoint-recovery",
			hex.EncodeToString(digest[:])+".json",
		)
	}

	setStatus := func(state string) func(context.Context, *oci.Container) error {
		return func(_ context.Context, c *oci.Container) error {
			c.SetState(&oci.ContainerState{State: specs.State{Status: specs.ContainerState(state)}})

			return nil
		}
	}

	expectContainerStatusUpdates := func() {
		ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), gomock.Any()).
			Return(nil).AnyTimes()
	}

	expectPause := func() {
		ociRuntimeMock.EXPECT().PauseContainer(gomock.Any(), gomock.Any()).
			DoAndReturn(setStatus(oci.ContainerStatePaused)).AnyTimes()
	}

	expectUnpause := func() {
		ociRuntimeMock.EXPECT().UnpauseContainer(gomock.Any(), gomock.Any()).
			DoAndReturn(setStatus(oci.ContainerStateRunning)).AnyTimes()
	}

	expectStorage := func() {
		storeMock.EXPECT().Container(gomock.Any()).
			Return(&cstorage.Container{LayerID: "layer"}, nil).AnyTimes()
		storeMock.EXPECT().Changes(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		imageServerMock.EXPECT().GetStore().Return(storeMock).AnyTimes()
		storeMock.EXPECT().Mount(gomock.Any(), gomock.Any()).
			Return(t.MustTempDir("pod-checkpoint-rootfs"), nil).AnyTimes()
	}

	expectCriuDump := func() {
		ociRuntimeMock.EXPECT().
			CheckpointContainer(gomock.Any(), gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, c *oci.Container, _ *specs.Spec, _ bool) error {
				Expect(os.MkdirAll(c.CheckpointPath(), 0o700)).To(Succeed())
				Expect(os.WriteFile(
					filepath.Join(c.CheckpointPath(), "inventory.img"),
					nil,
					0o600,
				)).To(Succeed())

				return os.WriteFile(
					filepath.Join(c.CheckpointPath(), "pages-1.img"),
					[]byte(c.ID()),
					0o600,
				)
			}).AnyTimes()
	}

	outputEntries := func() []string {
		GinkgoHelper()

		entries, err := os.ReadDir(output)
		Expect(err).ToNot(HaveOccurred())

		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}

		return names
	}

	containerStatus := func(c *oci.Container) string {
		return string(c.State().Status)
	}

	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuntimeInLibConfig()
		serverConfig.SetCheckpointRestore(true)
		serverConfig.Runtimes[config.DefaultRuntime].RuntimeType = config.DefaultRuntimeType
		serverConfig.Runtimes["kata"] = &config.RuntimeHandler{
			RuntimePath: serverConfig.Runtimes[config.DefaultRuntime].RuntimePath,
			RuntimeType: config.RuntimeTypeVM,
		}

		podCgroup = &podCheckpointCgroup{
			state:     cgroups.Thawed,
			freezeErr: map[cgroups.FreezerState]error{},
		}
		cgroupManagerMock = cgmgrmock.NewMockCgroupManager(mockCtrl)
		serverConfig.SetCgroupManager(cgroupManagerMock)
		setupSUT()

		graphRoot = t.MustTempDir("pod-checkpoint-graphroot")
		storeMock.EXPECT().GraphRoot().Return(graphRoot).AnyTimes()
		cgroupManagerMock.EXPECT().
			SandboxCgroupManager(podCheckpointParent, podCheckpointSandboxID).
			Return(podCgroup, nil).AnyTimes()

		output = t.MustTempDir("pod-checkpoint-output")
		podSandbox = addPodCheckpointSandbox(podCheckpointSandboxID, "")
		appContainers = []*oci.Container{
			addAppContainer(strings.Repeat("a", 64), "app"),
			addAppContainer(strings.Repeat("b", 64), "sidecar"),
		}
	})

	AfterEach(func() {
		// The server keeps probing the monitors of its containers after the
		// spec, and the runtime mock no longer accepts calls then.
		for _, container := range registered {
			sut.ContainerServer.RemoveContainer(context.Background(), container)
		}

		registered = nil

		afterEach()
	})

	It("should write a manifest and archives for every container", func() {
		expectContainerStatusUpdates()
		expectPause()
		expectUnpause()
		expectStorage()
		expectCriuDump()

		Expect(checkpointPod(podCheckpointRequest(
			appContainers[0].ID(),
			appContainers[1].ID(),
		))).To(Succeed())

		data, err := os.ReadFile(filepath.Join(output, "checkpoint-manifest.json"))
		Expect(err).ToNot(HaveOccurred())

		var manifest struct {
			Version        int    `json:"version"`
			Runtime        string `json:"runtime"`
			RuntimeHandler string `json:"runtimeHandler"`
			SandboxID      string `json:"sandboxId"`
			Containers     []struct {
				Name    string                 `json:"name"`
				ID      string                 `json:"id"`
				Archive string                 `json:"archive"`
				SHA256  string                 `json:"sha256"`
				Config  *types.ContainerConfig `json:"config"`
				ImageID string                 `json:"imageId"`
			} `json:"containers"`
		}
		Expect(json.Unmarshal(data, &manifest)).To(Succeed())
		Expect(manifest.Version).To(Equal(1))
		Expect(manifest.Runtime).To(Equal("cri-o"))
		Expect(manifest.RuntimeHandler).To(Equal(serverConfig.DefaultRuntime))
		Expect(manifest.SandboxID).To(Equal(podSandbox.ID()))
		Expect(manifest.Containers).To(HaveLen(2))

		for i, entry := range manifest.Containers {
			Expect(entry.ID).To(Equal(appContainers[i].ID()))
			Expect(entry.Name).To(Equal(appContainers[i].Name()))
			Expect(entry.Config.GetMetadata().GetName()).To(Equal(entry.Name))
			Expect(entry.ImageID).To(Equal(podCheckpointImageID))

			archive, err := os.ReadFile(filepath.Join(output, entry.Archive))
			Expect(err).ToNot(HaveOccurred())

			digest := sha256.Sum256(archive)
			Expect(entry.SHA256).To(Equal(hex.EncodeToString(digest[:])))
		}

		Expect(outputEntries()).To(ConsistOf(
			"checkpoint-manifest.json",
			"containers",
			"pod-config.json",
		))
		Expect(podCgroup.freezeRequests()).To(Equal([]cgroups.FreezerState{
			cgroups.Frozen, cgroups.Thawed, cgroups.Thawed,
		}))

		for _, container := range appContainers {
			Expect(containerStatus(container)).To(Equal(oci.ContainerStateRunning))
			Expect(container.CheckpointPath()).ToNot(BeADirectory())
		}

		Expect(recoveryMarkerPath(podSandbox.ID())).ToNot(BeAnExistingFile())
	})

	It("should resume a Pod left paused by an interrupted checkpoint", func() {
		storeMock.EXPECT().Container("unknown-container").
			Return(nil, cstorage.ErrContainerUnknown)
		expectContainerStatusUpdates()
		expectPause()
		expectUnpause()
		expectStorage()
		expectCriuDump()

		Expect(os.MkdirAll(filepath.Dir(recoveryMarkerPath(podSandbox.ID())), 0o700)).To(Succeed())
		writeJSON(
			filepath.Dir(recoveryMarkerPath(podSandbox.ID())),
			filepath.Base(recoveryMarkerPath(podSandbox.ID())),
			map[string]any{
				"version":      1,
				"sandboxId":    podSandbox.ID(),
				"cgroupParent": podCheckpointParent,
				"containerIds": []string{appContainers[0].ID(), "unknown-container"},
			},
		)
		appContainers[0].SetState(&oci.ContainerState{
			State: specs.State{Status: oci.ContainerStatePaused},
		})

		podCgroup.state = cgroups.Frozen

		Expect(checkpointPod(podCheckpointRequest(appContainers[0].ID()))).To(Succeed())

		Expect(podCgroup.freezeRequests()[0]).To(Equal(cgroups.Thawed))
		Expect(containerStatus(appContainers[0])).To(Equal(oci.ContainerStateRunning))
		Expect(recoveryMarkerPath(podSandbox.ID())).ToNot(BeAnExistingFile())
		Expect(filepath.Join(output, "checkpoint-manifest.json")).To(BeAnExistingFile())
	})

	It("should refuse a recovery marker that does not match the sandbox cgroup", func() {
		Expect(os.MkdirAll(filepath.Dir(recoveryMarkerPath(podSandbox.ID())), 0o700)).To(Succeed())
		writeJSON(
			filepath.Dir(recoveryMarkerPath(podSandbox.ID())),
			filepath.Base(recoveryMarkerPath(podSandbox.ID())),
			map[string]any{
				"version":      1,
				"sandboxId":    podSandbox.ID(),
				"cgroupParent": "other.slice",
				"containerIds": []string{appContainers[0].ID()},
			},
		)

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("cgroup does not match")))
		Expect(recoveryMarkerPath(podSandbox.ID())).To(BeAnExistingFile())
	})

	It("should refuse a recovery marker that names a container of another sandbox", func() {
		otherSandbox := addPodCheckpointSandbox(strings.Repeat("c", 64), "")
		other := newPodCheckpointContainer(strings.Repeat("d", 64), "other", otherSandbox.ID())
		registerContainer(other)

		Expect(os.MkdirAll(filepath.Dir(recoveryMarkerPath(podSandbox.ID())), 0o700)).To(Succeed())
		writeJSON(
			filepath.Dir(recoveryMarkerPath(podSandbox.ID())),
			filepath.Base(recoveryMarkerPath(podSandbox.ID())),
			map[string]any{
				"version":      1,
				"sandboxId":    podSandbox.ID(),
				"cgroupParent": podCheckpointParent,
				"containerIds": []string{other.ID()},
			},
		)

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("container from another sandbox")))
	})

	It("should refuse to recover a container that storage has not loaded", func() {
		Expect(os.MkdirAll(filepath.Dir(recoveryMarkerPath(podSandbox.ID())), 0o700)).To(Succeed())
		writeJSON(
			filepath.Dir(recoveryMarkerPath(podSandbox.ID())),
			filepath.Base(recoveryMarkerPath(podSandbox.ID())),
			map[string]any{
				"version":      1,
				"sandboxId":    podSandbox.ID(),
				"cgroupParent": podCheckpointParent,
				"containerIds": []string{"unloaded"},
			},
		)
		storeMock.EXPECT().Container("unloaded").Return(&cstorage.Container{}, nil)

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring(`"unloaded" has not been loaded`)))
	})

	It("should return storage errors while recovering a missing container", func() {
		Expect(os.MkdirAll(filepath.Dir(recoveryMarkerPath(podSandbox.ID())), 0o700)).To(Succeed())
		writeJSON(
			filepath.Dir(recoveryMarkerPath(podSandbox.ID())),
			filepath.Base(recoveryMarkerPath(podSandbox.ID())),
			map[string]any{
				"version":      1,
				"sandboxId":    podSandbox.ID(),
				"cgroupParent": podCheckpointParent,
				"containerIds": []string{"broken"},
			},
		)
		storeMock.EXPECT().Container("broken").Return(nil, errors.New("storage failed"))

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("storage failed")))
	})

	It("should resume the Pod and remove partial output when pausing fails", func() {
		expectContainerStatusUpdates()
		expectUnpause()
		gomock.InOrder(
			ociRuntimeMock.EXPECT().PauseContainer(gomock.Any(), appContainers[0]).
				DoAndReturn(setStatus(oci.ContainerStatePaused)),
			ociRuntimeMock.EXPECT().PauseContainer(gomock.Any(), appContainers[1]).
				Return(errors.New("pause failed")),
		)

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID(), appContainers[1].ID()))
		Expect(err).To(MatchError(ContainSubstring("pause failed")))

		Expect(outputEntries()).To(BeEmpty())
		Expect(containerStatus(appContainers[0])).To(Equal(oci.ContainerStateRunning))
		Expect(podCgroup.freezeRequests()).To(Equal([]cgroups.FreezerState{
			cgroups.Frozen, cgroups.Thawed,
		}))
		Expect(recoveryMarkerPath(podSandbox.ID())).ToNot(BeAnExistingFile())
	})

	It("should join recovery errors to the checkpoint error", func() {
		expectContainerStatusUpdates()
		ociRuntimeMock.EXPECT().PauseContainer(gomock.Any(), gomock.Any()).
			DoAndReturn(setStatus(oci.ContainerStatePaused))
		ociRuntimeMock.EXPECT().UnpauseContainer(gomock.Any(), gomock.Any()).
			Return(errors.New("unpause failed"))
		podCgroup.freezeErr[cgroups.Thawed] = errors.New("thaw failed")

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("thaw Pod cgroup after pausing containers")))
		Expect(err).To(MatchError(ContainSubstring("unpause failed")))
		Expect(recoveryMarkerPath(podSandbox.ID())).To(BeAnExistingFile())
	})

	It("should fail when a container is not paused after pausing it", func() {
		expectContainerStatusUpdates()
		expectUnpause()
		ociRuntimeMock.EXPECT().PauseContainer(gomock.Any(), gomock.Any()).Return(nil)

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("expected paused")))
		Expect(outputEntries()).To(BeEmpty())
	})

	It("should fail when the paused state cannot be verified", func() {
		ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), gomock.Any()).Return(nil)
		ociRuntimeMock.EXPECT().PauseContainer(gomock.Any(), gomock.Any()).
			DoAndReturn(setStatus(oci.ContainerStatePaused))
		ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), gomock.Any()).
			Return(errors.New("state unavailable"))
		ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), gomock.Any()).
			Return(nil).AnyTimes()
		expectUnpause()

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("verify paused container")))
		Expect(containerStatus(appContainers[0])).To(Equal(oci.ContainerStateRunning))
	})

	It("should resume the Pod when CRIU fails", func() {
		expectContainerStatusUpdates()
		expectPause()
		expectUnpause()
		ociRuntimeMock.EXPECT().
			CheckpointContainer(gomock.Any(), gomock.Any(), gomock.Any(), true).
			Return(errors.New("criu failed"))

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("criu failed")))
		Expect(outputEntries()).To(BeEmpty())
		Expect(containerStatus(appContainers[0])).To(Equal(oci.ContainerStateRunning))
		Expect(appContainers[0].CheckpointPath()).ToNot(BeADirectory())
	})

	It("should stop and recover when the request is canceled", func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		expectContainerStatusUpdates()
		expectUnpause()
		ociRuntimeMock.EXPECT().PauseContainer(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, c *oci.Container) error {
				cancel()

				return setStatus(oci.ContainerStatePaused)(ctx, c)
			})

		_, err := server.CheckpointPodForTest(sut, ctx, podCheckpointRequest(
			appContainers[0].ID(),
			appContainers[1].ID(),
		))
		Expect(err).To(MatchError(context.Canceled))
		Expect(outputEntries()).To(BeEmpty())
		Expect(containerStatus(appContainers[0])).To(Equal(oci.ContainerStateRunning))
		Expect(recoveryMarkerPath(podSandbox.ID())).ToNot(BeAnExistingFile())
	})

	It("should fail when resuming the Pod after CRIU fails", func() {
		expectContainerStatusUpdates()
		expectPause()
		expectStorage()
		expectCriuDump()
		ociRuntimeMock.EXPECT().UnpauseContainer(gomock.Any(), gomock.Any()).
			Return(errors.New("unpause failed")).AnyTimes()

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("resume Pod after checkpoint")))
		Expect(recoveryMarkerPath(podSandbox.ID())).To(BeAnExistingFile())
	})

	It("should reject a Pod cgroup that is not thawed", func() {
		expectContainerStatusUpdates()

		podCgroup.state = cgroups.Frozen

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
		Expect(err).To(MatchError(ContainSubstring("expected THAWED")))
		Expect(recoveryMarkerPath(podSandbox.ID())).ToNot(BeAnExistingFile())
	})

	It("should return freezer and cgroup errors", func() {
		expectContainerStatusUpdates()

		podCgroup.stateErr = errors.New("freezer unavailable")

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("freezer unavailable")))

		podCgroup.stateErr = nil
		podCgroup.freezeErr[cgroups.Frozen] = errors.New("freeze failed")

		err = checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("freeze failed")))
		Expect(outputEntries()).To(BeEmpty())
	})

	It("should stop and recover when the request is canceled during CRIU", func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		expectContainerStatusUpdates()
		expectPause()
		expectUnpause()
		expectStorage()
		ociRuntimeMock.EXPECT().
			CheckpointContainer(gomock.Any(), gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, c *oci.Container, _ *specs.Spec, _ bool) error {
				cancel()

				return os.MkdirAll(c.CheckpointPath(), 0o700)
			})

		_, err := server.CheckpointPodForTest(sut, ctx, podCheckpointRequest(
			appContainers[0].ID(),
			appContainers[1].ID(),
		))
		Expect(err).To(MatchError(context.Canceled))
		Expect(outputEntries()).To(BeEmpty())
		Expect(appContainers[0].CheckpointPath()).ToNot(BeADirectory())

		for _, container := range appContainers {
			Expect(containerStatus(container)).To(Equal(oci.ContainerStateRunning))
		}
	})

	It("should remove unpublished archives when the request is canceled after resuming", func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		expectContainerStatusUpdates()
		expectPause()
		expectStorage()
		expectCriuDump()
		ociRuntimeMock.EXPECT().UnpauseContainer(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, c *oci.Container) error {
				cancel()

				return setStatus(oci.ContainerStateRunning)(ctx, c)
			})

		_, err := server.CheckpointPodForTest(sut, ctx, podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(context.Canceled))
		Expect(outputEntries()).To(BeEmpty())
		Expect(containerStatus(appContainers[0])).To(Equal(oci.ContainerStateRunning))
		Expect(recoveryMarkerPath(podSandbox.ID())).ToNot(BeAnExistingFile())
	})

	It("should remove unpublished archives when the export fails", func() {
		expectContainerStatusUpdates()
		expectPause()
		expectUnpause()
		expectStorage()
		ociRuntimeMock.EXPECT().
			CheckpointContainer(gomock.Any(), gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, c *oci.Container, _ *specs.Spec, _ bool) error {
				Expect(os.MkdirAll(c.CheckpointPath(), 0o700)).To(Succeed())

				// Export opens the temporary archive after the Pod resumed.
				digest := sha256.Sum256([]byte(c.ID()))
				partial := filepath.Join(
					output,
					"containers",
					hex.EncodeToString(digest[:])+".tar.partial",
				)

				return os.Mkdir(partial, 0o700)
			})

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("export checkpoint for container")))
		Expect(err).To(MatchError(ContainSubstring("error creating checkpoint export file")))
		Expect(outputEntries()).To(BeEmpty())
		Expect(recoveryMarkerPath(podSandbox.ID())).ToNot(BeAnExistingFile())
	})

	It("should remove unpublished archives when an archive cannot be published", func() {
		expectContainerStatusUpdates()
		expectPause()
		expectUnpause()
		expectStorage()
		ociRuntimeMock.EXPECT().
			CheckpointContainer(gomock.Any(), gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, c *oci.Container, _ *specs.Spec, _ bool) error {
				Expect(os.MkdirAll(c.CheckpointPath(), 0o700)).To(Succeed())
				Expect(os.WriteFile(
					filepath.Join(c.CheckpointPath(), "inventory.img"),
					nil,
					0o600,
				)).To(Succeed())

				archives, err := filepath.Glob(filepath.Join(output, "containers"))
				Expect(err).ToNot(HaveOccurred())
				Expect(archives).To(HaveLen(1))

				digest := sha256.Sum256([]byte(c.ID()))

				occupied := filepath.Join(archives[0], hex.EncodeToString(digest[:])+".tar")
				Expect(os.MkdirAll(filepath.Join(occupied, "entry"), 0o700)).To(Succeed())

				return nil
			})

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("publish checkpoint archive")))
		Expect(outputEntries()).To(BeEmpty())
	})

	It("should not publish an archive without CRIU images", func() {
		expectContainerStatusUpdates()
		expectPause()
		expectUnpause()
		expectStorage()
		ociRuntimeMock.EXPECT().
			CheckpointContainer(gomock.Any(), gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, c *oci.Container, _ *specs.Spec, _ bool) error {
				return os.MkdirAll(c.CheckpointPath(), 0o700)
			})

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("checkpoint archive is missing")))
		Expect(outputEntries()).To(BeEmpty())
		Expect(containerStatus(appContainers[0])).To(Equal(oci.ContainerStateRunning))
	})

	It("should refuse other checkpoints while the Pod is checkpointed", func() {
		expectContainerStatusUpdates()

		podCgroup.inspected = func() {
			podCgroup.inspected = nil

			_, err := sut.CheckpointContainer(
				context.Background(),
				&types.CheckpointContainerRequest{ContainerId: appContainers[0].ID()},
			)
			Expect(status.Code(err)).To(Equal(codes.Aborted))

			request := podCheckpointRequest(appContainers[1].ID())
			request.OutputPath = t.MustTempDir("pod-checkpoint-concurrent")
			Expect(status.Code(checkpointPod(request))).To(Equal(codes.Aborted))
		}
		podCgroup.stateErr = errors.New("stop after the concurrent requests")

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("stop after the concurrent requests")))
	})

	It("should fail when the output changes before it is written", func() {
		expectContainerStatusUpdates()

		podCgroup.inspected = func() {
			Expect(os.Mkdir(filepath.Join(output, "containers"), 0o700)).To(Succeed())
		}

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(err).To(MatchError(ContainSubstring("create checkpoint container directory")))
		Expect(outputEntries()).To(BeEmpty())
		Expect(recoveryMarkerPath(podSandbox.ID())).ToNot(BeAnExistingFile())
	})

	It("should report containers that recovery cannot inspect or save", func() {
		gomock.InOrder(
			// Look up both containers and verify that the first one paused.
			ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), gomock.Any()).
				Return(nil).Times(3),
			ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), appContainers[0]).
				Return(errors.New("state unavailable")),
			ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), gomock.Any()).
				Return(nil).AnyTimes(),
		)
		gomock.InOrder(
			ociRuntimeMock.EXPECT().PauseContainer(gomock.Any(), appContainers[0]).
				DoAndReturn(setStatus(oci.ContainerStatePaused)),
			ociRuntimeMock.EXPECT().PauseContainer(gomock.Any(), appContainers[1]).
				DoAndReturn(func(_ context.Context, c *oci.Container) error {
					Expect(os.RemoveAll(c.Dir())).To(Succeed())

					return errors.New("pause failed")
				}),
		)

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID(), appContainers[1].ID()))
		Expect(err).To(MatchError(ContainSubstring("pause failed")))
		Expect(err).To(MatchError(ContainSubstring("inspect container")))
		Expect(err).To(MatchError(ContainSubstring("persist recovered container")))
		Expect(recoveryMarkerPath(podSandbox.ID())).To(BeAnExistingFile())
	})

	It("should keep the recovery marker when the Pod cgroup cannot be loaded", func() {
		other := addPodCheckpointSandbox(strings.Repeat("4", 64), "")
		cgroupManagerMock.EXPECT().SandboxCgroupManager(podCheckpointParent, other.ID()).
			Return(nil, errors.New("no cgroup"))

		Expect(os.MkdirAll(filepath.Dir(recoveryMarkerPath(other.ID())), 0o700)).To(Succeed())
		writeJSON(
			filepath.Dir(recoveryMarkerPath(other.ID())),
			filepath.Base(recoveryMarkerPath(other.ID())),
			map[string]any{
				"version":      1,
				"sandboxId":    other.ID(),
				"cgroupParent": podCheckpointParent,
				"containerIds": []string{"container"},
			},
		)

		err := checkpointPod(&types.CheckpointPodRequest{
			PodSandboxId: other.ID(),
			ContainerIds: []string{"container"},
			OutputPath:   output,
		})
		Expect(err).To(MatchError(ContainSubstring("recover previous Pod checkpoint")))
		Expect(err).To(MatchError(ContainSubstring("no cgroup")))
		Expect(recoveryMarkerPath(other.ID())).To(BeAnExistingFile())
	})

	It("should return an error when the sandbox cgroup cannot be loaded", func() {
		other := addPodCheckpointSandbox(strings.Repeat("e", 64), "")
		container := newPodCheckpointContainer(strings.Repeat("f", 64), "app", other.ID())
		registerContainer(container)
		expectContainerStatusUpdates()
		cgroupManagerMock.EXPECT().SandboxCgroupManager(podCheckpointParent, other.ID()).
			Return(nil, errors.New("no cgroup"))

		err := checkpointPod(&types.CheckpointPodRequest{
			PodSandboxId: other.ID(),
			ContainerIds: []string{container.ID()},
			OutputPath:   output,
		})
		Expect(err).To(MatchError(ContainSubstring("no cgroup")))
	})

	It("should reject a stopped sandbox", func() {
		expectContainerStatusUpdates()
		podSandbox.SetStopped(context.Background(), false)

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
		Expect(err).To(MatchError(ContainSubstring("is stopped")))
	})

	It("should reject a sandbox without saved checkpoint metadata", func() {
		expectContainerStatusUpdates()
		Expect(os.Remove(filepath.Join(
			podSandbox.InfraContainer().Dir(),
			"pod-sandbox-config.json",
		))).To(Succeed())

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
		Expect(err).To(MatchError(ContainSubstring("load sandbox checkpoint metadata")))
	})

	DescribeTable("should reject invalid requests",
		func(change func(*types.CheckpointPodRequest), code codes.Code, message string) {
			ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), gomock.Any()).
				Return(nil).AnyTimes()

			request := podCheckpointRequest(appContainers[0].ID())
			change(request)

			err := checkpointPod(request)
			Expect(status.Code(err)).To(Equal(code))
			Expect(err).To(MatchError(ContainSubstring(message)))
			Expect(outputEntries()).To(BeEmpty())
		},
		Entry("relative output", func(r *types.CheckpointPodRequest) {
			r.OutputPath = "relative"
		}, codes.InvalidArgument, "invalid checkpoint output"),
		Entry("missing sandbox", func(r *types.CheckpointPodRequest) {
			r.PodSandboxId = ""
		}, codes.InvalidArgument, "pod sandbox ID is required"),
		Entry("no containers", func(r *types.CheckpointPodRequest) {
			r.ContainerIds = nil
		}, codes.InvalidArgument, "between 1 and 1024"),
		Entry("too many containers", func(r *types.CheckpointPodRequest) {
			r.ContainerIds = make([]string, 1025)
		}, codes.InvalidArgument, "between 1 and 1024"),
		Entry("unknown sandbox", func(r *types.CheckpointPodRequest) {
			r.PodSandboxId = "unknown"
		}, codes.NotFound, "find pod sandbox"),
		Entry("short sandbox ID", func(r *types.CheckpointPodRequest) {
			r.PodSandboxId = podCheckpointSandboxID[:12]
		}, codes.InvalidArgument, "is not canonical"),
		Entry("empty container ID", func(r *types.CheckpointPodRequest) {
			r.ContainerIds = []string{""}
		}, codes.InvalidArgument, "must not be empty"),
		Entry("duplicate container ID", func(r *types.CheckpointPodRequest) {
			r.ContainerIds = append(r.GetContainerIds(), r.GetContainerIds()[0])
		}, codes.InvalidArgument, "is duplicated"),
		Entry("unknown container", func(r *types.CheckpointPodRequest) {
			r.ContainerIds = []string{"unknown"}
		}, codes.NotFound, "find container"),
		Entry("short container ID", func(r *types.CheckpointPodRequest) {
			r.ContainerIds = []string{strings.Repeat("a", 12)}
		}, codes.InvalidArgument, "is not canonical"),
	)

	It("should reject a non-empty output directory", func() {
		expectContainerStatusUpdates()
		Expect(os.WriteFile(filepath.Join(output, "existing"), nil, 0o600)).To(Succeed())

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		Expect(err).To(MatchError(ContainSubstring("must be empty")))
	})

	It("should reject a sandbox with an unsupported runtime type", func() {
		other := addPodCheckpointSandbox(strings.Repeat("9", 64), "kata")

		err := checkpointPod(&types.CheckpointPodRequest{
			PodSandboxId: other.ID(),
			ContainerIds: []string{"container"},
			OutputPath:   output,
		})
		Expect(status.Code(err)).To(Equal(codes.Unimplemented))
	})

	It("should reject a sandbox with an unknown runtime handler", func() {
		other := addPodCheckpointSandbox(strings.Repeat("8", 64), "missing")

		err := checkpointPod(&types.CheckpointPodRequest{
			PodSandboxId: other.ID(),
			ContainerIds: []string{"container"},
			OutputPath:   output,
		})
		Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
		Expect(err).To(MatchError(ContainSubstring("resolve runtime handler")))
	})

	DescribeTable("should reject containers that cannot be checkpointed",
		func(prepare func(*oci.Container), code codes.Code, message string) {
			ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), gomock.Any()).
				Return(nil).AnyTimes()
			prepare(appContainers[1])

			err := checkpointPod(podCheckpointRequest(appContainers[0].ID(), appContainers[1].ID()))
			Expect(status.Code(err)).To(Equal(code))
			Expect(err).To(MatchError(ContainSubstring(message)))
			Expect(outputEntries()).To(BeEmpty())
		},
		Entry("stopped container", func(c *oci.Container) {
			c.SetState(&oci.ContainerState{State: specs.State{Status: oci.ContainerStateStopped}})
		}, codes.FailedPrecondition, "expected running"),
		Entry("missing CRI config", func(c *oci.Container) {
			Expect(os.Remove(filepath.Join(c.Dir(), "container-config.json"))).To(Succeed())
		}, codes.FailedPrecondition, "load checkpoint metadata"),
		Entry("unnamed CRI config", func(c *oci.Container) {
			writeJSON(c.Dir(), "container-config.json", &types.ContainerConfig{})
		}, codes.FailedPrecondition, "has no checkpoint metadata name"),
		Entry("duplicate CRI config name", func(c *oci.Container) {
			writeJSON(c.Dir(), "container-config.json", &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "app"},
			})
		}, codes.FailedPrecondition, "multiple containers"),
	)

	It("should reject a container from another sandbox", func() {
		other := addPodCheckpointSandbox(strings.Repeat("7", 64), "")
		container := newPodCheckpointContainer(strings.Repeat("6", 64), "other", other.ID())
		registerContainer(container)

		err := checkpointPod(podCheckpointRequest(container.ID()))
		Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		Expect(err).To(MatchError(ContainSubstring("does not belong")))
	})

	It("should reject a container without a base image ID", func() {
		container, err := oci.NewContainer(strings.Repeat("5", 64), "noimage", "", "", nil,
			nil, nil, "image", nil, nil, "", &types.ContainerMetadata{Name: "noimage"},
			podSandbox.ID(), false, false, false, "", t.MustTempDir("noimage"), time.Now(), "")
		Expect(err).ToNot(HaveOccurred())
		container.SetState(&oci.ContainerState{
			State: specs.State{Status: oci.ContainerStateRunning},
		})
		registerContainer(container)
		expectContainerStatusUpdates()

		err = checkpointPod(podCheckpointRequest(container.ID()))
		Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
		Expect(err).To(MatchError(ContainSubstring("has no base image ID")))
	})

	It("should return container status errors", func() {
		ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), gomock.Any()).
			Return(errors.New("state unavailable"))

		err := checkpointPod(podCheckpointRequest(appContainers[0].ID()))
		Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
		Expect(err).To(MatchError(ContainSubstring("state unavailable")))
	})
})
