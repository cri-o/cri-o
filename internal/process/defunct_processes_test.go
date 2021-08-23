package process_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/process"
)

// The actual test suite
var _ = t.Describe("Process", func() {
	t.Describe("ParseDefunctProcessesForPathAndParent", func() {
		Context("Should succeed", func() {
			It("when given a valid path name and there are defunct processes", func() {
				defunctCount, _, err := process.ParseDefunctProcessesForPathAndParent("./testing/proc_success_1", 0)

				Expect(err).To(BeNil())
				Expect(defunctCount).To(Equal(uint(7)))
			})
			It("to get children when given a valid path name and there are defunct processes", func() {
				_, children, err := process.ParseDefunctProcessesForPathAndParent("./testing/proc_success_1", 1)

				Expect(err).To(BeNil())
				Expect(len(children)).To(Equal(2))
			})
			It("when given a valid path name but there are no defunct processes", func() {
				defunctCount, _, err := process.ParseDefunctProcessesForPathAndParent("./testing/proc_success_2", 0)

				Expect(err).To(BeNil())
				Expect(defunctCount).To(Equal(uint(0)))
			})
			It("when given a valid path name but there are no processes", func() {
				defunctCount, _, err := process.ParseDefunctProcessesForPathAndParent("./testing/proc_success_3", 0)

				Expect(err).To(BeNil())
				Expect(defunctCount).To(Equal(uint(0)))
			})
			It("when given a valid path name but there are no directories", func() {
				defunctCount, _, err := process.ParseDefunctProcessesForPathAndParent("./testing/proc_success_4", 0)

				Expect(err).To(BeNil())
				Expect(defunctCount).To(Equal(uint(0)))
			})
		})
		Context("Should fail", func() {
			It("when given an invalid path name", func() {
				defunctCount, _, err := process.ParseDefunctProcessesForPathAndParent("./test/proc", 0)
				formattedErr := fmt.Sprintf("%v", err)

				Expect(formattedErr).To(Equal("open ./test/proc: no such file or directory"))
				Expect(defunctCount).To(Equal(uint(0)))
			})
			It("when the given path name does not belong to a directory", func() {
				defunctCount, _, err := process.ParseDefunctProcessesForPathAndParent("./testing/proc_fail", 0)
				formattedErr := fmt.Sprintf("%v", err)

				Expect(formattedErr).To(Equal("readdirent ./testing/proc_fail: not a directory"))
				Expect(defunctCount).To(Equal(uint(0)))
			})
		})
	})
})
