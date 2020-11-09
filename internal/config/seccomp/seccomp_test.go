package seccomp_test

import (
	"context"
	"io/ioutil"

	containers_seccomp "github.com/containers/common/pkg/seccomp"
	"github.com/cri-o/cri-o/internal/config/seccomp"
	"github.com/cri-o/cri-o/server/cri/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-tools/generate"
	k8sV1 "k8s.io/api/core/v1"
)

// The actual test suite
var _ = t.Describe("Config", func() {
	var sut *seccomp.Config

	BeforeEach(func() {
		sut = seccomp.New()
		Expect(sut).NotTo(BeNil())
	})

	writeProfileFile := func() string {
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
			}`), 0o644)).To(BeNil())
		return file
	}

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
			file := writeProfileFile()

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

	t.Describe("Setup", func() {
		It("should succeed with profile from file", func() {
			// Given
			generator, err := generate.New("linux")
			Expect(err).To(BeNil())
			file := writeProfileFile()

			// When
			err = sut.Setup(
				context.Background(),
				&generator,
				nil,
				k8sV1.SeccompLocalhostProfileNamePrefix+file,
			)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with profile from file and runtime default", func() {
			// Given
			generator, err := generate.New("linux")
			Expect(err).To(BeNil())

			// When
			err = sut.Setup(
				context.Background(),
				&generator,
				nil,
				k8sV1.SeccompProfileRuntimeDefault,
			)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with profile from file if wrong filename", func() {
			// Given
			generator, err := generate.New("linux")
			Expect(err).To(BeNil())

			// When
			err = sut.Setup(
				context.Background(),
				&generator,
				nil,
				"not-existing",
			)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should succeed with custom profile from field", func() {
			// Given
			generator, err := generate.New("linux")
			Expect(err).To(BeNil())
			field := &types.SecurityProfile{
				ProfileType: types.SecurityProfileTypeRuntimeDefault,
			}

			// When
			err = sut.Setup(
				context.Background(),
				&generator,
				field,
				"",
			)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with custom profile from field", func() {
			// Given
			generator, err := generate.New("linux")
			Expect(err).To(BeNil())
			file := writeProfileFile()
			field := &types.SecurityProfile{
				ProfileType:  types.SecurityProfileTypeLocalhost,
				LocalhostRef: file,
			}

			// When
			err = sut.Setup(
				context.Background(),
				&generator,
				field,
				"",
			)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with custom profile from field if not existing", func() {
			// Given
			generator, err := generate.New("linux")
			Expect(err).To(BeNil())
			field := &types.SecurityProfile{
				ProfileType:  types.SecurityProfileTypeLocalhost,
				LocalhostRef: "not-existing",
			}

			// When
			err = sut.Setup(
				context.Background(),
				&generator,
				field,
				"",
			)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
