package plugins

import (
	"os"
	"path/filepath"
)

var specsPaths = []string{filepath.Join(os.Getenv("programdata"), "containers", "storage", "plugins")}
