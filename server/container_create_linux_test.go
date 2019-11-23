// +build linux

package server

import (
	"context"
	"testing"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/opencontainers/runc/libcontainer/devices"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func TestAddOCIBindsForDev(t *testing.T) {
	specgen, err := generate.New("linux")
	if err != nil {

		t.Error(err)
	}
	config := &pb.ContainerConfig{
		Mounts: []*pb.Mount{
			{
				ContainerPath: "/dev",
				HostPath:      "/dev",
			},
		},
	}
	_, binds, err := addOCIBindMounts(context.Background(), "", config, &specgen, "")
	if err != nil {
		t.Error(err)
	}
	for _, m := range specgen.Mounts() {
		if m.Destination == "/dev" {
			t.Error("/dev shouldn't be in the spec if it's bind mounted from kube")
		}
	}
	var foundDev bool
	for _, b := range binds {
		if b.Destination == "/dev" {
			foundDev = true
			break
		}
	}
	if !foundDev {
		t.Error("no /dev mount found in spec mounts")
	}
}

func TestAddOCIBindsForSys(t *testing.T) {
	specgen, err := generate.New("linux")
	if err != nil {

		t.Error(err)
	}
	config := &pb.ContainerConfig{
		Mounts: []*pb.Mount{
			{
				ContainerPath: "/sys",
				HostPath:      "/sys",
			},
		},
	}
	_, binds, err := addOCIBindMounts(context.Background(), "", config, &specgen, "")
	if err != nil {
		t.Error(err)
	}
	var howManySys int
	for _, b := range binds {
		if b.Destination == "/sys" && b.Type != "sysfs" {
			howManySys++
		}
	}
	if howManySys != 1 {
		t.Error("there is not a single /sys bind mount")
	}
}

func TestAddDevicesPlatformPrivileged(t *testing.T) {
	specgen, err := generate.New("linux")
	if err != nil {
		t.Error(err)
	}

	type testdata struct {
		testDescription              string
		privileged                   bool
		privilegedWithoutHostDevices bool
		expectHostDevices            bool
	}

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
		config := &pb.ContainerConfig{
			Linux: &pb.LinuxContainerConfig{
				SecurityContext: &pb.LinuxContainerSecurityContext{},
			},
		}

		config.Linux.SecurityContext.Privileged = test.privileged
		config.Devices = []*pb.Device{}
		specgen.Config.Linux.Devices = []specs.LinuxDevice{}

		err = addDevicesPlatform(context.Background(), &sandbox.Sandbox{}, config, test.privilegedWithoutHostDevices, &specgen)
		if err != nil {
			t.Error(err)
		}

		if !test.expectHostDevices {
			if len(specgen.Config.Linux.Devices) != 0 {
				t.Errorf("%s, Devices found in spec : %+v", test.testDescription, specgen.Config.Linux.Devices)
			}
		} else {
			hostDevices, err := devices.HostDevices()
			if err != nil {
				t.Error(err)
			}

			if len(specgen.Config.Linux.Devices) != len(hostDevices) {
				t.Errorf("%s, Number of devices in spec %d does not equal to the number of host devices %d", test.testDescription, len(specgen.Config.Linux.Devices), len(hostDevices))
			}
		}
	}
}
