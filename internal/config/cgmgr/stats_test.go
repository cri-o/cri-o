package cgmgr_test

import (
	"fmt"
	"os"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Stats", func() {
	t.Describe("UpdateWithMemoryStatsFromFile", func() {
		var file string
		BeforeEach(func() {
			file = t.MustTempFile("memoryStatFile")
		})
		It("fail if invalid file", func() {
			_, err := cgmgr.MemoryStatsFromFile("invalid", "", 0)
			Expect(err).ToNot(BeNil())
		})
		It("should get stats from file", func() {
			var (
				inactiveFileVal    uint64 = 100
				rssVal             uint64 = 101
				pgFaultVal         uint64 = 102
				pgMajFaultVal      uint64 = 102
				expectedUsage      uint64 = 1
				inactiveFileSearch        = "inactive_file "
			)
			data := fmt.Sprintf("%s%d\npgfault %d\npgmajfault %d\nrss %d", inactiveFileSearch,
				inactiveFileVal, pgFaultVal, pgMajFaultVal, rssVal)

			Expect(os.WriteFile(file, []byte(data), 0o600)).To(BeNil())

			memStats, err := cgmgr.MemoryStatsFromFile(file, inactiveFileSearch, inactiveFileVal+expectedUsage)
			Expect(err).To(BeNil())
			Expect(memStats).NotTo(BeNil())

			Expect(memStats.Rss).To(Equal(rssVal))
			Expect(memStats.PgFault).To(Equal(pgFaultVal))
			Expect(memStats.PgMajFault).To(Equal(pgMajFaultVal))
			Expect(memStats.WorkingSet).To(Equal(expectedUsage))
		})
		It("should get stats from file with different inactive search string", func() {
			var (
				inactiveFileVal    uint64 = 100
				expectedUsage      uint64 = 1
				inactiveFileSearch        = "total_inactive_file "
			)
			data := fmt.Sprintf("%s%d", inactiveFileSearch, inactiveFileVal)

			Expect(os.WriteFile(file, []byte(data), 0o600)).To(BeNil())

			memStats, err := cgmgr.MemoryStatsFromFile(file, inactiveFileSearch, inactiveFileVal+expectedUsage)
			Expect(err).To(BeNil())
			Expect(memStats).NotTo(BeNil())

			Expect(memStats.WorkingSet).To(Equal(expectedUsage))
		})
		It("should fail from invalid", func() {
			var (
				inactiveFileVal    = "failure"
				inactiveFileSearch = "total_inactive_file "
			)
			data := fmt.Sprintf("%s%s", inactiveFileSearch, inactiveFileVal)

			Expect(os.WriteFile(file, []byte(data), 0o600)).To(BeNil())

			_, err := cgmgr.MemoryStatsFromFile(file, inactiveFileSearch, 0)

			Expect(err).NotTo(BeNil())
		})
		It("should not set WorkingSetBytes if negative", func() {
			var (
				inactiveFileVal    uint64 = 2
				usage              uint64 = 1
				inactiveFileSearch        = "total_inactive_file "
			)
			data := fmt.Sprintf("%s%d", inactiveFileSearch, inactiveFileVal)

			Expect(os.WriteFile(file, []byte(data), 0o600)).To(BeNil())

			memStats, err := cgmgr.MemoryStatsFromFile(file, inactiveFileSearch, usage)

			Expect(err).To(BeNil())
			Expect(memStats.WorkingSet).To(Equal(uint64(0)))
		})
	})
})
