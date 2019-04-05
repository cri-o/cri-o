// +build seccomp,amd64

package seccomp_test

import (
	"testing"

	"github.com/cri-o/cri-o/pkg/seccomp"
	. "github.com/cri-o/cri-o/test/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	libseccomp "github.com/seccomp/libseccomp-golang"
)

// TestSeccomp runs the created specs
func TestSeccomp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Seccomp")
}

// nolint: gochecknoglobals
var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})

// The actual test suite
var _ = t.Describe("Seccomp", func() {
	t.Describe("Enabled", func() {
		It("should be enabled by default", func() {
			// Given
			// When
			enabled := seccomp.IsEnabled()

			// Then
			Expect(enabled).To(BeTrue())
		})
	})

	t.Describe("LoadProfileFromStruct", func() {
		It("should succeed to load", func() {
			// Given
			// When
			err := seccomp.LoadProfileFromStruct(&seccomp.Seccomp{},
				&generate.Generator{})

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed to load with provided architectures", func() {
			// Given
			nativeArch, err := libseccomp.GetNativeArch()
			Expect(err).To(BeNil())

			// When
			err = seccomp.LoadProfileFromStruct(&seccomp.Seccomp{
				DefaultAction: "action",
				Syscalls: []*seccomp.Syscall{
					{Excludes: seccomp.Filter{
						Arches: []string{nativeArch.String()}}},
					{Excludes: seccomp.Filter{
						Caps: []string{"cap"}}},
					{Includes: seccomp.Filter{
						Arches: []string{"arch"}}},
					{Includes: seccomp.Filter{
						Caps: []string{"other_cap"}}},
					{Name: "name", Args: []*seccomp.Arg{{}}},
					{Names: []string{"name", "name"}},
				},
				Architectures: []seccomp.Arch{seccomp.Arch("arch")},
			}, &generate.Generator{
				Config: &specs.Spec{
					Process: &specs.Process{
						Capabilities: &specs.LinuxCapabilities{
							Permitted: []string{"cap"},
						},
					},
					Linux: &specs.Linux{Seccomp: &specs.LinuxSeccomp{}},
				},
			})

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed to load with provided arch map", func() {
			// Given
			arch := seccomp.ArchX86_64

			// When
			err := seccomp.LoadProfileFromStruct(&seccomp.Seccomp{
				DefaultAction: "action",
				ArchMap: []seccomp.Architecture{
					{arch, []seccomp.Arch{seccomp.Arch("arch")}},
				}}, &generate.Generator{
				Config: &specs.Spec{
					Linux: &specs.Linux{Seccomp: &specs.LinuxSeccomp{}},
				},
			})

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail to load on invalid data", func() {
			// Given
			// When
			err := seccomp.LoadProfileFromStruct(&seccomp.Seccomp{
				DefaultAction: "action",
				Architectures: []seccomp.Arch{seccomp.Arch("arch")},
				ArchMap: []seccomp.Architecture{{
					seccomp.Arch("arch"), nil,
				}},
			}, &generate.Generator{})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail to load on given `name` and `names`", func() {
			// Given
			// When
			err := seccomp.LoadProfileFromStruct(&seccomp.Seccomp{
				DefaultAction: "action",
				Syscalls: []*seccomp.Syscall{
					{Name: "name", Names: []string{"name"}},
				},
				Architectures: []seccomp.Arch{seccomp.Arch("arch")},
			}, &generate.Generator{
				Config: &specs.Spec{
					Linux: &specs.Linux{
						Seccomp: &specs.LinuxSeccomp{},
					},
				},
			})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on nil generator", func() {
			// Given
			// When
			err := seccomp.LoadProfileFromStruct(&seccomp.Seccomp{
				DefaultAction: "action",
				Syscalls:      []*seccomp.Syscall{{Name: "name"}}},
				nil)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on empty generator", func() {
			// Given
			// When
			err := seccomp.LoadProfileFromStruct(&seccomp.Seccomp{
				DefaultAction: "action",
				Syscalls:      []*seccomp.Syscall{{Name: "name"}}},
				&generate.Generator{})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on empty generator config", func() {
			// Given
			// When
			err := seccomp.LoadProfileFromStruct(&seccomp.Seccomp{
				DefaultAction: "action",
				Syscalls:      []*seccomp.Syscall{{Name: "name"}}},
				&generate.Generator{Config: &specs.Spec{}})

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("LoadProfileFromBytes", func() {
		It("should succeed to load", func() {
			// Given
			// When
			err := seccomp.LoadProfileFromBytes([]byte("{}"),
				&generate.Generator{})

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail to load on invalid JSON", func() {
			// Given
			// When
			err := seccomp.LoadProfileFromBytes([]byte(""),
				&generate.Generator{})

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

})
