package lib

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/kubernetes-sigs/cri-o/oci"
)

// TestConfigToFile ensures Config.ToFile(..) encodes and writes out
// a Config instance toa a file on disk.
func TestConfigToFile(t *testing.T) {
	// Test with a default configuration
	c := DefaultConfig()
	tmpfile, err := ioutil.TempFile("", "config")
	if err != nil {
		t.Fatalf("Unable to create temporary file: %+v", err)
	}
	// Clean up temporary file
	defer os.Remove(tmpfile.Name())

	// Make the ToFile calls
	err = c.ToFile(tmpfile.Name())
	// Make sure no errors occurred while populating the file
	if err != nil {
		t.Fatalf("Unable to write to temporary file: %+v", err)
	}

	// Make sure the file is on disk
	if _, err := os.Stat(tmpfile.Name()); os.IsNotExist(err) {
		t.Fatalf("The config file was not written to disk: %+v", err)
	}
}

// TestConfigUpdateFromFile ensures Config.UpdateFromFile(..) properly
// updates an already create Config instancec with new data.
func TestConfigUpdateFromFile(t *testing.T) {
	// Test with a default configuration
	c := DefaultConfig()
	// Make the ToFile calls
	err := c.UpdateFromFile("testdata/config.toml")
	// Make sure no errors occurred while populating from the file
	if err != nil {
		t.Fatalf("Unable update config from file: %+v", err)
	}

	// Check fields that should have changed after UpdateFromFile
	if c.Storage != "overlay2" {
		t.Fatalf("Update failed. Storage did not change to overlay2")
	}

	if c.RuntimeConfig.PidsLimit != 2048 {
		t.Fatalf("Update failed. RuntimeConfig.PidsLimit did not change to 2048")
	}
}

func TestConfigValidateDefaultSuccess(t *testing.T) {
	c := DefaultConfig()

	if err := c.Validate(false); err != nil {
		t.Error(err)
	}
}

func TestConfigValidateDefaultSuccessOnExecution(t *testing.T) {
	c := DefaultConfig()

	// since some test systems do not have runc installed, assume a more
	// generally available executable
	c.Runtimes["runc"] = oci.RuntimeHandler{RuntimePath: "/bin/sh"}

	if err := c.Validate(true); err != nil {
		t.Error(err)
	}
}

func TestConfigValidateSuccessAdditionalDevices(t *testing.T) {
	c := DefaultConfig()

	c.AdditionalDevices = []string{"/dev/null:/dev/null:rw"}

	if err := c.Validate(false); err != nil {
		t.Error(err)
	}
}

func TestConfigValidateFailOnParseUlimit(t *testing.T) {
	c := DefaultConfig()

	c.DefaultUlimits = []string{"wrong"}

	if err := c.Validate(false); err == nil {
		t.Error("should fail on wrong ParseUlimit")
	}
}

func TestConfigValidateFailOnInvalidDeviceSpecification(t *testing.T) {
	c := DefaultConfig()

	c.AdditionalDevices = []string{"::::"}

	if err := c.Validate(false); err == nil {
		t.Error("should fail on wrong invalid device specification")
	}
}

func TestConfigValidateFailOnInvalidDevice(t *testing.T) {
	c := DefaultConfig()

	c.AdditionalDevices = []string{"wrong"}

	if err := c.Validate(false); err == nil {
		t.Error("should fail on invalid device")
	}
}

func TestConfigValidateFailOnInvalidDeviceMode(t *testing.T) {
	c := DefaultConfig()

	c.AdditionalDevices = []string{"/dev/null:/dev/null:abc"}

	if err := c.Validate(false); err == nil {
		t.Error("should fail on invalid device mode")
	}
}

func TestConfigValidateFailOnInvalidDeviceFirst(t *testing.T) {
	c := DefaultConfig()

	c.AdditionalDevices = []string{"wrong:/dev/null:rw"}

	if err := c.Validate(false); err == nil {
		t.Error("should fail on invalid first device")
	}
}

func TestConfigValidateFailOnInvalidDeviceSecond(t *testing.T) {
	c := DefaultConfig()

	c.AdditionalDevices = []string{"/dev/null:wrong:rw"}

	if err := c.Validate(false); err == nil {
		t.Error("should fail on invalid second device")
	}
}

func TestConfigValidateFailOnNoDefaultRuntime(t *testing.T) {
	c := DefaultConfig()

	c.Runtimes = make(map[string]oci.RuntimeHandler)

	if err := c.Validate(false); err == nil {
		t.Error("should fail on no default runtime")
	}
}

func TestConfigValidateFailOnConflictingDefinition(t *testing.T) {
	c := DefaultConfig()

	c.Runtimes[oci.UntrustedRuntime] = oci.RuntimeHandler{}
	c.RuntimeUntrustedWorkload = "value"

	if err := c.Validate(false); err == nil {
		t.Error("should fail on conflicting definitions")
	}
}

func TestConfigValidateFailOnExecutionWithoutExistingRuntime(t *testing.T) {
	c := DefaultConfig()

	c.Runtime = "not-existing"

	if err := c.Validate(true); err == nil {
		t.Error("should fail on non existing runtime")
	}
}

func TestConfigValidateFailOnExecutionWithoutExistingRuntimeHandler(t *testing.T) {
	c := DefaultConfig()

	c.Runtimes["runc"] = oci.RuntimeHandler{RuntimePath: "not-existing"}

	if err := c.Validate(true); err == nil {
		t.Error("should fail on non existing runtime")
	}
}
