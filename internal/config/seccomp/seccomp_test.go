package seccomp_test

import (
	"io/ioutil"

	"github.com/cri-o/cri-o/internal/config/seccomp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	containers_seccomp "github.com/seccomp/containers-golang"
)

// The actual test suite
var _ = t.Describe("Config", func() {
	var sut *seccomp.Config

	BeforeEach(func() {
		sut = seccomp.New()
		Expect(sut).NotTo(BeNil())
	})

	t.Describe("Profile", func() {
		It("should be the default without any load", func() {
			// Given
			// When
			res := sut.Profile()

			// Then
			Expect(res).To(Equal(containers_seccomp.DefaultProfile()))
		})
	})

	t.Describe("LoadProfile", func() {
		It("should succeed with default profile", func() {
			// Given

			// When
			err := sut.LoadProfile("")

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with profile", func() {
			// Given
			file := t.MustTempFile("")
			Expect(ioutil.WriteFile(file, []byte(`{
				"names": ["clone"],
				"action": "SCMP_ACT_ALLOW",
				"args": [
					{
					"index": 1,
					"value": 2080505856,
					"valueTwo": 0,
					"op": "SCMP_CMP_MASKED_EQ"
					}
				],
				"comment": "s390 parameter ordering for clone is different",
				"includes": {
					"arches": ["s390", "s390x"]
				},
				"excludes": {
					"caps": ["CAP_SYS_ADMIN"]
				}
			}`), 0644)).To(BeNil())

			// When
			err := sut.LoadProfile(file)

			// Then
			Expect(err).To(BeNil())
		})

		if sut != nil && !sut.IsDisabled() {
			It("should fail with non-existing profile", func() {
				// Given
				// When
				err := sut.LoadProfile("/proc/not/existing/file")

				// Then
				Expect(err).NotTo(BeNil())
			})
		}
	})
})
