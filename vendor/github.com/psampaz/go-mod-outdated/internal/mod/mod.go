// Package mod provides functionality around modules
package mod

import (
	"time"
)

// Module holds information about a specific module listed by go list
type Module struct {
	Path      string       `json:",omitempty"` // module path
	Version   string       `json:",omitempty"` // module version
	Versions  []string     `json:",omitempty"` // available module versions
	Replace   *Module      `json:",omitempty"` // replaced by this module
	Time      *time.Time   `json:",omitempty"` // time version was created
	Update    *Module      `json:",omitempty"` // available update (with -u)
	Main      bool         `json:",omitempty"` // is this the main module?
	Indirect  bool         `json:",omitempty"` // module is only indirectly needed by main module
	Dir       string       `json:",omitempty"` // directory holding local copy of files, if any
	GoMod     string       `json:",omitempty"` // path to go.mod file describing module, if any
	Error     *ModuleError `json:",omitempty"` // error loading module
	GoVersion string       `json:",omitempty"` // go version used in module
}

// ModuleError represents the error when a module cannot be loaded
type ModuleError struct {
	Err string // error text
}

// InvalidTimestamp checks if the version reported as update by the go list command is actually newer that current version
func (m *Module) InvalidTimestamp() bool {
	var mod Module
	if m.Replace != nil {
		mod = *m.Replace
	} else {
		mod = *m
	}

	if mod.Time != nil && mod.Update != nil {
		return mod.Time.After(*mod.Update.Time)
	}

	return false
}

// CurrentVersion returns the current version of the module taking into consideration the any Replace settings
func (m *Module) CurrentVersion() string {
	var mod Module
	if m.Replace != nil {
		mod = *m.Replace
	} else {
		mod = *m
	}

	return mod.Version
}

// HasUpdate checks if the module has a new version
func (m *Module) HasUpdate() bool {
	var mod Module
	if m.Replace != nil {
		mod = *m.Replace
	} else {
		mod = *m
	}

	return mod.Update != nil
}

// NewVersion returns the version of the update taking into consideration the any Replace settings
func (m *Module) NewVersion() string {
	var mod Module
	if m.Replace != nil {
		mod = *m.Replace
	} else {
		mod = *m
	}

	if mod.Update == nil {
		return ""
	}

	return mod.Update.Version
}

// FilterModules filters the list of modules provided by the go list command
func FilterModules(modules []Module, hasUpdate, isDirect bool) []Module {
	out := make([]Module, 0)

	for k := range modules {
		if modules[k].Main {
			continue
		}

		if hasUpdate && modules[k].Update == nil {
			continue
		}

		if isDirect && modules[k].Indirect {
			continue
		}

		out = append(out, modules[k])
	}

	return out
}
