package container_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runc/libcontainer/devices"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
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
				config := &types.ContainerConfig{
					Metadata: &types.ContainerMetadata{Name: "name"},
					Linux: &types.LinuxContainerConfig{
						SecurityContext: &types.LinuxContainerSecurityContext{
							Privileged: test.privileged,
						},
					},
					Devices: []*types.Device{},
				}
				sboxConfig := &types.PodSandboxConfig{
					Linux: &types.LinuxPodSandboxConfig{
						SecurityContext: &types.LinuxSandboxSecurityContext{
							Privileged: test.privileged,
						},
					},
				}
				Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())
				Expect(sut.SetPrivileged()).To(BeNil())
				Expect(sut.Privileged()).To(Equal(test.privileged))
				Expect(len(hostDevices)).NotTo(Equal(0))

				// When
				err := sut.SpecAddDevices(nil, nil, test.privilegedWithoutHostDevices, false)
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
	t.Describe("SpecAddDevice", func() {
		type testdata struct {
			testDescription                    string
			uid, gid                           *types.Int64Value
			deviceOwnershipFromSecurityContext bool
			expectedDeviceUID                  uint32
			expectedDeviceGID                  uint32
		}
		hostDevices, err := devices.HostDevices()
		Expect(err).To(BeNil())

		testDevice := hostDevices[0]

		tests := []testdata{
			{
				testDescription:   "Expect non-root container's Devices Uid/Gid to be the same as the device Uid/Gid on the host when deviceOwnershipFromSecurityContext is disabled",
				uid:               &types.Int64Value{Value: 1},
				gid:               &types.Int64Value{Value: 10},
				expectedDeviceUID: testDevice.Uid,
				expectedDeviceGID: testDevice.Gid,
			},
			{
				testDescription:   "Expect root container's Devices Uid/Gid to be the same as the device Uid/Gid on the host when deviceOwnershipFromSecurityContext is disabled",
				uid:               &types.Int64Value{Value: 0},
				gid:               &types.Int64Value{Value: 0},
				expectedDeviceUID: testDevice.Uid,
				expectedDeviceGID: testDevice.Gid,
			},
			{
				testDescription:                    "Expect non-root container's Devices Uid/Gid to be the same as RunAsUser/RunAsGroup when deviceOwnershipFromSecurityContext is enabled",
				uid:                                &types.Int64Value{Value: 1},
				gid:                                &types.Int64Value{Value: 10},
				deviceOwnershipFromSecurityContext: true,
				expectedDeviceUID:                  1,
				expectedDeviceGID:                  10,
			},
			{
				testDescription:                    "Expect root container's Devices Uid/Gid to be the same as the device Uid/Gid on the host when deviceOwnershipFromSecurityContext is enabled",
				uid:                                &types.Int64Value{Value: 0},
				gid:                                &types.Int64Value{Value: 0},
				deviceOwnershipFromSecurityContext: true,
				expectedDeviceUID:                  testDevice.Uid,
				expectedDeviceGID:                  testDevice.Gid,
			},
		}

		for _, test := range tests {
			test := test
			It(test.testDescription, func() {
				// Given
				config := &types.ContainerConfig{
					Metadata: &types.ContainerMetadata{Name: "name"},
					Linux: &types.LinuxContainerConfig{
						SecurityContext: &types.LinuxContainerSecurityContext{
							RunAsUser:  test.uid,
							RunAsGroup: test.gid,
						},
					},
					Devices: []*types.Device{
						{
							ContainerPath: testDevice.Path,
							HostPath:      testDevice.Path,
							Permissions:   "r",
						},
					},
				}
				sboxConfig := &types.PodSandboxConfig{
					Linux: &types.LinuxPodSandboxConfig{
						SecurityContext: &types.LinuxSandboxSecurityContext{
							RunAsUser:  test.uid,
							RunAsGroup: test.gid,
						},
					},
				}
				Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())
				Expect(len(hostDevices)).NotTo(Equal(0))

				// When
				err := sut.SpecAddDevices(nil, nil, false, test.deviceOwnershipFromSecurityContext)
				// Then
				Expect(err).To(BeNil())

				Expect(len(sut.Spec().Config.Linux.Devices)).To(Equal(1))
				Expect(*sut.Spec().Config.Linux.Devices[0].UID).To(Equal(test.expectedDeviceUID))
				Expect(*sut.Spec().Config.Linux.Devices[0].GID).To(Equal(test.expectedDeviceGID))
			})
		}
	})
})
