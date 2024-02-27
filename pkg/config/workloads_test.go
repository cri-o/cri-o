package config_test

import (
	"github.com/cri-o/cri-o/pkg/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Workloads", func() {
	BeforeEach(beforeEach)

	It("should fail on invalid cpuset", func() {
		// Given
		workloads := config.Workloads{
			"management": &config.WorkloadConfig{
				Resources: &config.Resources{
					CPUSet: "-20",
				},
			},
		}
		// When
		sut.Workloads = workloads
		err := sut.Workloads.Validate()
		// Then
		Expect(err).NotTo(BeNil())
	})

	It("should fail when cpuquota is less than cpushares", func() {
		// Given
		workloads := config.Workloads{
			"management": &config.WorkloadConfig{
				Resources: &config.Resources{
					CPUShares: 2000,
					CPUQuota:  1000,
				},
			},
		}
		// When
		sut.Workloads = workloads
		err := sut.Workloads.Validate()
		// Then
		Expect(err).NotTo(BeNil())
	})

	It("should fail when cpuperiod is less than 1000 usecs", func() {
		// Given
		workloads := config.Workloads{
			"management": &config.WorkloadConfig{
				Resources: &config.Resources{
					CPUPeriod: 999,
				},
			},
		}
		// When
		sut.Workloads = workloads
		err := sut.Workloads.Validate()
		// Then
		Expect(err).NotTo(BeNil())
	})

	It("should contain default values for resources", func() {
		// Given
		workloads := config.Workloads{
			"management": &config.WorkloadConfig{},
		}
		// When
		sut.Workloads = workloads
		err := sut.Workloads.Validate()
		// Then
		Expect(err).NotTo(BeNil())
	})
})
