package seccomp_test

import (
	"context"
	"errors"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
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

	writeBaselineFile := func() string {
		file := t.MustTempFile("baseline")
		Expect(os.WriteFile(file, []byte(`{
				"defaultAction": "SCMP_ACT_ERRNO",
				"syscalls": [{"names": ["read"], "action": "SCMP_ACT_ALLOW"}]
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

	t.Describe("LoadBaselineProfile", func() {
		BeforeEach(func() {
			if sut.IsDisabled() {
				Skip("tests need enabled seccomp")
			}
		})

		It("should use the loaded profile without a path", func() {
			// Given
			// When
			err := sut.LoadBaselineProfile("", false)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.OCIProfilesSupported()).To(BeTrue())
			Expect(sut.BaselineProfile()).To(Equal(sut.Profile()))
		})

		It("should succeed with a valid profile", func() {
			// Given
			file := writeBaselineFile()

			// When
			err := sut.LoadBaselineProfile(file, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.BaselineProfile().Syscalls).To(HaveLen(1))
			Expect(sut.Profile()).To(Equal(seccomp.DefaultProfile()))
		})

		It("should reset to the loaded profile", func() {
			// Given
			Expect(sut.LoadBaselineProfile(writeBaselineFile(), false)).To(Succeed())

			// When
			err := sut.LoadBaselineProfile("", false)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.BaselineProfile()).To(Equal(sut.Profile()))
		})

		for name, content := range map[string]string{
			"invalid JSON":   `{`,
			"no filter":      `{}`,
			"unknown action": `{"defaultAction": "SCMP_ACT_ERRNO", "syscalls": [{"names": ["read"], "action": "SCMP_ACT_WRONG"}]}`,
			"no listener":    `{"defaultAction": "SCMP_ACT_NOTIFY"}`,
		} {
			It("should fail with "+name, func() {
				// Given
				file := t.MustTempFile("baseline")
				Expect(os.WriteFile(file, []byte(content), 0o644)).To(Succeed())

				// When
				err := sut.LoadBaselineProfile(file, false)

				// Then
				Expect(err).To(HaveOccurred())
				Expect(sut.BaselineProfile()).To(Equal(sut.Profile()))
			})
		}

		It("should fail with a missing profile", func() {
			// Given
			// When
			err := sut.LoadBaselineProfile("/proc/not/existing/file", false)

			// Then
			Expect(err).To(HaveOccurred())
		})
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

		It("should succeed with runtime default profile from field", func() {
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
				nil,
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(ref).To(Equal(types.SecurityProfile_RuntimeDefault.String()))
		})

		It("should succeed with localhost profile from field", func() {
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
				nil,
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(ref).To(Equal(file))
		})

		t.Describe("OCI profile", func() {
			var (
				generator generate.Generator
				profiles  *fakeOCIProfiles
			)

			const ref = "registry.k8s.io/profile@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

			BeforeEach(func() {
				var err error

				generator, err = generate.New("linux")
				Expect(err).ToNot(HaveOccurred())

				profiles = &fakeOCIProfiles{
					result: &rspec.LinuxSeccomp{DefaultAction: rspec.ActErrno},
				}

				Expect(sut.LoadBaselineProfile("", false)).To(Succeed())
			})

			setup := func(field *types.SecurityProfile, ociProfiles seccomp.OCIProfiles) (string, error) {
				_, ref, err := sut.Setup(
					context.Background(),
					nil,
					nil,
					"",
					"ctr",
					nil,
					nil,
					&generator,
					field,
					"",
					ociProfiles,
				)

				return ref, err
			}

			It("should load the merge with the runtime default as baseline", func() {
				// Given
				field := &types.SecurityProfile{ProfileType: types.SecurityProfile_OCI, OciRef: ref}

				// When
				res, err := setup(field, profiles)

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ref))
				Expect(profiles.ref).To(Equal(ref))
				Expect(profiles.baseline.DefaultAction).To(Equal(rspec.ActErrno))
				Expect(profiles.base).To(BeNil())
				Expect(generator.Config.Linux.Seccomp).To(Equal(profiles.result))
			})

			It("should not add a runtime default base profile to the same baseline", func() {
				// Given
				field := &types.SecurityProfile{
					ProfileType: types.SecurityProfile_OCI,
					OciRef:      ref,
					BaseProfile: &types.SecurityProfileBase{
						Type: types.SecurityProfileBase_RuntimeDefault,
					},
				}

				// When
				_, err := setup(field, profiles)

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(profiles.baseline).NotTo(BeNil())
				Expect(profiles.base).To(BeNil())
			})

			It("should add the runtime default base profile to a configured baseline", func() {
				// Given
				Expect(sut.LoadBaselineProfile(writeBaselineFile(), false)).To(Succeed())

				field := &types.SecurityProfile{
					ProfileType: types.SecurityProfile_OCI,
					OciRef:      ref,
					BaseProfile: &types.SecurityProfileBase{
						Type: types.SecurityProfileBase_RuntimeDefault,
					},
				}

				// When
				_, err := setup(field, profiles)

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(profiles.baseline.Syscalls).To(HaveLen(1))
				Expect(len(profiles.base.Syscalls)).To(BeNumerically(">", 1))
			})

			It("should add a localhost base profile", func() {
				// Given
				field := &types.SecurityProfile{
					ProfileType: types.SecurityProfile_OCI,
					OciRef:      ref,
					BaseProfile: &types.SecurityProfileBase{
						Type:         types.SecurityProfileBase_Localhost,
						LocalhostRef: writeBaselineFile(),
					},
				}

				// When
				_, err := setup(field, profiles)

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(profiles.baseline).NotTo(BeNil())
				Expect(profiles.base.Syscalls).To(HaveLen(1))
			})

			It(
				"should intersect a configured baseline with the profile of a runtime handler",
				func() {
					// Given
					handlerProfile := t.MustTempFile("handler")
					Expect(os.WriteFile(handlerProfile, []byte(`{
					"defaultAction": "SCMP_ACT_ERRNO",
					"syscalls": [{"names": ["write"], "action": "SCMP_ACT_ALLOW"}]
				}`), 0o644)).To(Succeed())
					Expect(sut.LoadProfile(handlerProfile)).To(Succeed())
					Expect(sut.LoadBaselineProfile(writeBaselineFile(), true)).To(Succeed())

					field := &types.SecurityProfile{
						ProfileType: types.SecurityProfile_OCI,
						OciRef:      ref,
						BaseProfile: &types.SecurityProfileBase{
							Type: types.SecurityProfileBase_RuntimeDefault,
						},
					}

					// When
					_, err := setup(field, profiles)

					// Then: the baseline allows read, the handler write, so
					// the floor allows neither, and it includes the default.
					Expect(err).ToNot(HaveOccurred())
					Expect(profiles.baseline.DefaultAction).To(Equal(rspec.ActErrno))

					for _, syscall := range profiles.baseline.Syscalls {
						Expect(syscall.Action).NotTo(Equal(rspec.ActAllow))
					}

					Expect(profiles.base).To(BeNil())
				},
			)

			It("should not support OCI profiles with an invalid runtime default", func() {
				// Given
				file := t.MustTempFile("invalid")
				Expect(os.WriteFile(file, []byte(`{"defaultAction": "SCMP_ACT_WRONG"}`), 0o644)).
					To(Succeed())
				Expect(sut.LoadProfile(file)).To(Succeed())
				Expect(sut.LoadBaselineProfile("", false)).To(Succeed())

				field := &types.SecurityProfile{ProfileType: types.SecurityProfile_OCI, OciRef: ref}

				// When
				_, err := setup(field, profiles)

				// Then
				Expect(err).To(HaveOccurred())
				Expect(sut.OCIProfilesSupported()).To(BeFalse())
				Expect(profiles.ref).To(BeEmpty())
			})

			It("should leave an unconfined runtime default out of the merge", func() {
				// Given
				file := t.MustTempFile("unconfined")
				Expect(os.WriteFile(file, []byte(`{}`), 0o644)).To(Succeed())
				Expect(sut.LoadProfile(file)).To(Succeed())
				Expect(sut.LoadBaselineProfile("", false)).To(Succeed())

				field := &types.SecurityProfile{ProfileType: types.SecurityProfile_OCI, OciRef: ref}

				// When
				_, err := setup(field, profiles)

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(profiles.ref).To(Equal(ref))
				Expect(profiles.baseline).To(BeNil())
				Expect(profiles.base).To(BeNil())
			})

			It("should leave an unconfined localhost base profile out of the merge", func() {
				// Given
				file := t.MustTempFile("unconfined")
				Expect(os.WriteFile(file, []byte(`{}`), 0o644)).To(Succeed())

				field := &types.SecurityProfile{
					ProfileType: types.SecurityProfile_OCI,
					OciRef:      ref,
					BaseProfile: &types.SecurityProfileBase{
						Type:         types.SecurityProfileBase_Localhost,
						LocalhostRef: file,
					},
				}

				// When
				_, err := setup(field, profiles)

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(profiles.baseline).NotTo(BeNil())
				Expect(profiles.base).To(BeNil())
			})

			It("should fail with a missing localhost base profile", func() {
				// Given
				field := &types.SecurityProfile{
					ProfileType: types.SecurityProfile_OCI,
					OciRef:      ref,
					BaseProfile: &types.SecurityProfileBase{
						Type:         types.SecurityProfileBase_Localhost,
						LocalhostRef: "not-existing",
					},
				}

				// When
				_, err := setup(field, profiles)

				// Then
				Expect(err).To(HaveOccurred())
				Expect(profiles.ref).To(BeEmpty())
			})

			It("should fail if the merge fails", func() {
				// Given
				profiles.err = errors.New("not pulled")
				field := &types.SecurityProfile{ProfileType: types.SecurityProfile_OCI, OciRef: ref}

				// When
				_, err := setup(field, profiles)

				// Then
				Expect(err).To(MatchError(profiles.err))
			})

			It("should fail without OCI profiles", func() {
				// Given
				field := &types.SecurityProfile{ProfileType: types.SecurityProfile_OCI, OciRef: ref}

				// When
				_, err := setup(field, nil)

				// Then
				Expect(err).To(HaveOccurred())
			})
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
				nil,
			)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
})

// fakeOCIProfiles records the merge it is asked for.
type fakeOCIProfiles struct {
	ref            string
	baseline, base *rspec.LinuxSeccomp
	result         *rspec.LinuxSeccomp
	err            error
}

func (f *fakeOCIProfiles) MergeSeccomp(
	_ context.Context,
	ref string,
	baseline, base *rspec.LinuxSeccomp,
) (*rspec.LinuxSeccomp, error) {
	f.ref, f.baseline, f.base = ref, baseline, base

	return f.result, f.err
}
