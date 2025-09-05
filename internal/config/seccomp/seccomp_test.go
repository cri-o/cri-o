package seccomp_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/seccomp"
)

// The actual test suite.
var _ = t.Describe("Config", func() {
	var sut *seccomp.Config

	BeforeEach(func() {
		sut = seccomp.New()
		Expect(sut).NotTo(BeNil())
	})

	writeProfileFile := func() string {
		file := t.MustTempFile("")
		Expect(os.WriteFile(file, []byte(`{
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
			}`), 0o644)).To(Succeed())

		return file
	}

	t.Describe("Profile", func() {
		It("should be the default without any load", func() {
			// Given
			// When
			res := sut.Profile()

			// Then
			Expect(res).To(Equal(seccomp.DefaultProfile()))
		})
	})

	t.Describe("LoadProfile", func() {
		It("should succeed with profile", func() {
			// Given
			file := writeProfileFile()

			// When
			err := sut.LoadProfile(file)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		if sut != nil && !sut.IsDisabled() {
			It("should not fail with non-existing profile", func() {
				// Given
				// When
				err := sut.LoadProfile("/proc/not/existing/file")

				// Then
				Expect(err).ToNot(HaveOccurred())
			})
		}
	})

	t.Describe("LoadDefaultProfile", func() {
		It("should succeed", func() {
			// Given
			// When
			err := sut.LoadDefaultProfile()

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.Profile()).To(Equal(seccomp.DefaultProfile()))
		})
	})

	t.Describe("Setup", func() {
		BeforeEach(func() {
			if sut.IsDisabled() {
				Skip("tests need to run as root and enabled seccomp")
			}
		})

		It("should succeed with custom profile from field", func() {
			// Given
			generator, err := generate.New("linux")
			Expect(err).ToNot(HaveOccurred())
			field := &types.SecurityProfile{
				ProfileType: types.SecurityProfile_RuntimeDefault,
			}

			// When
			_, ref, err := sut.Setup(
				context.Background(),
				nil,
				nil,
				"",
				"",
				nil,
				nil,
				&generator,
				field,
				"",
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(ref).To(Equal(types.SecurityProfile_RuntimeDefault.String()))
		})

		It("should succeed with custom profile from field", func() {
			// Given
			generator, err := generate.New("linux")
			Expect(err).ToNot(HaveOccurred())
			file := writeProfileFile()
			field := &types.SecurityProfile{
				ProfileType:  types.SecurityProfile_Localhost,
				LocalhostRef: file,
			}

			// When
			_, ref, err := sut.Setup(
				context.Background(),
				nil,
				nil,
				"",
				"",
				nil,
				nil,
				&generator,
				field,
				"",
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(ref).To(Equal(file))
		})

		It("should fail with custom profile from field if not existing", func() {
			// Given
			generator, err := generate.New("linux")
			Expect(err).ToNot(HaveOccurred())
			field := &types.SecurityProfile{
				ProfileType:  types.SecurityProfile_Localhost,
				LocalhostRef: "not-existing",
			}

			// When
			_, _, err = sut.Setup(
				context.Background(),
				nil,
				nil,
				"",
				"",
				nil,
				nil,
				&generator,
				field,
				"",
			)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
})
