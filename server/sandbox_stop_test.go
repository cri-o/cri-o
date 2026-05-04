package server_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
)

// The actual test suite.
var _ = t.Describe("PodSandboxStatus", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("PodSandboxStatus", func() {
		It("should succeed with already stopped sandbox", func() {
			ctx := context.TODO()
			// Given
			addContainerAndSandbox()
			testSandbox.SetStopped(ctx, false)
			Expect(testSandbox.SetNetworkStopped(ctx, true)).To(Succeed())

			// When
			_, err := sut.StopPodSandbox(context.Background(),
				&types.StopPodSandboxRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		DescribeTable("should run the post-stop cleanup of an exited container only if it is pending",
			func(postStopDone bool) {
				ctx := context.TODO()
				// Given
				addContainerAndSandbox()
				Expect(testSandbox.SetNetworkStopped(ctx, true)).To(Succeed())
				// Unmounting a private shm requires root.
				testSandbox.SetShmPath(sandbox.DevShmPath)

				ctr, err := oci.NewContainer("exited", "", "", "",
					make(map[string]string), make(map[string]string),
					make(map[string]string), "", nil, nil, "",
					&types.ContainerMetadata{}, testSandbox.ID(), false, false,
					false, "", "", time.Now(), "")
				Expect(err).ToNot(HaveOccurred())
				ctr.SetState(&oci.ContainerState{
					State:    specs.State{Status: oci.ContainerStateStopped},
					ExitCode: new(int32(0)),
				})

				if postStopDone {
					ctr.SetPostStopDone()
				}

				sut.AddContainer(ctx, ctr)
				sut.Runtime().SetRuntimeImplForContainer(ctr, ociRuntimeMock)
				DeferCleanup(func() {
					sut.Runtime().SetRuntimeImplForContainer(ctr, nil)
				})
				ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				// The infra container is always stopped last.
				sut.Runtime().SetRuntimeImplForContainer(testContainer, ociRuntimeMock)
				ociRuntimeMock.EXPECT().StopContainer(gomock.Any(), testContainer, gomock.Any()).
					Return(nil).AnyTimes()
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), testContainer).
					Return(nil).AnyTimes()
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), testContainer).
					Return(nil).AnyTimes()
				runtimeServerMock.EXPECT().StopContainer(gomock.Any(), testContainer.ID()).
					Return(nil).AnyTimes()

				// The exited container is not stopped again, only its post-stop
				// cleanup runs if it is pending.
				if !postStopDone {
					runtimeServerMock.EXPECT().StopContainer(gomock.Any(), ctr.ID()).Return(nil)
					ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), ctr).Return(nil)
					ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), ctr).Return(nil)
				}

				// When
				_, err = sut.StopPodSandbox(context.Background(),
					&types.StopPodSandboxRequest{PodSandboxId: testSandbox.ID()})

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(ctr.PostStopDone()).To(BeTrue())
			},
			Entry("pending", false),
			Entry("done", true),
		)

		It("should succeed with inavailable sandbox", func() {
			// Given
			// When
			_, err := sut.StopPodSandbox(context.Background(),
				&types.StopPodSandboxRequest{PodSandboxId: "invalid"})

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail with empty sandbox ID", func() {
			// Given
			// When
			_, err := sut.StopPodSandbox(context.Background(),
				&types.StopPodSandboxRequest{})

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
})
