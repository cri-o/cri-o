package migrate

import (
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/pkg/errors"
)

// All possible migration scenarios
const (
	FromPrevious = From1_20
	From1_17     = "1.17"
	From1_20     = "1.20"
)

// Config migrates the provided config from the provided scenario to the
// current one.
func Config(cfg *config.Config, from string) error {
	// 1.17 specific settings
	if from == From1_17 {
		return migrateFrom1_17(cfg)
	}

	// 1.20 specific settings
	if from == From1_20 {
		return migrateFrom1_20(cfg)
	}

	return errors.Errorf("unsupported migration version %q", from)
}
