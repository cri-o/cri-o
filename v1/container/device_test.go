package container_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runc/libcontainer/devices"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var _ = t.Describe("Container", func() {
	t.Describe("SpecAddDevice", func() {
		type testdata struct {
			testDescription              string
			privileged                   bool
			privilegedWithoutHostDevices bool
			expectHostDevices            bool
		}
		hostDevices, err := devices.HostDevices()
		Expect(err).To(BeNil())

		tests := []testdata{
			{
				testDescription:              "Expect no host devices for non-privileged container",
				privileged:                   false,
				privilegedWithoutHostDevices: false,
				expectHostDevices:            false,
			},
			{
				testDescription:              "Expect no host devices for non-privileged container when privilegedWithoutHostDevices is true",
				privileged:                   false,
				privilegedWithoutHostDevices: true,
				expectHostDevices:            false,
			},
			{
				testDescription:              "Expect host devices for privileged container",
				privileged:                   true,
				privilegedWithoutHostDevices: false,
				expectHostDevices:            true,
			},
			{
				testDescription:              "Expect no host devices for privileged container when privilegedWithoutHostDevices is true",
				privileged:                   true,
				privilegedWithoutHostDevices: true,
				expectHostDevices:            false,
			},
		}

		for _, test := range tests {
			test := test
			It(test.testDescription, func() {
				// Given
				config := &pb.ContainerConfig{
					Metadata: &pb.ContainerMetadata{Name: "name"},
					Linux: &pb.LinuxContainerConfig{
						SecurityContext: &pb.LinuxContainerSecurityContext{
							Privileged: test.privileged,
						},
					},
					Devices: []*pb.Device{},
				}
				sboxConfig := &pb.PodSandboxConfig{
					Linux: &pb.LinuxPodSandboxConfig{
						SecurityContext: &pb.LinuxSandboxSecurityContext{
							Privileged: test.privileged,
						},
					},
				}
				Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())
				Expect(sut.SetPrivileged()).To(BeNil())
				Expect(sut.Privileged()).To(Equal(test.privileged))
				Expect(len(hostDevices)).NotTo(Equal(0))

				// When
				err := sut.SpecAddDevices(nil, nil, test.privilegedWithoutHostDevices)
				// Then
				Expect(err).To(BeNil())

				if !test.expectHostDevices {
					Expect(len(sut.Spec().Config.Linux.Devices)).To(Equal(0))
				} else {
					Expect(len(sut.Spec().Config.Linux.Devices)).To(Equal(len(hostDevices)))
				}
			})
		}
	})
})
