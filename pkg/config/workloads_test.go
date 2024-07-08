package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/pkg/config"
)

// Helper function for pointer reference.
func pointer[A any](m A) *A {
	return &m
}

// The actual test suite.
var _ = t.Describe("Workloads config", func() {
	BeforeEach(beforeEach)

	It("should fail on invalid cpuset", func() {
		// Given
		workloads := config.Workloads{
			"management": &config.WorkloadConfig{
				ActivationAnnotation: "target.workload.openshift.io/management",
				AnnotationPrefix:     "resources.workload.openshift.io",
				Resources: &config.Resources{
					CPUSet: "-20",
				},
			},
		}
		// When
		sut.Workloads = workloads
		err := sut.Workloads.Validate()
		// Then
		Expect(err).To(HaveOccurred())
	})

	It("should fail when cpuquota is less than cpushares", func() {
		// Given
		workloads := config.Workloads{
			"management": &config.WorkloadConfig{
				ActivationAnnotation: "target.workload.openshift.io/management",
				AnnotationPrefix:     "resources.workload.openshift.io",
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
		Expect(err).To(HaveOccurred())
	})

	It("should fail when cpuperiod is less than 1000 usecs", func() {
		// Given
		workloads := config.Workloads{
			"management": &config.WorkloadConfig{
				ActivationAnnotation: "target.workload.openshift.io/management",
				AnnotationPrefix:     "resources.workload.openshift.io",
				Resources: &config.Resources{
					CPUPeriod: 999,
				},
			},
		}
		// When
		sut.Workloads = workloads
		err := sut.Workloads.Validate()
		// Then
		Expect(err).To(HaveOccurred())
	})

	It("should contain default values for resources", func() {
		// Given
		workloads := config.Workloads{
			"management": &config.WorkloadConfig{
				ActivationAnnotation: "target.workload.openshift.io/management",
				AnnotationPrefix:     "resources.workload.openshift.io",
				Resources:            &config.Resources{},
			},
		}
		// When
		err := workloads.Validate()
		// Then
		Expect(err).NotTo(HaveOccurred())
	})

	It("resources should validate defaults", func() {
		testCases := []struct {
			description string
			resources   config.Resources
		}{
			{
				description: "when only cpushares are provided",
				resources: config.Resources{
					CPUShares: 37,
				},
			},
			{
				description: "when only cpuquota is provided",
				resources: config.Resources{
					CPUQuota: 3700,
				},
			},
			{
				description: "when only cpuperiod is provided",
				resources: config.Resources{
					CPUPeriod: 37000,
				},
			},
			{
				description: "when only cpuset is provided",
				resources: config.Resources{
					CPUSet: "0-1",
				},
			},
		}

		for _, tc := range testCases {
			By(tc.description, func() {
				err := tc.resources.ValidateDefaults()
				Expect(err).NotTo(HaveOccurred())
			})
		}
	})

	It("resources should mutate the container spec", func() {
		testCases := []struct {
			description       string
			resources         config.Resources
			expectedCPUSet    string
			expectedCPUShare  *uint64
			expectedCPUQuota  *int64
			expectedCPUPeriod *uint64
		}{
			{
				description: "when values are provided",
				resources: config.Resources{
					CPUShares: 15,
					CPUSet:    "0-1",
					CPUQuota:  20,
					CPUPeriod: 50000,
				},
				expectedCPUSet:    "0-1",
				expectedCPUQuota:  pointer(int64(20)),
				expectedCPUShare:  pointer(uint64(15)),
				expectedCPUPeriod: pointer(uint64(50000)),
			},
		}

		for _, tc := range testCases {
			By(tc.description, func() {
				g := &generate.Generator{
					Config: &rspec.Spec{
						Linux: &rspec.Linux{
							Resources: &rspec.LinuxResources{},
						},
					},
				}
				tc.resources.MutateSpec(g)
				Expect(g.Config.Linux.Resources.CPU.Quota).To(Equal(tc.expectedCPUQuota))
				Expect(g.Config.Linux.Resources.CPU.Shares).To(Equal(tc.expectedCPUShare))
				Expect(g.Config.Linux.Resources.CPU.Period).To(Equal(tc.expectedCPUPeriod))
				Expect(g.Config.Linux.Resources.CPU.Cpus).To(Equal(tc.expectedCPUSet))
			})
		}
	})

	It("should mutate container spec based on annotation", func() {
		const (
			workloadsKey                = "management"
			containerName               = "limitbox"
			resourceContainerPrefix     = "resources.workload.openshift.io"
			resourceContainerAnnotation = resourceContainerPrefix + "/" + containerName
			workloadTargetAnnotation    = "target.workload.openshift.io/" + workloadsKey
		)

		// CPUShare support was the first feature implemented for workloads.
		// We always check 'cpushares' in every test case to insure its operation
		// does not change for existing users.
		testCases := []struct {
			description       string
			annotations       map[string]string
			expectedCPUShare  *uint64
			expectedCPUQuota  *int64
			expectedCPUPeriod *uint64
		}{
			{
				description: "when cpushares and cpulimit are present",
				annotations: map[string]string{
					resourceContainerAnnotation: "{\"cpushares\":15,\"cpulimit\":35}",
					workloadTargetAnnotation:    "{\"effect\":\"PreferredDuringScheduling\"}",
				},
				expectedCPUQuota: pointer(int64(3500)),
				expectedCPUShare: pointer(uint64(15)),
			},
			{
				description: "when cpulimit is present it should override cpuquota",
				annotations: map[string]string{
					resourceContainerAnnotation: "{\"cpushares\":35,\"cpulimit\":105,\"cpuquota\":35}",
					workloadTargetAnnotation:    "{\"effect\":\"PreferredDuringScheduling\"}",
				},
				expectedCPUQuota: pointer(int64(10500)),
				expectedCPUShare: pointer(uint64(35)),
			},
			{
				description: "when cpuquota is present",
				annotations: map[string]string{
					resourceContainerAnnotation: "{\"cpushares\":25,\"cpuquota\":35}",
					workloadTargetAnnotation:    "{\"effect\":\"PreferredDuringScheduling\"}",
				},
				expectedCPUQuota: pointer(int64(35)),
				expectedCPUShare: pointer(uint64(25)),
			},
			{
				description: "when only cpuperiod is present",
				annotations: map[string]string{
					resourceContainerAnnotation: "{\"cpushares\":65,\"cpuperiod\":50000}",
					workloadTargetAnnotation:    "{\"effect\":\"PreferredDuringScheduling\"}",
				},
				expectedCPUPeriod: pointer(uint64(50000)),
				expectedCPUShare:  pointer(uint64(65)),
			},
			{
				description: "when only cpushares is present",
				annotations: map[string]string{
					resourceContainerAnnotation: "{\"cpushares\":45}",
					workloadTargetAnnotation:    "{\"effect\":\"PreferredDuringScheduling\"}",
				},
				expectedCPUShare: pointer(uint64(45)),
			},
		}

		for _, tc := range testCases {
			By(tc.description, func() {
				// Given
				workloads := config.Workloads{
					workloadsKey: &config.WorkloadConfig{
						AnnotationPrefix:     resourceContainerPrefix,
						ActivationAnnotation: workloadTargetAnnotation,
						Resources: &config.Resources{
							CPUShares: 0,
							CPUSet:    "",
						},
					},
				}

				g := &generate.Generator{
					Config: &rspec.Spec{
						Linux: &rspec.Linux{
							Resources: &rspec.LinuxResources{},
						},
					},
				}
				err := workloads.MutateSpecGivenAnnotations(containerName, g, tc.annotations)
				Expect(err).NotTo(HaveOccurred())
				Expect(g.Config.Linux.Resources.CPU.Quota).To(Equal(tc.expectedCPUQuota))
				Expect(g.Config.Linux.Resources.CPU.Shares).To(Equal(tc.expectedCPUShare))
				Expect(g.Config.Linux.Resources.CPU.Period).To(Equal(tc.expectedCPUPeriod))
				Expect(g.Config.Linux.Resources.CPU.Cpus).To(Equal(""))
			})
		}
	})
})
