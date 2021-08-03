package process_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/process"
)

// The actual test suite
var _ = t.Describe("Process", func() {
	t.Describe("DefunctProcessesForPath", func() {
		Context("Should succeed", func() {
			It("when given a valid path name and there are defunct processes", func() {
				defunctCount, err := process.DefunctProcessesForPath("./testing/proc_success_1")

				Expect(err).To(BeNil())
				Expect(defunctCount).To(Equal(uint(7)))
			})
			It("when given a valid path name but there are no defunct processes", func() {
				defunctCount, err := process.DefunctProcessesForPath("./testing/proc_success_2")

				Expect(err).To(BeNil())
				Expect(defunctCount).To(Equal(uint(0)))
			})
			It("when given a valid path name but there are no processes", func() {
				defunctCount, err := process.DefunctProcessesForPath("./testing/proc_success_3")

				Expect(err).To(BeNil())
				Expect(defunctCount).To(Equal(uint(0)))
			})
			It("when given a valid path name but there are no directories", func() {
				defunctCount, err := process.DefunctProcessesForPath("./testing/proc_success_4")

				Expect(err).To(BeNil())
				Expect(defunctCount).To(Equal(uint(0)))
			})
		})
		Context("Should fail", func() {
			It("when given an invalid path name", func() {
				defunctCount, err := process.DefunctProcessesForPath("./test/proc")
				formattedErr := fmt.Sprintf("%v", err)

				Expect(formattedErr).To(Equal("open ./test/proc: no such file or directory"))
				Expect(defunctCount).To(Equal(uint(0)))
			})
			It("when the given path name does not belong to a directory", func() {
				defunctCount, err := process.DefunctProcessesForPath("./testing/proc_fail")
				formattedErr := fmt.Sprintf("%v", err)

				Expect(formattedErr).To(Equal("readdirent ./testing/proc_fail: not a directory"))
				Expect(defunctCount).To(Equal(uint(0)))
			})
		})
	})
})
