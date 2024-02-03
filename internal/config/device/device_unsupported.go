//go:build !linux

package device

// Device holds the runtime spec
// fields needed for a device
type Device struct {
}

type Config struct {
}

// New creates a new device Config
func New() *Config {
	return &Config{}
}

func (d *Config) LoadDevices(devsFromConfig []string) error {
	return nil
}

// Devices returns the devices saved in the Config
func (d *Config) Devices() []Device {
	return nil
}

func DevicesFromAnnotation(annotation string, allowedDevices []string) ([]Device, error) {
	return []Device{}, nil
}
