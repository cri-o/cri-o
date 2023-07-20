//go:build !linux
// +build !linux

package capabilities

// Capabilities is the default representation for capabilities
type Capabilities []string

func Default() Capabilities {
	return []string{}
}

// Validate checks if the provided capabilities are available on the system
func (c Capabilities) Validate() error {
	return nil
}
