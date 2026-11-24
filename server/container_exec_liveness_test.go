package server_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
)

var _ = t.Describe("Container exec eligibility", func() {
	for _, method := range []string{"ExecSync", "Exec"} {
		Context(method, func() {
			BeforeEach(func() {
				beforeEach()
				setupSUT()
				addContainerAndSandbox()
			})
			AfterEach(afterEach)

			mockExec := func(err error) {
				ociRuntimeMock.EXPECT().
					ProbeMonitor(gomock.Any(), testContainer).
					Return(nil).
					AnyTimes()

				if method == "ExecSync" {
					ociRuntimeMock.EXPECT().
						ExecSyncContainer(gomock.Any(), testContainer, []string{"true"}, int64(0)).
						Return(nil, err)
				} else {
					ociRuntimeMock.EXPECT().
						ExecContainer(gomock.Any(), testContainer, []string{"true"},
							gomock.Any(), gomock.Any(), gomock.Any(), false, gomock.Any()).
						Return(err)
				}

				sut.Runtime().SetRuntimeImpl(testContainer.ID(), ociRuntimeMock)
			}

			exec := func() error {
				if method == "ExecSync" {
					_, err := sut.ExecSync(context.Background(), &types.ExecSyncRequest{
						ContainerId: testContainer.ID(), Cmd: []string{"true"},
					})

					return err
				}

				return testStreamService.Exec(
					context.Background(),
					testContainer.ID(),
					[]string{"true"},
					nil,
					nil,
					nil,
					false,
					nil,
				)
			}

			It("should answer NotFound when the container is not running", func() {
				mockExec(fmt.Errorf("%w: stopped", oci.ErrContainerNotRunning))

				err := exec()
				Expect(status.Code(err)).To(Equal(codes.NotFound))
				Expect(err.Error()).To(ContainSubstring("container is not created or running"))
			})

			DescribeTable("should report why the container state is unknown",
				func(cause error, code codes.Code) {
					mockExec(&oci.ErrContainerStateUnknown{Err: cause})

					err := exec()
					Expect(status.Code(err)).To(Equal(code))
					Expect(err.Error()).To(ContainSubstring(cause.Error()))
				},
				Entry("context deadline", context.DeadlineExceeded, codes.DeadlineExceeded),
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
				Entry(
					"other failure",
					errors.New("shim for container is not connected"),
					codes.Unavailable,
				),
			)

			It("should pass errors of the exec itself through", func() {
				mockExec(errors.New("exec failed"))

				err := exec()
				Expect(err).To(MatchError("exec failed"))
				Expect(status.Code(err)).To(Equal(codes.Unknown))
			})

			It("should not create a runtime for a container whose pod is gone", func() {
				serverConfig.Runtimes[serverConfig.DefaultRuntime].RuntimeType = config.RuntimeTypePod

				Expect(sut.Runtime().HasRuntimeImpl(testContainer.ID())).To(BeFalse())
				Expect(sut.Runtime().HasRuntimeImpl(testContainer.Sandbox())).To(BeFalse())

				err := exec()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no runtime implementation"))
				Expect(sut.Runtime().HasRuntimeImpl(testContainer.ID())).To(BeFalse())
				Expect(sut.Runtime().HasRuntimeImpl(testContainer.Sandbox())).To(BeFalse())
			})
		})
	}
})
