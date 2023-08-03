//go:build !linux
// +build !linux

package apparmor

// DefaultProfile is the default profile name
const DefaultProfile = "crio-default"

// Config is the global AppArmor configuration type
type Config struct {
	enabled bool
}

// New creates a new default AppArmor configuration instance
func New() *Config {
	return &Config{
		enabled: false,
	}
}

// LoadProfile can be used to load a AppArmor profile from the provided path.
// This method will not fail if AppArmor is disabled.
func (c *Config) LoadProfile(profile string) error {
	return nil
}
