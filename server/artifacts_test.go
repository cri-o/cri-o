package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/ociartifact"
	"github.com/cri-o/cri-o/server"
)

// The actual test suite.
var _ = t.Describe("Artifacts", func() {
	const artifact = "my-artifact"
	var (
		ctx   = context.Background()
		paths = []ociartifact.BlobMountPath{
			{Name: "1"},
			{Name: "dir-1/2"},
			{Name: "dir-1/3"},
			{Name: "dir-2/4"},
			{Name: "dir-2/5"},
		}
	)

	It("should succeed with empty sub path", func() {
		// Given

		// When
		res, err := server.FilterMountPathsBySubPath(ctx, artifact, "", paths)

		// Then
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(paths))
	})

	It("should succeed with filtered sub path", func() {
		// Given

		// When
		res, err := server.FilterMountPathsBySubPath(ctx, artifact, "dir-1", paths)

		// Then
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal([]ociartifact.BlobMountPath{
			{Name: "2"},
			{Name: "3"},
		}))
	})

	It("should succeed with '.' sub path", func() {
		// Given

		// When
		res, err := server.FilterMountPathsBySubPath(ctx, artifact, ".", paths)

		// Then
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(paths))
	})

	It("should succeed with './' prefix in sub path", func() {
		// Given

		// When
		res, err := server.FilterMountPathsBySubPath(ctx, artifact, "./dir-1", paths)

		// Then
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal([]ociartifact.BlobMountPath{
			{Name: "2"},
			{Name: "3"},
		}))
	})

	It("should fail if sub path is not existing", func() {
		// Given

		// When
		res, err := server.FilterMountPathsBySubPath(ctx, artifact, "dir-3", paths)

		// Then
		Expect(err).To(HaveOccurred())
		Expect(res).To(BeNil())
	})
})
