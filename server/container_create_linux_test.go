// +build linux

package server

import (
	"context"
	"testing"

	"github.com/cri-o/cri-o/server/cri/types"
	"github.com/opencontainers/runtime-tools/generate"
)

func TestAddOCIBindsForDev(t *testing.T) {
	specgen, err := generate.New("linux")
	if err != nil {
		t.Error(err)
	}
	config := &types.ContainerConfig{
		Mounts: []*types.Mount{
			{
				ContainerPath: "/dev",
				HostPath:      "/dev",
			},
		},
	}
	_, binds, err := addOCIBindMounts(context.Background(), "", config, &specgen, "", nil)
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
	config := &types.ContainerConfig{
		Mounts: []*types.Mount{
			{
				ContainerPath: "/sys",
				HostPath:      "/sys",
			},
		},
	}
	_, binds, err := addOCIBindMounts(context.Background(), "", config, &specgen, "", nil)
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
