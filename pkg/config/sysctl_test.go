package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Sysctl", func() {
	BeforeEach(beforeEach)

	It("should succeed to parse sysctls in default config", func() {
		// Given
		// When
		sysctls, err := sut.Sysctls()

		// Then
		Expect(err).To(BeNil())
		Expect(sysctls).To(BeEmpty())
	})

	It("should succeed to parse sysctls in key=value format", func() {
		// Given
		sut.DefaultSysctls = []string{
			"a=b",
			"",
			"key=value",
			"some-fancy-foo=some-other-bar",
		}

		// When
		sysctls, err := sut.Sysctls()

		// Then
		Expect(err).To(BeNil())
		Expect(sysctls).To(HaveLen(3))
		Expect(sysctls[0].Key()).To(Equal("a"))
		Expect(sysctls[0].Value()).To(Equal("b"))
		Expect(sysctls[1].Key()).To(Equal("key"))
		Expect(sysctls[1].Value()).To(Equal("value"))
		Expect(sysctls[2].Key()).To(Equal("some-fancy-foo"))
		Expect(sysctls[2].Value()).To(Equal("some-other-bar"))
	})

	It("should fail to parse sysctls in wrong format", func() {
		// Given
		sut.DefaultSysctls = []string{"wrong-format"}

		// When
		sysctls, err := sut.Sysctls()

		// Then
		Expect(err).NotTo(BeNil())
		Expect(sysctls).To(BeNil())
	})

	It("should fail to parse sysctls with extra spaces", func() {
		// Given
		sut.DefaultSysctls = []string{"key = val"}

		// When
		sysctls, err := sut.Sysctls()

		// Then
		Expect(err).NotTo(BeNil())
		Expect(sysctls).To(BeNil())
	})

	It("should fail to validate not whitelisted sysctl with host NET and IPC namespaces", func() {
		// Given
		sut.DefaultSysctls = []string{"a=b"}
		sysctls, err := sut.Sysctls()
		Expect(err).To(BeNil())

		// When
		err = sysctls[0].Validate(true, true)

		// Then
		Expect(err).NotTo(BeNil())
	})

	It("should fail to validate not whitelisted sysctl without host NET and IPC namespaces", func() {
		// Given
		sut.DefaultSysctls = []string{"a=b"}
		sysctls, err := sut.Sysctls()
		Expect(err).To(BeNil())

		// When
		err = sysctls[0].Validate(false, false)

		// Then
		Expect(err).NotTo(BeNil())
	})

	It("should fail to validate whitelisted sysctl with enabled host NET namespace", func() {
		// Given
		sut.DefaultSysctls = []string{"net.ipv4.ip_forward=1"}
		sysctls, err := sut.Sysctls()
		Expect(err).To(BeNil())

		// When
		err = sysctls[0].Validate(true, false)

		// Then
		Expect(err).NotTo(BeNil())
	})

	It("should fail to validate whitelisted sysctl with enabled host IPC namespace", func() {
		// Given
		sut.DefaultSysctls = []string{"kernel.shmmax=100"}
		sysctls, err := sut.Sysctls()
		Expect(err).To(BeNil())

		// When
		err = sysctls[0].Validate(false, true)

		// Then
		Expect(err).NotTo(BeNil())
	})

	It("should succeed to validate whitelisted sysctl with disabled host NET namespace", func() {
		// Given
		sut.DefaultSysctls = []string{"net.ipv4.ip_forward=1"}
		sysctls, err := sut.Sysctls()
		Expect(err).To(BeNil())

		// When
		err = sysctls[0].Validate(false, true)

		// Then
		Expect(err).To(BeNil())
	})

	It("should succeed to validate whitelisted kernel sysctl with disabled host NET and IPC namespaces", func() {
		// Given
		sut.DefaultSysctls = []string{"kernel.sem=32001 1 1"}
		sysctls, err := sut.Sysctls()
		Expect(err).To(BeNil())

		// When
		err = sysctls[0].Validate(false, false)

		// Then
		Expect(err).To(BeNil())
	})

	It("should fail to validate whitelisted kernel sysctl with enabled host IPC namespace", func() {
		// Given
		sut.DefaultSysctls = []string{"kernel.sem=32001 1 1"}
		sysctls, err := sut.Sysctls()
		Expect(err).To(BeNil())

		// When
		err = sysctls[0].Validate(false, true)

		// Then
		Expect(err).NotTo(BeNil())
	})
})
