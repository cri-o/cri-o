package server_test

import (
	"context"
	"fmt"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
)

type livenessTask struct {
	task.TaskService

	err error
}

func (f *livenessTask) Kill(ctx context.Context, _ *task.KillRequest) (*emptypb.Empty, error) {
	if f.err != nil {
		return nil, f.err
	}

	<-ctx.Done()

	return nil, ctx.Err()
}

var _ = t.Describe("ContainerExec liveness", func() {
	for _, method := range []string{"ExecSync", "Exec"} {
		Context(method, func() {
			BeforeEach(func() {
				beforeEach()
				setupSUT()
				addContainerAndSandbox()
			})
			AfterEach(afterEach)

			exec := func(ctx context.Context) error {
				if method == "ExecSync" {
					_, err := sut.ExecSync(ctx, &types.ExecSyncRequest{
						ContainerId: testContainer.ID(), Cmd: []string{"true"},
					})

					return err
				}

				return testStreamService.Exec(
					ctx,
					testContainer.ID(),
					[]string{"true"},
					nil,
					nil,
					nil,
					false,
					nil,
				)
			}

			It("should reject a stopped pod workload with no runtime entries", func() {
				serverConfig.Runtimes[serverConfig.DefaultRuntime].RuntimeType = config.RuntimeTypePod

				testContainer.SetState(&oci.ContainerState{
					State: specs.State{Status: oci.ContainerStateStopped}, InitPid: 1,
				})
				Expect(testContainer.IsInfra()).To(BeFalse())
				Expect(testContainer.Living()).NotTo(Succeed())
				Expect(sut.Runtime().HasRuntimeImpl(testContainer.ID())).To(BeFalse())
				Expect(sut.Runtime().HasRuntimeImpl(testContainer.Sandbox())).To(BeFalse())

				err := exec(context.Background())
				Expect(status.Code(err)).To(Equal(codes.Unavailable))
				Expect(err.Error()).To(ContainSubstring("no runtime implementation"))
				Expect(sut.Runtime().HasRuntimeImpl(testContainer.ID())).To(BeFalse())
				Expect(sut.Runtime().HasRuntimeImpl(testContainer.Sandbox())).To(BeFalse())
			})

			It("should not recreate a runtime removed after the liveness check", func() {
				serverConfig.Runtimes[serverConfig.DefaultRuntime].RuntimeType = config.RuntimeTypePod

				ociRuntimeMock.EXPECT().
					ProbeMonitor(gomock.Any(), testContainer).
					Return(nil).
					AnyTimes()
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), testContainer).Return(nil)
				ociRuntimeMock.EXPECT().IsContainerAlive(gomock.Any(), testContainer).
					DoAndReturn(func(ctx context.Context, c *oci.Container) (bool, error) {
						Expect(sut.Runtime().DeleteContainer(ctx, c)).To(Succeed())

						return true, nil
					})
				sut.Runtime().SetRuntimeImpl(testContainer.ID(), ociRuntimeMock)

				err := exec(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no runtime implementation"))
				Expect(sut.Runtime().HasRuntimeImpl(testContainer.ID())).To(BeFalse())
			})

			DescribeTable(
				"should preserve VM liveness error status",
				func(taskErr error, code codes.Code) {
					vm := oci.NewRuntimeVMWithTask(&livenessTask{err: taskErr})
					sut.Runtime().SetRuntimeImpl(testContainer.ID(), vm)

					err := exec(context.Background())
					Expect(status.Code(err)).To(Equal(code))
					Expect(err.Error()).To(ContainSubstring(taskErr.Error()))
				},
				Entry("context deadline", context.DeadlineExceeded, codes.DeadlineExceeded),
				Entry(
					"wrapped deadline",
					fmt.Errorf("kill: %w", context.DeadlineExceeded),
					codes.DeadlineExceeded,
				),
				Entry(
					"gRPC deadline",
					status.Error(codes.DeadlineExceeded, "shim deadline"),
					codes.DeadlineExceeded,
				),
				Entry("context cancellation", context.Canceled, codes.Canceled),
				Entry(
					"gRPC cancellation",
					status.Error(codes.Canceled, "shim canceled"),
					codes.Canceled,
				),
			)

			It("should return DeadlineExceeded when the liveness context expires", func() {
				vm := oci.NewRuntimeVMWithTask(&livenessTask{})
				sut.Runtime().SetRuntimeImpl(testContainer.ID(), vm)

				ctx, cancel := context.WithDeadline(
					context.Background(),
					time.Now().Add(-time.Second),
				)
				defer cancel()

				Expect(status.Code(exec(ctx))).To(Equal(codes.DeadlineExceeded))
			})
		})
	}
})
