package server

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/cri-o/cri-o/lib"
	"github.com/cri-o/cri-o/oci"
)

const fixturePath = "fixtures/crio.conf"

func must(t *testing.T, err error) {
	if err != nil {
		t.Error(err)
	}
}

func fails(t *testing.T, err error) {
	if err == nil {
		t.Error(err)
	}
}

func assertAllFieldsEquality(t *testing.T, c Config) {
	testCases := []struct {
		fieldValue, expected interface{}
	}{
		{c.RootConfig.Root, "/var/lib/containers/storage"},
		{c.RootConfig.RunRoot, "/var/run/containers/storage"},
		{c.RootConfig.Storage, "overlay"},
		{len(c.RootConfig.StorageOptions), 0},

		{c.APIConfig.Listen, "/var/run/crio.sock"},
		{c.APIConfig.StreamPort, "10010"},
		{c.APIConfig.StreamAddress, "localhost"},

		{c.RuntimeConfig.Conmon, "/usr/local/libexec/crio/conmon"},
		{c.RuntimeConfig.ConmonEnv[0], "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		{c.RuntimeConfig.SELinux, true},
		{c.RuntimeConfig.SeccompProfile, "/etc/crio/seccomp.json"},
		{c.RuntimeConfig.ApparmorProfile, "crio-default"},
		{c.RuntimeConfig.CgroupManager, "cgroupfs"},
		{c.RuntimeConfig.PidsLimit, int64(1024)},

		{c.ImageConfig.DefaultTransport, "docker://"},
		{c.ImageConfig.PauseImage, "kubernetes/pause"},
		{c.ImageConfig.PauseImageAuthFile, "/var/lib/kubelet/config.json"},
		{c.ImageConfig.PauseCommand, "/pause"},
		{c.ImageConfig.SignaturePolicyPath, "/tmp"},
		{c.ImageConfig.ImageVolumes, lib.ImageVolumesType("mkdir")},
		{c.ImageConfig.InsecureRegistries[0], "insecure-registry:1234"},
		{c.ImageConfig.Registries[0], "registry:4321"},

		{c.NetworkConfig.NetworkDir, "/etc/cni/net.d/"},
		{c.NetworkConfig.PluginDirs[0], "/opt/cni/bin/"},
	}
	for _, tc := range testCases {
		if tc.fieldValue != tc.expected {
			t.Errorf(`Expecting: "%s", got: "%s"`, tc.expected, tc.fieldValue)
		}
	}
}

func TestUpdateFromFile(t *testing.T) {
	c := Config{}

	must(t, c.UpdateFromFile(fixturePath))

	assertAllFieldsEquality(t, c)
}

func TestToFile(t *testing.T) {
	configFromFixture := Config{}

	must(t, configFromFixture.UpdateFromFile(fixturePath))

	f, err := ioutil.TempFile("", "crio.conf")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(f.Name())

	must(t, configFromFixture.ToFile(f.Name()))

	writtenConfig := Config{}
	err = writtenConfig.UpdateFromFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	assertAllFieldsEquality(t, writtenConfig)
}

func TestConfigValidateDefaultSuccess(t *testing.T) {
	defaultConfig := DefaultConfig()
	must(t, defaultConfig.Validate(false))
}

func TestConfigValidateDefaultSuccessOnExecution(t *testing.T) {
	defaultConfig := DefaultConfig()

	// since some test systems do not have runc installed, assume a more
	// generally available executable
	const validPath = "/bin/sh"
	validDirPath := path.Join(os.TempDir(), "crio-empty")
	defer os.RemoveAll(validDirPath)
	defaultConfig.Runtimes["runc"] = oci.RuntimeHandler{RuntimePath: validDirPath}
	defaultConfig.Conmon = validPath
	defaultConfig.NetworkConfig.NetworkDir = validDirPath
	defaultConfig.NetworkConfig.PluginDirs = []string{validDirPath}

	must(t, defaultConfig.Validate(true))
}

func TestConfigValidateFailsOnUnrecognizedImageVolumeType(t *testing.T) {
	defaultConfig := DefaultConfig()
	defaultConfig.ImageVolumes = "wrong"
	fails(t, defaultConfig.Validate(false))
}

func TestConfigValidateFailsOnInvalidRuntimeConfig(t *testing.T) {
	defaultConfig := DefaultConfig()
	defaultConfig.DefaultUlimits = []string{"wrong"}
	fails(t, defaultConfig.Validate(false))
}

func TestConfigValidateFailsOnUIDMappings(t *testing.T) {
	defaultConfig := DefaultConfig()
	defaultConfig.UIDMappings = "value"
	defaultConfig.ManageNetworkNSLifecycle = true
	fails(t, defaultConfig.Validate(false))
}

func TestConfigValidateFailsOnGIDMappings(t *testing.T) {
	defaultConfig := DefaultConfig()
	defaultConfig.GIDMappings = "value"
	defaultConfig.ManageNetworkNSLifecycle = true
	fails(t, defaultConfig.Validate(false))
}

func TestConfigValidateFailsOnInvalidLogSizeMax(t *testing.T) {
	defaultConfig := DefaultConfig()
	defaultConfig.LogSizeMax = 1
	fails(t, defaultConfig.Validate(false))
}
