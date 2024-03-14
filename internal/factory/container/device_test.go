package container_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rspec "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/opencontainers/runc/libcontainer/devices"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	"tags.cncf.io/container-device-interface/pkg/cdi"
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
		Expect(err).ToNot(HaveOccurred())

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
				Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())
				Expect(sut.SetPrivileged()).To(Succeed())
				Expect(sut.Privileged()).To(Equal(test.privileged))
				Expect(hostDevices).NotTo(BeEmpty())

				// When
				err := sut.SpecAddDevices(nil, nil, test.privilegedWithoutHostDevices, false)
				// Then
				Expect(err).ToNot(HaveOccurred())

				if !test.expectHostDevices {
					Expect(sut.Spec().Config.Linux.Devices).To(BeEmpty())
				} else {
					Expect(sut.Spec().Config.Linux.Devices).To(HaveLen(len(hostDevices)))
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
		Expect(err).ToNot(HaveOccurred())

		// Find a host device with uid != gid using first device as fallback.
		testDevice := hostDevices[0]
		if testDevice.Uid == testDevice.Gid {
			for _, d := range hostDevices {
				if d.Uid != d.Gid {
					testDevice = d
					break
				}
			}
		}

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
				Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())
				Expect(hostDevices).NotTo(BeEmpty())

				// When
				err := sut.SpecAddDevices(nil, nil, false, test.deviceOwnershipFromSecurityContext)
				// Then
				Expect(err).ToNot(HaveOccurred())

				Expect(sut.Spec().Config.Linux.Devices).To(HaveLen(1))
				Expect(*sut.Spec().Config.Linux.Devices[0].UID).To(Equal(test.expectedDeviceUID))
				Expect(*sut.Spec().Config.Linux.Devices[0].GID).To(Equal(test.expectedDeviceGID))
			})
		}
	})

	t.Describe("SpecAdd(CDI)Devices", func() {
		writeCDISpecFiles := func(content []string) error {
			if len(content) == 0 {
				return nil
			}

			dir := t.MustTempDir("cdi")
			for idx, data := range content {
				file := filepath.Join(dir, fmt.Sprintf("spec-%d.yaml", idx))
				err := os.WriteFile(file, []byte(data), 0o644)
				if err != nil {
					return err
				}
			}

			return cdi.GetRegistry(cdi.WithSpecDirs(dir)).Refresh()
		}

		type testdata struct {
			testDescription string
			cdiSpecFiles    []string
			cdiDevices      []*types.CDIDevice
			annotations     map[string]string
			expectError     bool
			expectDevices   []rspec.LinuxDevice
			expectEnv       []string
		}

		tests := []testdata{
			// test CDI device injection by dedicated CRI CDIDevices field
			{
				testDescription: "Expect no CDI error for nil CDIDevices",
			},
			{
				testDescription: "Expect no CDI error for empty CDIDevices",
				cdiDevices:      []*types.CDIDevice{},
			},
			{
				testDescription: "Expect CDI error for invalid CDI device reference in CDIDevices",
				cdiDevices: []*types.CDIDevice{
					{
						Name: "foobar",
					},
				},
				expectError: true,
			},
			{
				testDescription: "Expect CDI error for unresolvable CDIDevices",
				cdiDevices: []*types.CDIDevice{
					{
						Name: "vendor1.com/device=no-such-dev",
					},
				},
				expectError: true,
			},
			{
				testDescription: "Expect properly injected resolvable CDIDevices",
				cdiSpecFiles: []string{
					`
cdiVersion: "0.3.0"
kind: "vendor1.com/device"
devices:
  - name: foo
    containerEdits:
      deviceNodes:
        - path: /dev/loop8
          type: b
          major: 7
          minor: 8
      env:
        - FOO=injected
containerEdits:
  env:
    - "VENDOR1=present"
`,
					`
cdiVersion: "0.3.0"
kind: "vendor2.com/device"
devices:
  - name: bar
    containerEdits:
      deviceNodes:
        - path: /dev/loop9
          type: b
          major: 7
          minor: 9
      env:
        - BAR=injected
containerEdits:
  env:
    - "VENDOR2=present"
`,
				},
				cdiDevices: []*types.CDIDevice{
					{
						Name: "vendor1.com/device=foo",
					},
					{
						Name: "vendor2.com/device=bar",
					},
				},
				expectDevices: []rspec.LinuxDevice{
					{
						Path:  "/dev/loop8",
						Type:  "b",
						Major: 7,
						Minor: 8,
					},
					{
						Path:  "/dev/loop9",
						Type:  "b",
						Major: 7,
						Minor: 9,
					},
				},
				expectEnv: []string{
					"FOO=injected",
					"VENDOR1=present",
					"BAR=injected",
					"VENDOR2=present",
				},
			},
			// test CDI device injection by annotations
			{
				testDescription: "Expect no CDI error for nil annotations",
			},
			{
				testDescription: "Expect no CDI error for empty annotations",
				annotations:     map[string]string{},
			},
			{
				testDescription: "Expect CDI error for invalid CDI device reference in annotations",
				annotations: map[string]string{
					cdi.AnnotationPrefix + "devices": "foobar",
				},
				expectError: true,
			},
			{
				testDescription: "Expect CDI error for unresolvable devices",
				annotations: map[string]string{
					cdi.AnnotationPrefix + "vendor1_devices": "vendor1.com/device=no-such-dev",
				},
				expectError: true,
			},
			{
				testDescription: "Expect properly injected resolvable CDI devices",
				cdiSpecFiles: []string{
					`
cdiVersion: "0.3.0"
kind: "vendor1.com/device"
devices:
  - name: foo
    containerEdits:
      deviceNodes:
        - path: /dev/loop8
          type: b
          major: 7
          minor: 8
      env:
        - FOO=injected
containerEdits:
  env:
    - "VENDOR1=present"
`,
					`
cdiVersion: "0.3.0"
kind: "vendor2.com/device"
devices:
  - name: bar
    containerEdits:
      deviceNodes:
        - path: /dev/loop9
          type: b
          major: 7
          minor: 9
      env:
        - BAR=injected
containerEdits:
  env:
    - "VENDOR2=present"
`,
				},
				annotations: map[string]string{
					cdi.AnnotationPrefix + "vendor1_devices": "vendor1.com/device=foo",
					cdi.AnnotationPrefix + "vendor2_devices": "vendor2.com/device=bar",
				},
				expectDevices: []rspec.LinuxDevice{
					{
						Path:  "/dev/loop8",
						Type:  "b",
						Major: 7,
						Minor: 8,
					},
					{
						Path:  "/dev/loop9",
						Type:  "b",
						Major: 7,
						Minor: 9,
					},
				},
				expectEnv: []string{
					"FOO=injected",
					"VENDOR1=present",
					"BAR=injected",
					"VENDOR2=present",
				},
			},
		}

		for _, test := range tests {
			test := test
			It(test.testDescription, func() {
				// Given
				config := &types.ContainerConfig{
					Metadata:    &types.ContainerMetadata{Name: "name"},
					Annotations: test.annotations,
					Linux: &types.LinuxContainerConfig{
						SecurityContext: &types.LinuxContainerSecurityContext{},
					},
					Devices:    []*types.Device{},
					CDIDevices: test.cdiDevices,
				}
				sboxConfig := &types.PodSandboxConfig{
					Linux: &types.LinuxPodSandboxConfig{
						SecurityContext: &types.LinuxSandboxSecurityContext{},
					},
				}
				Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())
				Expect(sut.SetPrivileged()).To(Succeed())
				Expect(writeCDISpecFiles(test.cdiSpecFiles)).To(Succeed())

				// When
				err := sut.SpecAddDevices(nil, nil, false, false)

				// Then
				Expect(err != nil).To(Equal(test.expectError))
				if err == nil {
					Expect(sut.Spec().Config.Process.Env).Should(ContainElements(test.expectEnv))
					Expect(sut.Spec().Config.Linux.Devices).Should(ContainElements(test.expectDevices))
				}
			})
		}
	})
})
