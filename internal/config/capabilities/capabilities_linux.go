package capabilities

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	common "go.podman.io/common/pkg/capabilities"
)

// Capabilities is the default representation for capabilities.
type Capabilities []string

// Default returns the default capabilities as string slice.
func Default() Capabilities {
	return []string{
		"CHOWN",
		"DAC_OVERRIDE",
		"FSETID",
		"FOWNER",
		"SETGID",
		"SETUID",
		"SETPCAP",
		"NET_BIND_SERVICE",
		"KILL",
	}
}

// Validate checks if the provided capabilities are available on the system.
func (c Capabilities) Validate() error {
	caps := Capabilities{}
	for _, cap := range c {
		caps = append(caps, "CAP_"+strings.ToUpper(cap))
	}

	if err := common.ValidateCapabilities(caps); err != nil {
		return fmt.Errorf("validating capabilities: %w", err)
	}

	logrus.Infof("Using default capabilities: %s", strings.Join(caps, ", "))

	return nil
}
