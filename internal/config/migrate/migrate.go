package migrate

import (
	"fmt"

	"github.com/cri-o/cri-o/pkg/config"
)

// All possible migration scenarios
const (
	FromPrevious = From1_17
	From1_17     = "1.17"
)

// Config migrates the provided config from the provided scenario to the
// current one.
func Config(cfg *config.Config, from string) error {
	// 1.17 specific settings
	if from == From1_17 {
		return migrateFrom1_17(cfg)
	}

	return fmt.Errorf("unsupported migration version %q", from)
}
