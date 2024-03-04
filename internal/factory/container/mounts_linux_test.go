package container

import (
	"context"
	"testing"

	sconfig "github.com/cri-o/cri-o/pkg/config"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestAddOCIBindsForDev(t *testing.T) {
	ctr, err := New()
	c := ctr.getContainerInfo()
	if err != nil {
		t.Error(err)
	}
	if err := ctr.SetConfig(&types.ContainerConfig{
		Mounts: []*types.Mount{
			{
				ContainerPath: "/dev",
				HostPath:      "/dev",
			},
		},
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Error(err)
	}

	config := sconfig.Config{}
	config.RuntimeConfig.BindMountPrefix = ""
	config.Root = ""
	config.AbsentMountSourcesToReject = nil
	_, err = c.addOCIBindMounts(context.Background(), "", &config, false, false, false)
	if err != nil {
		t.Error(err)
	}
	specAddMounts(c)
	for _, m := range ctr.Spec().Mounts() {
		if m.Destination == "/dev" {
			t.Error("/dev shouldn't be in the spec if it's bind mounted from kube")
		}
	}
	var foundDev bool
	for _, b := range ctr.getContainerInfo().mountInfo.criMounts {
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
	ctr, err := New()
	c := ctr.getContainerInfo()
	if err != nil {
		t.Error(err)
	}
	if err := ctr.SetConfig(&types.ContainerConfig{
		Mounts: []*types.Mount{
			{
				ContainerPath: "/sys",
				HostPath:      "/sys",
			},
		},
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Error(err)
	}

	config := sconfig.Config{}
	config.RuntimeConfig.BindMountPrefix = ""
	config.Root = ""
	config.AbsentMountSourcesToReject = nil
	_, err = c.addOCIBindMounts(context.Background(), "", &config, false, false, false)
	if err != nil {
		t.Error(err)
	}
	var howManySys int
	for _, b := range c.mountInfo.criMounts {
		if b.Destination == "/sys" && b.Type != "sysfs" {
			howManySys++
		}
	}
	if howManySys != 1 {
		t.Error("there is not a single /sys bind mount")
	}
}

func TestAddOCIBindsCGroupRW(t *testing.T) {
	ctr, err := New()
	c := ctr.getContainerInfo()
	if err != nil {
		t.Error(err)
	}
	if err := ctr.SetConfig(&types.ContainerConfig{
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Error(err)
	}

	config := sconfig.Config{}
	config.RuntimeConfig.BindMountPrefix = ""
	config.Root = ""
	config.AbsentMountSourcesToReject = nil
	_, err = c.addOCIBindMounts(context.Background(), "", &config, false, false, true)
	if err != nil {
		t.Error(err)
	}
	specAddMounts(c)
	var hasCgroupRW bool
	for _, m := range ctr.Spec().Mounts() {
		if m.Destination == "/sys/fs/cgroup" {
			for _, o := range m.Options {
				if o == "rw" {
					hasCgroupRW = true
				}
			}
		}
	}
	if !hasCgroupRW {
		t.Error("Cgroup mount not added with RW.")
	}

	ctr, err = New()
	if err != nil {
		t.Error(err)
	}
	if err := ctr.SetConfig(&types.ContainerConfig{
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Error(err)
	}
	var hasCgroupRO bool
	_, err = c.addOCIBindMounts(context.Background(), "", &config, false, false, false)
	if err != nil {
		t.Error(err)
	}
	specAddMounts(c)
	for _, m := range ctr.Spec().Mounts() {
		if m.Destination == "/sys/fs/cgroup" {
			for _, o := range m.Options {
				if o == "ro" {
					hasCgroupRO = true
				}
			}
		}
	}
	if !hasCgroupRO {
		t.Error("Cgroup mount not added with RO.")
	}
}

func TestAddOCIBindsErrorWithoutIDMap(t *testing.T) {
	ctr, err := New()
	c := ctr.getContainerInfo()
	if err != nil {
		t.Fatal(err)
	}

	if err := ctr.SetConfig(&types.ContainerConfig{
		Mounts: []*types.Mount{
			{
				ContainerPath: "/sys",
				HostPath:      "/sys",
				UidMappings: []*types.IDMapping{
					{
						HostId:      1000,
						ContainerId: 1,
						Length:      1000,
					},
				},
			},
		},
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Fatal(err)
	}
	config := sconfig.Config{}
	config.RuntimeConfig.BindMountPrefix = ""
	config.Root = ""
	config.AbsentMountSourcesToReject = nil
	_, err = c.addOCIBindMounts(context.Background(), "", &config, false, false, false)
	if err == nil {
		t.Errorf("Should have failed to create id mapped mount with no id map support")
	}

	_, err = c.addOCIBindMounts(context.Background(), "", &config, false, true, false)
	if err != nil {
		t.Errorf("%v", err)
	}
}
