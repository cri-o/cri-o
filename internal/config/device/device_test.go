package device_test

import (
	"github.com/cri-o/cri-o/internal/config/device"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = t.Describe("DeviceConfig", func() {
	var d *device.Config
	BeforeEach(func() {
		d = device.New()
	})
	t.Describe("LoadDevices", func() {
		It("should fail with poorly formatted device", func() {
			// Given
			// When
			err := d.LoadDevices([]string{"invalid:invalid"})
			// Then
			Expect(err).NotTo(BeNil())
			Expect(d.Devices()).To(BeEmpty())
		})
		It("should fail if invalid device", func() {
			// Given
			// When
			err := d.LoadDevices([]string{"/dev/invalid"})
			// Then
			Expect(err).NotTo(BeNil())
			Expect(d.Devices()).To(BeEmpty())
		})
		It("should succeed with valid device", func() {
			// Given
			// When
			err := d.LoadDevices([]string{"/dev/null:/dev/null:w"})
			// Then
			Expect(err).To(BeNil())
			Expect(d.Devices()).NotTo(BeEmpty())
		})
		It("should succeed with empty", func() {
			// Given
			// When
			err := d.LoadDevices([]string{""})
			// Then
			Expect(err).To(BeNil())
			Expect(d.Devices()).To(BeEmpty())
		})
	})
})
