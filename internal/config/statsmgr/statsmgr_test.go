package statsmgr_test

import (
	"os"
	"path/filepath"

	"github.com/cri-o/cri-o/internal/config/statsmgr"
	"github.com/cri-o/cri-o/server/cri/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = t.Describe("StatsManager", func() {
	t.Describe("GetDiskUsageStats", func() {
		It("should succeed at the current working directory", func() {
			// Given
			// When
			bytes, inodes, err := statsmgr.GetDiskUsageStats(".")

			// Then
			Expect(err).To(BeNil())
			Expect(bytes).To(SatisfyAll(BeNumerically(">", 0)))
			Expect(inodes).To(SatisfyAll(BeNumerically(">", 0)))
		})

		It("should fail on invalid path", func() {
			// Given
			// When
			bytes, inodes, err := statsmgr.GetDiskUsageStats("/not-existing")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(bytes).To(BeEquivalentTo(0))
			Expect(inodes).To(BeEquivalentTo(0))
		})
	})
	for _, name := range []string{"legacy", "cached"} {
		name := name
		t.Describe(name+"StatsManager", func() {
			var (
				sut   statsmgr.StatsManager
				path  string
				stats *types.ContainerStats
				ctrID = "ctrID"
			)
			BeforeEach(func() {
				sut = statsmgr.New(name)
				path = t.MustTempDir(name + "-stats-mgr")
				stats = &types.ContainerStats{
					Attributes: &types.ContainerAttributes{
						ID: ctrID,
					},
				}
			})
			AfterEach(func() {
				sut.Shutdown()
			})
			It("should fail if stats improperly passed in", func() {
				// Then
				Expect(sut.UpdateWithDiskStats([]*types.ContainerStats{nil})).To(Not(BeNil()))
				stats.Attributes = nil
				Expect(sut.UpdateWithDiskStats([]*types.ContainerStats{nil})).To(Not(BeNil()))
			})
			It("should be able to get stats for added ID", func() {
				// Given
				sut.AddID(ctrID, path)
				// When
				Expect(sut.UpdateWithDiskStats([]*types.ContainerStats{stats})).To(BeNil())
				// Then
				Expect(stats.WritableLayer).To(Not(BeNil()))
				Expect(stats.WritableLayer.UsedBytes.Value).To(Not(BeZero()))
				Expect(stats.WritableLayer.InodesUsed.Value).To(Not(BeZero()))
			})
			It("should be able to remove non-existent ID", func() {
				sut.RemoveID(ctrID)
			})
			It("should fail to get stats for non-existent dir", func() {
				// Given
				sut.AddID(ctrID, "/notthere")
				// Then
				Expect(sut.UpdateWithDiskStats([]*types.ContainerStats{stats})).NotTo(BeNil())
			})
			It("should succeed eventually to get stats for newly created dir", func() {
				// When
				path = filepath.Join(path, "new-path")
				sut.AddID(ctrID, path)
				Expect(sut.UpdateWithDiskStats([]*types.ContainerStats{stats})).NotTo(BeNil())
				// Given
				os.MkdirAll(path, 0o755)
				// Then
				Expect(sut.UpdateWithDiskStats([]*types.ContainerStats{stats})).To(BeNil())
				// Given
			})
			It("should not fail to get stats for non-existent ID", func() {
				// Then
				Expect(sut.UpdateWithDiskStats([]*types.ContainerStats{stats})).To(BeNil())
				Expect(stats.WritableLayer).NotTo(BeNil())
				Expect(stats.WritableLayer.UsedBytes.Value).To(BeZero())
				Expect(stats.WritableLayer.InodesUsed.Value).To(BeZero())
			})
		})
	}
})
