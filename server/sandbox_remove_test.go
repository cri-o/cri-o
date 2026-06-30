package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var _ = t.Describe("PodSandboxRemove", func() {
	ctx := context.TODO()

	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("PodSandboxRemove", func() {
		It("should fail with empty sandbox ID", func() {
			_, err := sut.RemovePodSandbox(context.Background(),
				&types.RemovePodSandboxRequest{})

			Expect(err).To(HaveOccurred())
		})

		It("should return NotFound when sandbox is not created", func() {
			Expect(sut.AddSandbox(ctx, testSandbox)).To(Succeed())
			Expect(sut.PodIDIndex().Add(testSandbox.ID())).To(Succeed())

			_, err := sut.RemovePodSandbox(context.Background(),
				&types.RemovePodSandboxRequest{PodSandboxId: testSandbox.ID()})

			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
		})

		It("should succeed with no error when sandbox not found", func() {
			_, err := sut.RemovePodSandbox(context.Background(),
				&types.RemovePodSandboxRequest{PodSandboxId: "nonexistent"})

			Expect(err).NotTo(HaveOccurred())
		})
	})
})
